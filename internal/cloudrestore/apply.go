package cloudrestore

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	syncpkg "clinic-api/internal/sync"
)

type ApplyResult struct {
	Tables        int    `json:"tables"`
	Rows          int64  `json:"rows"`
	LocalBaseline int64  `json:"localBaseline"`
	CloudBaseline int64  `json:"cloudBaseline"`
	Backup        string `json:"backup,omitempty"`
	RestoreID     string `json:"restoreId,omitempty"`
}

// ApplySnapshot replaces exactly the tables declared by sync.SyncedTables.
// Node-local tables remain cloud-owned. Tokens and notifications are preserved
// for user IDs that still exist after the authoritative local users table is
// installed.
func ApplySnapshot(target, source *sql.DB) (result ApplyResult, err error) {
	if err := validateSnapshot(source, target); err != nil {
		return result, err
	}
	result.LocalBaseline, err = outboxHighWater(source)
	if err != nil {
		return result, fmt.Errorf("read local outbox baseline: %w", err)
	}
	result.CloudBaseline, err = outboxHighWater(target)
	if err != nil {
		return result, fmt.Errorf("read cloud outbox baseline: %w", err)
	}

	tx, err := target.Begin()
	if err != nil {
		return result, fmt.Errorf("begin restore: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 1 WHERE rowid = 1`); err != nil {
		return result, fmt.Errorf("raise sync guard: %w", err)
	}
	if err = preserveNodeLocalRows(tx); err != nil {
		return result, err
	}

	for i := len(syncpkg.SyncedTables) - 1; i >= 0; i-- {
		name := syncpkg.SyncedTables[i].Name
		if _, err = tx.Exec(`DELETE FROM ` + quoteIdent(name)); err != nil {
			return result, fmt.Errorf("clear %s: %w", name, err)
		}
	}

	for _, table := range syncpkg.SyncedTables {
		var n int64
		n, err = copyTable(tx, source, table.Name)
		if err != nil {
			return result, err
		}
		result.Rows += n
		result.Tables++
	}

	if err = restoreNodeLocalRows(tx); err != nil {
		return result, err
	}
	if _, err = tx.Exec(`DELETE FROM sync_log`); err != nil {
		return result, fmt.Errorf("clear cloud outbox: %w", err)
	}
	if _, err = tx.Exec(`DELETE FROM sync_conflicts`); err != nil {
		return result, fmt.Errorf("clear cloud conflicts: %w", err)
	}
	if _, err = tx.Exec(`DELETE FROM sync_state`); err != nil {
		return result, fmt.Errorf("clear cloud sync state: %w", err)
	}
	if err = setSyncState(tx, result.CloudBaseline, result.LocalBaseline); err != nil {
		return result, err
	}
	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		return result, fmt.Errorf("lower sync guard: %w", err)
	}
	if err = foreignKeyCheckTx(tx); err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, fmt.Errorf("commit restore: %w", err)
	}
	if err = checkDatabase(target); err != nil {
		return result, fmt.Errorf("validate restored cloud database: %w", err)
	}
	return result, nil
}

// FinalizeLocal clears the local outbox only after the cloud has committed the
// uploaded snapshot. Keeping sqlite_sequence intact makes future sequence IDs
// greater than the returned baseline.
func FinalizeLocal(db *sql.DB, expectedLocalBaseline, cloudBaseline int64) error {
	current, err := outboxHighWater(db)
	if err != nil {
		return fmt.Errorf("read local outbox baseline: %w", err)
	}
	if current != expectedLocalBaseline {
		return fmt.Errorf("local data changed during restore: outbox baseline %d became %d", expectedLocalBaseline, current)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM sync_log`); err != nil {
		return fmt.Errorf("clear local outbox: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM sync_conflicts`); err != nil {
		return fmt.Errorf("clear local conflicts: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM sync_state`); err != nil {
		return fmt.Errorf("clear local sync state: %w", err)
	}
	if err := setSyncState(tx, expectedLocalBaseline, cloudBaseline); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		return fmt.Errorf("reset local sync guard: %w", err)
	}
	return tx.Commit()
}

func preserveNodeLocalRows(tx *sql.Tx) error {
	for _, statement := range []string{
		`DROP TABLE IF EXISTS temp._cloud_restore_tokens`,
		`DROP TABLE IF EXISTS temp._cloud_restore_notifications`,
		`CREATE TEMP TABLE _cloud_restore_tokens AS SELECT * FROM tokens`,
		`CREATE TEMP TABLE _cloud_restore_notifications AS SELECT * FROM notifications`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("preserve node-local rows: %w", err)
		}
	}
	return nil
}

func restoreNodeLocalRows(tx *sql.Tx) error {
	for _, statement := range []string{
		`INSERT INTO tokens SELECT t.* FROM _cloud_restore_tokens t JOIN users u ON u.id = t.user_id`,
		`INSERT INTO notifications SELECT n.* FROM _cloud_restore_notifications n JOIN users u ON u.id = n.user_id`,
		`DROP TABLE _cloud_restore_tokens`,
		`DROP TABLE _cloud_restore_notifications`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("restore node-local rows: %w", err)
		}
	}
	return nil
}

func copyTable(tx *sql.Tx, source *sql.DB, table string) (int64, error) {
	rows, err := source.Query(`SELECT * FROM ` + quoteIdent(table))
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("columns %s: %w", table, err)
	}
	quoted := make([]string, len(columns))
	marks := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = quoteIdent(column)
		marks[i] = "?"
	}
	statement := `INSERT INTO ` + quoteIdent(table) + ` (` + strings.Join(quoted, ",") + `) VALUES (` + strings.Join(marks, ",") + `)`
	insert, err := tx.Prepare(statement)
	if err != nil {
		return 0, fmt.Errorf("prepare %s: %w", table, err)
	}
	defer insert.Close()

	var count int64
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return 0, fmt.Errorf("scan %s: %w", table, err)
		}
		if _, err := insert.Exec(values...); err != nil {
			return 0, fmt.Errorf("insert %s: %w", table, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read %s: %w", table, err)
	}
	return count, nil
}

func setSyncState(tx *sql.Tx, pushed, pulled int64) error {
	_, err := tx.Exec(`INSERT INTO sync_state(peer, last_pushed_seq, last_pulled_seq, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(peer) DO UPDATE SET
			last_pushed_seq = excluded.last_pushed_seq,
			last_pulled_seq = excluded.last_pulled_seq,
			updated_at = excluded.updated_at`,
		syncpkg.SyncedPeer, pushed, pulled, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("reset sync state: %w", err)
	}
	return nil
}

func foreignKeyCheckTx(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("foreign_key_check reported a violation")
	}
	return rows.Err()
}
