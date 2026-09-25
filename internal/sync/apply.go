package sync

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/tracking"

	"github.com/google/uuid"
)

// InvalidateCache is injected at startup to avoid a middleware import cycle.
var InvalidateCache func(key string)

// RecalcBalance is injected at startup to avoid a store import cycle.
var RecalcBalance func(tx *sql.Tx, balanceID string) error

// FKViolation names a row whose foreign key points at a missing parent: a
// row written without its parent, or a row left behind by a delete. Apply
// and a cloud restore report it instead of a bare commit failure.
type FKViolation struct {
	Table  string
	RowID  string
	Parent string
}

func (v FKViolation) Error() string {
	return fmt.Sprintf("%s/%s references %s", v.Table, v.RowID, v.Parent)
}

// Apply writes a sync batch in one tx. Local builds use local-wins LWW;
// cloud builds accept incoming rows. NoDelete tables reject deletes.
// Foreign keys are deferred for the batch, so rows may arrive in any order
// (a rescheduled appointment before its original, a child before a parent
// written later), and the batch's own references are probed once before the
// commit; the deferred commit check stays the backstop.
func Apply(db *sql.DB, batch []LogEntry) (maxApplied int64, conflicts []ConflictEntry, err error) {
	if len(batch) == 0 {
		return 0, nil, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, nil, fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
		return 0, nil, fmt.Errorf("defer foreign keys: %w", err)
	}
	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 1 WHERE rowid = 1`); err != nil {
		return 0, nil, fmt.Errorf("raise sync guard: %w", err)
	}

	// Parents first for upserts, children first for deletes.
	byTable := make(map[string][]LogEntry)
	for _, e := range batch {
		byTable[e.Table] = append(byTable[e.Table], e)
	}

	changedTables := make(map[string]struct{})

	for _, t := range SyncedTables {
		for _, e := range byTable[t.Name] {
			if e.Op == "delete" {
				continue
			}
			c, applyErr := applyUpsert(tx, t, e)
			if applyErr != nil {
				err = fmt.Errorf("apply %s/%s: %w", e.Table, e.RowID, applyErr)
				return 0, nil, err
			}
			if c != nil {
				conflicts = append(conflicts, *c)
			} else {
				changedTables[t.Name] = struct{}{}
			}
			if e.Seq > maxApplied {
				maxApplied = e.Seq
			}
		}
	}

	for i := len(SyncedTables) - 1; i >= 0; i-- {
		t := SyncedTables[i]
		for _, e := range byTable[t.Name] {
			if e.Op != "delete" {
				continue
			}
			c, applyErr := applyDelete(tx, t, e)
			if applyErr != nil {
				err = fmt.Errorf("delete %s/%s: %w", e.Table, e.RowID, applyErr)
				return 0, nil, err
			}
			if c != nil {
				conflicts = append(conflicts, *c)
			} else {
				changedTables[t.Name] = struct{}{}
			}
			if e.Seq > maxApplied {
				maxApplied = e.Seq
			}
		}
	}

	for _, c := range conflicts {
		if _, err = tx.Exec(
			`INSERT INTO sync_conflicts(id, table_name, row_id, local_json, remote_json, resolution) VALUES (?,?,?,?,?,?)`,
			uuid.Must(uuid.NewV7()).String(), c.Table, c.RowID, c.LocalJSON, c.RemoteJSON, c.Resolution,
		); err != nil {
			return 0, nil, fmt.Errorf("log conflict: %w", err)
		}
	}

	if err = recalcBalancesForApplied(tx, byTable["balance_transactions"]); err != nil {
		return 0, nil, err
	}

	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		return 0, nil, fmt.Errorf("lower sync guard: %w", err)
	}

	if err = checkBatchForeignKeys(tx, byTable); err != nil {
		return 0, nil, err
	}
	if err = tx.Commit(); err != nil {
		return 0, nil, fmt.Errorf("commit: %w", err)
	}

	reportApplyOutcome(len(batch), conflicts)
	invalidateCachesFor(changedTables)
	return maxApplied, conflicts, nil
}

// recalcBalancesForApplied rebuilds balances touched by applied transactions so
// the projection can't drift from source. Missing balance rows are skipped.
func recalcBalancesForApplied(tx *sql.Tx, entries []LogEntry) error {
	if RecalcBalance == nil {
		return nil
	}
	affected := map[string]struct{}{}
	for _, e := range entries {
		if e.Op == "delete" {
			continue
		}
		var row struct {
			From string `json:"from_balance_id"`
			To   string `json:"to_balance_id"`
		}
		if json.Unmarshal(e.RowJSON, &row) != nil {
			continue
		}
		if row.From != "" {
			affected[row.From] = struct{}{}
		}
		if row.To != "" {
			affected[row.To] = struct{}{}
		}
	}
	for id := range affected {
		if err := RecalcBalance(tx, id); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("recalc balance %s: %w", id, err)
		}
	}
	return nil
}

// reportApplyOutcome fires only when an apply produced conflicts. Successful
// applies happen on every change and would flood logs; errors are tracked
// at the cycle/handler level instead.
func reportApplyOutcome(total int, conflicts []ConflictEntry) {
	if len(conflicts) == 0 {
		return
	}
	side := "local"
	if buildmode.Cloud {
		side = "cloud"
	}
	applied := total - len(conflicts)
	tracking.WarnWith(nil,
		fmt.Sprintf("[sync] %s applied %d rows with %d conflicts", side, applied, len(conflicts)),
		map[string]any{"conflicts": conflicts},
	)
}

// invalidateCachesFor clears cache buckets affected by applied sync rows.
func invalidateCachesFor(changedTables map[string]struct{}) {
	if InvalidateCache == nil || len(changedTables) == 0 {
		return
	}
	keys := make(map[string]struct{})
	for name := range changedTables {
		t, ok := IsSyncedTable(name)
		if !ok {
			continue
		}
		for _, k := range t.CacheKeys {
			keys[k] = struct{}{}
		}
	}
	for k := range keys {
		InvalidateCache(k)
	}
}

// applyUpsert returns a conflict only when local-wins rejects the row.
func applyUpsert(tx *sql.Tx, t TableInfo, e LogEntry) (*ConflictEntry, error) {
	if len(e.RowJSON) == 0 {
		return nil, fmt.Errorf("empty row_json for %s/%s op=%s", e.Table, e.RowID, e.Op)
	}
	var row map[string]any
	if err := json.Unmarshal(e.RowJSON, &row); err != nil {
		return nil, fmt.Errorf("decode row_json: %w", err)
	}
	if _, ok := row[t.PK()]; !ok {
		return nil, fmt.Errorf("row_json missing %s", t.PK())
	}

	if !buildmode.Cloud && t.HasUpdatedAt {
		keep, localJSON, err := localWinsCheck(tx, t, e)
		if err != nil {
			return nil, err
		}
		if keep {
			return &ConflictEntry{
				Table:      t.Name,
				RowID:      e.RowID,
				LocalJSON:  localJSON,
				RemoteJSON: string(e.RowJSON),
				Resolution: "local_wins",
			}, nil
		}
	}

	return nil, upsert(tx, t, row)
}

func applyDelete(tx *sql.Tx, t TableInfo, e LogEntry) (*ConflictEntry, error) {
	if t.NoDelete {
		body, _, _, _ := fetchLiveRow(tx, t, e.RowID)
		return &ConflictEntry{
			Table:      t.Name,
			RowID:      e.RowID,
			LocalJSON:  string(body),
			RemoteJSON: "",
			Resolution: "no_delete",
		}, nil
	}
	if !buildmode.Cloud && t.HasUpdatedAt {
		keep, localJSON, err := localWinsCheck(tx, t, e)
		if err != nil {
			return nil, err
		}
		if keep {
			return &ConflictEntry{
				Table:      t.Name,
				RowID:      e.RowID,
				LocalJSON:  localJSON,
				RemoteJSON: "",
				Resolution: "local_wins",
			}, nil
		}
	}
	_, err := tx.Exec(
		fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, quoteIdent(t.Name), quoteIdent(t.PK())),
		e.RowID,
	)
	return nil, err
}

// localWinsCheck keeps local rows whose updated_at is newer.
func localWinsCheck(tx *sql.Tx, t TableInfo, e LogEntry) (keep bool, localJSON string, err error) {
	var localUpdatedAt sql.NullString
	scanErr := tx.QueryRow(
		fmt.Sprintf(`SELECT IFNULL(updated_at, '') FROM %s WHERE %s = ?`,
			quoteIdent(t.Name), quoteIdent(t.PK())),
		e.RowID,
	).Scan(&localUpdatedAt)
	if scanErr == sql.ErrNoRows {
		return false, "", nil
	}
	if scanErr != nil {
		return false, "", scanErr
	}

	if e.UpdatedAt == "" || localUpdatedAt.String == "" {
		return false, "", nil
	}

	if localUpdatedAt.String > e.UpdatedAt {
		body, _, _, _ := fetchLiveRow(tx, t, e.RowID)
		return true, string(body), nil
	}
	return false, "", nil
}

func upsert(tx *sql.Tx, t TableInfo, row map[string]any) error {
	pk := t.PK()
	keys := sortedKeys(row)
	cols := make([]string, 0, len(keys))
	placeholders := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	setClauses := make([]string, 0, len(keys))
	for _, k := range keys {
		cols = append(cols, quoteIdent(k))
		placeholders = append(placeholders, "?")
		args = append(args, normalizeJSONValue(row[k]))
		if k != pk {
			setClauses = append(setClauses, fmt.Sprintf("%s = excluded.%s", quoteIdent(k), quoteIdent(k)))
		}
	}
	q := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(%s) DO UPDATE SET %s`,
		quoteIdent(t.Name),
		strings.Join(cols, ","),
		strings.Join(placeholders, ","),
		quoteIdent(pk),
		strings.Join(setClauses, ","),
	)
	_, err := tx.Exec(q, args...)
	return err
}

// checkBatchForeignKeys probes the batch's own references in the open
// transaction: every upserted row's parents must exist (rows this batch
// wrote included), and a deleted row must leave nothing behind that still
// references it. Anything the batch doesn't touch is left to the deferred
// commit check, the backstop.
func checkBatchForeignKeys(tx *sql.Tx, byTable map[string][]LogEntry) error {
	for _, t := range SyncedTables {
		entries, ok := byTable[t.Name]
		if !ok {
			continue
		}
		refs, err := parentRefs(tx, t.Name)
		if err != nil {
			return err
		}
		children, err := childRefs(tx, t.Name)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Op == "delete" {
				err = checkDeleteChildren(tx, t, e, children)
			} else {
				err = checkUpsertParents(tx, t, e, refs)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// checkUpsertParents probes every foreign key of the stored row: a kept
// local row is checked as it stands, a written row at what the batch wrote.
func checkUpsertParents(tx *sql.Tx, t TableInfo, e LogEntry, refs []FKRef) error {
	if len(refs) == 0 {
		return nil
	}
	cols := make([]string, len(refs))
	values := make([]sql.NullString, len(refs))
	scan := make([]any, len(refs))
	for i, ref := range refs {
		cols[i] = quoteIdent(ref.Column)
		scan[i] = &values[i]
	}
	err := tx.QueryRow(
		fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`,
			strings.Join(cols, ","), quoteIdent(t.Name), quoteIdent(t.PK())),
		e.RowID,
	).Scan(scan...)
	if err == sql.ErrNoRows {
		return nil // nothing of the row is stored; there is nothing to check
	}
	if err != nil {
		return fmt.Errorf("load %s/%s for its foreign keys: %w", t.Name, e.RowID, err)
	}

	for i, ref := range refs {
		if !values[i].Valid || values[i].String == "" {
			continue
		}
		var one int
		err := tx.QueryRow(
			fmt.Sprintf(`SELECT 1 FROM %s WHERE %s = ? LIMIT 1`,
				quoteIdent(ref.Parent), quoteIdent(tableSet[ref.Parent].PK())),
			values[i].String,
		).Scan(&one)
		if err == sql.ErrNoRows {
			return FKViolation{Table: t.Name, RowID: e.RowID, Parent: ref.Parent}
		}
		if err != nil {
			return fmt.Errorf("check %s/%s against %s: %w", t.Name, e.RowID, ref.Parent, err)
		}
	}
	return nil
}

// checkDeleteChildren probes the tables referencing a deleted row. A refused
// or kept delete leaves the row in place, so its references still hold.
func checkDeleteChildren(tx *sql.Tx, t TableInfo, e LogEntry, children []FKChild) error {
	if len(children) == 0 {
		return nil
	}
	var one int
	err := tx.QueryRow(
		fmt.Sprintf(`SELECT 1 FROM %s WHERE %s = ? LIMIT 1`, quoteIdent(t.Name), quoteIdent(t.PK())),
		e.RowID,
	).Scan(&one)
	if err != nil {
		if err == sql.ErrNoRows {
			return firstChildLeftBehind(tx, t, e, children)
		}
		return fmt.Errorf("check delete %s/%s: %w", t.Name, e.RowID, err)
	}
	return nil
}

func firstChildLeftBehind(tx *sql.Tx, t TableInfo, e LogEntry, children []FKChild) error {
	for _, c := range children {
		var childID string
		err := tx.QueryRow(
			fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ? LIMIT 1`,
				quoteIdent(tableSet[c.Table].PK()), quoteIdent(c.Table), quoteIdent(c.Column)),
			e.RowID,
		).Scan(&childID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return fmt.Errorf("check %s/%s children in %s: %w", t.Name, e.RowID, c.Table, err)
		}
		return FKViolation{Table: c.Table, RowID: childID, Parent: t.Name}
	}
	return nil
}

// CheckForeignKeys reports the first foreign-key violation in the database,
// naming the offending row and the table it references. A cloud restore runs
// it before committing its replacement of every synced table; an apply uses
// the batch-scoped checkBatchForeignKeys instead.
func CheckForeignKeys(tx *sql.Tx) error {
	v, err := firstFKViolation(tx)
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	if t, ok := IsSyncedTable(v.table); ok {
		var id string
		if err := tx.QueryRow(
			fmt.Sprintf(`SELECT %s FROM %s WHERE rowid = ?`, quoteIdent(t.PK()), quoteIdent(v.table)),
			v.rowid,
		).Scan(&id); err == nil {
			return FKViolation{Table: v.table, RowID: id, Parent: v.parent}
		}
	}
	return fmt.Errorf("%s row %d references %s", v.table, v.rowid, v.parent)
}

type fkViolation struct {
	table  string
	rowid  int64
	parent string
}

func firstFKViolation(tx *sql.Tx) (*fkViolation, error) {
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return nil, fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var v fkViolation
	var fkid int
	if err := rows.Scan(&v.table, &v.rowid, &v.parent, &fkid); err != nil {
		return nil, fmt.Errorf("foreign_key_check: %w", err)
	}
	return &v, nil
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// normalizeJSONValue converts decoded JSON values for SQLite writes.
func normalizeJSONValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case string:
		return x
	default:
		buf, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(buf)
	}
}
