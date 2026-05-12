package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"clinic-api/internal/sync"

	"github.com/pressly/goose/v3"
)

// Three AFTER triggers per synced business table, same shape for all:
//
//	CREATE TRIGGER _sync_log_<t>_<op> AFTER <OP> ON <t>
//	  WHEN (SELECT applying FROM _sync_applying WHERE rowid = 1) = 0
//	BEGIN
//	  DELETE FROM sync_log WHERE table_name = '<t>' AND row_id = NEW.id;
//	  INSERT INTO sync_log(table_name, row_id, op) VALUES ('<t>', NEW.id, '<op>');
//	END;
//
// The DELETE collapses prior entries for the same row in place: only the
// last write per (table, row_id) ever sits in sync_log, since that's all the
// receiver needs. The idx_sync_log_table_row index keeps the lookup cheap.
//
// The WHEN clause skips logging while sync.Apply is replaying remote changes
// (the _sync_applying flag, raised inside the apply transaction). Triggers
// carry no row data: sync.push reads the live row at ship time, so adding
// a column to a synced table does not require touching this migration.
//
// To start syncing a new table, add it to internal/sync.SyncedTables and
// re-edit this migration in place (per the project's no-new-migrations rule)
// to pick it up.

func init() {
	goose.AddMigrationContext(upSyncTriggers, downSyncTriggers)
}

func upSyncTriggers(ctx context.Context, tx *sql.Tx) error {
	for _, t := range sync.SyncedTables {
		for _, op := range syncOps {
			ref := "NEW"
			if op.sql == "DELETE" {
				ref = "OLD"
			}
			rowVal := ref + "." + t.PK()
			stmt := fmt.Sprintf(
				`CREATE TRIGGER IF NOT EXISTS _sync_log_%s_%s AFTER %s ON "%s" `+
					`WHEN (SELECT applying FROM _sync_applying WHERE rowid = 1) = 0 `+
					`BEGIN `+
					`DELETE FROM sync_log WHERE table_name = '%s' AND row_id = %s; `+
					`INSERT INTO sync_log(table_name,row_id,op) VALUES ('%s', %s, '%s'); `+
					`END;`,
				t.Name, op.suffix, op.sql, t.Name,
				t.Name, rowVal,
				t.Name, rowVal, op.logOp,
			)
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("create trigger %s/%s: %w", t.Name, op.suffix, err)
			}
		}
	}
	return nil
}

func downSyncTriggers(ctx context.Context, tx *sql.Tx) error {
	for _, t := range sync.SyncedTables {
		for _, op := range syncOps {
			if _, err := tx.ExecContext(ctx,
				fmt.Sprintf(`DROP TRIGGER IF EXISTS _sync_log_%s_%s`, t.Name, op.suffix),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

var syncOps = []struct {
	sql, suffix, logOp string
}{
	{"INSERT", "ins", "insert"},
	{"UPDATE", "upd", "update"},
	{"DELETE", "del", "delete"},
}
