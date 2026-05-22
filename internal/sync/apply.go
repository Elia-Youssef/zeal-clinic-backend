package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/tracking"

	"github.com/google/uuid"
)

// InvalidateCache is injected at startup to avoid a middleware import cycle.
var InvalidateCache func(key string)

// Apply writes a sync batch in one tx. Local builds use local-wins LWW;
// cloud builds accept incoming rows. NoDelete tables reject deletes.
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

	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		return 0, nil, fmt.Errorf("lower sync guard: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, nil, fmt.Errorf("commit: %w", err)
	}

	reportApplyOutcome(len(batch), conflicts)
	invalidateCachesFor(changedTables)
	return maxApplied, conflicts, nil
}

// reportApplyOutcome fires only when an apply produced conflicts. Successful
// applies happen on every change and would flood Sentry; errors are tracked
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
		localJSON, _ := fetchRowJSON(tx, t, e.RowID)
		return &ConflictEntry{
			Table:      t.Name,
			RowID:      e.RowID,
			LocalJSON:  localJSON,
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
		localJSON, _ = fetchRowJSON(tx, t, e.RowID)
		return true, localJSON, nil
	}
	return false, "", nil
}

// fetchRowJSON returns one row as JSON; empty means missing or scan failure.
func fetchRowJSON(tx *sql.Tx, t TableInfo, pkVal string) (string, error) {
	rows, err := tx.Query(
		fmt.Sprintf(`SELECT * FROM %s WHERE %s = ?`, quoteIdent(t.Name), quoteIdent(t.PK())),
		pkVal,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	byPK, err := scanRowsByPK(rows, t.PK())
	if err != nil || len(byPK) == 0 {
		return "", err
	}
	row, ok := byPK[pkVal]
	if !ok {
		return "", nil
	}
	buf, _ := json.Marshal(row)
	return string(buf), nil
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
