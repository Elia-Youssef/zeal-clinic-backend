package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"clinic-api/internal/buildmode"

	"github.com/google/uuid"
)

// InvalidateCache is wired at startup (cmd/server/main.go) to the HTTP cache
// middleware's InvalidateCache. Lives here as a hook because the middleware
// package transitively imports database/store, which would close an import
// cycle if sync imported middleware directly. Nil-safe: tests that don't
// wire it up simply skip invalidation.
var InvalidateCache func(key string)

// Apply ingests a batch of LogEntry rows into the database under one
// transaction. The trigger guard (_sync_applying.applying) is raised and
// lowered *inside* the same tx as the data writes so there's no window
// where the connection is free with the flag still raised; that would
// let a user write land with logging suppressed and silently drift from
// the cloud.
//
// On the local clinic binary (buildmode.Cloud=false) Apply enforces the
// LWW gate: a remote write only lands when local's updated_at is older
// than the incoming entry; otherwise local is preserved and a conflict is
// logged. On the cloud (buildmode.Cloud=true) all writes land
// unconditionally: local is the source of truth.
//
// NoDelete tables reject incoming DELETEs on both sides regardless
// (financial history invariant); the rejection is logged.
//
// Returns the highest seq successfully applied, plus any conflicts that
// were rejected/logged.
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

	// Apply inserts/updates parents-first; then deletes children-first.
	// We bucket the batch by table and re-issue in level order. seq order
	// is preserved within a bucket so per-row history collapses naturally
	// (later writes overwrite earlier ones).
	byTable := make(map[string][]LogEntry)
	for _, e := range batch {
		byTable[e.Table] = append(byTable[e.Table], e)
	}

	// Tables where at least one row actually landed (excludes LWW/no_delete
	// rejections). Used after commit to bust HTTP caches that depend on them.
	changedTables := make(map[string]struct{})

	// Inserts/updates pass: forward table order.
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

	// Deletes pass: reverse table order.
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

	// Persist conflict log inside the same tx so it can't disagree with state.
	for _, c := range conflicts {
		if _, err = tx.Exec(
			`INSERT INTO sync_conflicts(id, table_name, row_id, local_json, remote_json, resolution) VALUES (?,?,?,?,?,?)`,
			uuid.Must(uuid.NewV7()).String(), c.Table, c.RowID, c.LocalJSON, c.RemoteJSON, c.Resolution,
		); err != nil {
			return 0, nil, fmt.Errorf("log conflict: %w", err)
		}
	}

	// Lower the guard before commit so the committed state is always 0.
	// Same tx, atomic with the data writes.
	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		return 0, nil, fmt.Errorf("lower sync guard: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, nil, fmt.Errorf("commit: %w", err)
	}

	invalidateCachesFor(changedTables)
	return maxApplied, conflicts, nil
}

// invalidateCachesFor busts every HTTP cache bucket declared by the changed
// tables. Called after a successful Apply commit so callers that follow up
// with a realtime "data_changed" broadcast see fresh data on refetch; the
// cache middleware would otherwise serve the pre-sync response.
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

// applyUpsert handles insert/update entries. Returns a ConflictEntry only
// when LWW rejected the remote write (local wins); the caller logs it.
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
		// Financial history is never deleted via sync. Log so staff can review.
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

// localWinsCheck returns (true, localJSON, nil) if the existing row should
// be kept (local.updated_at is strictly newer than the incoming entry).
// Returns (false, _, nil) when the remote write should proceed.
//
// Caller must only invoke this when t.HasUpdatedAt is true.
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
		// No comparable timestamp: trust remote.
		return false, "", nil
	}

	// String compare works because both sides use SQLite's
	// datetime('now')/'yyyy-mm-dd hh:mm:ss' ISO-like format consistently.
	if localUpdatedAt.String > e.UpdatedAt {
		localJSON, _ = fetchRowJSON(tx, t, e.RowID)
		return true, localJSON, nil
	}
	return false, "", nil
}

// fetchRowJSON serialises a single row by its PK. Best-effort: empty on error/missing.
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

// normalizeJSONValue converts json.Unmarshal'd values back to types the
// SQLite driver accepts. JSON numbers come through as float64; coerce to
// int64 when they're whole, otherwise leave as float64. Booleans become 0/1.
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
		// Triggers ship scalar values; maps/slices would only appear for
		// JSON-typed columns. Re-marshal as text for safety.
		buf, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(buf)
	}
}
