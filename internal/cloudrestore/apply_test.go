package cloudrestore

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"clinic-api/internal/database"
	syncpkg "clinic-api/internal/sync"
)

const seededSuperAdminID = "b10829b3-19a0-4813-8e1c-df6f5990737b"

func openRestoreTestDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := database.OpenStandalone(filepath.Join(t.TempDir(), name+".db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestApplySnapshot_ReplacesSyncedTablesAndPreservesCloudLocalRows(t *testing.T) {
	source := openRestoreTestDB(t, "source")
	target := openRestoreTestDB(t, "target")

	mustExec(t, source, `INSERT INTO rooms (id, name, type) VALUES ('local-room', 'Local Room', 'General')`)
	mustExec(t, source, `INSERT INTO tokens (id, token, user_id, expires_at) VALUES ('source-token-id', 'source-token', ?, '2099-01-01T00:00:00Z')`, seededSuperAdminID)

	mustExec(t, target, `INSERT INTO rooms (id, name, type) VALUES ('cloud-room', 'Cloud Room', 'General')`)
	mustExec(t, target, `INSERT INTO users (id, username, display_name, role) VALUES ('cloud-user', 'cloud-user', 'Cloud User', 'staff')`)
	mustExec(t, target, `INSERT INTO tokens (id, token, user_id, expires_at) VALUES ('shared-token-id', 'shared-token', ?, '2099-01-01T00:00:00Z')`, seededSuperAdminID)
	mustExec(t, target, `INSERT INTO tokens (id, token, user_id, expires_at) VALUES ('cloud-token-id', 'cloud-token', 'cloud-user', '2099-01-01T00:00:00Z')`)
	mustExec(t, target, `INSERT INTO notifications (id, user_id, title) VALUES ('shared-note', ?, 'Keep me')`, seededSuperAdminID)
	mustExec(t, target, `INSERT INTO notifications (id, user_id, title) VALUES ('cloud-note', 'cloud-user', 'Remove me')`)
	mustExec(t, target, `INSERT INTO audit_log (id, user_id, user_role, action, entity_type, entity_id) VALUES ('audit-1', 'cloud-user', 'staff', 'create', 'rooms', 'cloud-room')`)
	mustExec(t, target, `INSERT INTO sync_conflicts (id, table_name, row_id) VALUES ('conflict-1', 'rooms', 'cloud-room')`)

	localBaseline, err := outboxHighWater(source)
	if err != nil {
		t.Fatal(err)
	}
	cloudBaseline, err := outboxHighWater(target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplySnapshot(target, source)
	if err != nil {
		t.Fatalf("ApplySnapshot: %v", err)
	}
	if result.LocalBaseline != localBaseline || result.CloudBaseline != cloudBaseline {
		t.Fatalf("baselines = local %d cloud %d, want local %d cloud %d", result.LocalBaseline, result.CloudBaseline, localBaseline, cloudBaseline)
	}
	if result.Tables == 0 || result.Rows == 0 {
		t.Fatalf("empty result: %+v", result)
	}

	assertCount(t, target, 1, `SELECT COUNT(*) FROM rooms WHERE id = 'local-room'`)
	assertCount(t, target, 0, `SELECT COUNT(*) FROM rooms WHERE id = 'cloud-room'`)
	assertCount(t, target, 1, `SELECT COUNT(*) FROM tokens WHERE token = 'shared-token'`)
	assertCount(t, target, 0, `SELECT COUNT(*) FROM tokens WHERE token IN ('source-token', 'cloud-token')`)
	assertCount(t, target, 1, `SELECT COUNT(*) FROM notifications WHERE id = 'shared-note'`)
	assertCount(t, target, 0, `SELECT COUNT(*) FROM notifications WHERE id = 'cloud-note'`)
	assertCount(t, target, 1, `SELECT COUNT(*) FROM audit_log WHERE id = 'audit-1'`)
	assertCount(t, target, 0, `SELECT COUNT(*) FROM sync_log`)
	assertCount(t, target, 0, `SELECT COUNT(*) FROM sync_conflicts`)

	var pushed, pulled int64
	if err := target.QueryRow(`SELECT last_pushed_seq, last_pulled_seq FROM sync_state WHERE peer = 'peer'`).Scan(&pushed, &pulled); err != nil {
		t.Fatal(err)
	}
	if pushed != cloudBaseline || pulled != localBaseline {
		t.Fatalf("cloud state = pushed %d pulled %d, want pushed %d pulled %d", pushed, pulled, cloudBaseline, localBaseline)
	}
}

func TestApplySnapshot_InvalidDumpLeavesCloudUnchanged(t *testing.T) {
	source := openRestoreTestDB(t, "source-invalid")
	target := openRestoreTestDB(t, "target-valid")
	mustExec(t, target, `INSERT INTO rooms (id, name, type) VALUES ('cloud-marker', 'Cloud Marker', 'General')`)

	mustExec(t, source, `PRAGMA foreign_keys = OFF`)
	mustExec(t, source, `INSERT INTO procedure_prices (id, procedure_id, price) VALUES ('orphan-price', 'missing-procedure', 10)`)
	mustExec(t, source, `PRAGMA foreign_keys = ON`)

	if _, err := ApplySnapshot(target, source); err == nil {
		t.Fatal("ApplySnapshot accepted a dump with a foreign-key violation")
	}
	assertCount(t, target, 1, `SELECT COUNT(*) FROM rooms WHERE id = 'cloud-marker'`)
}

// The whole-database check a restore runs before its commit names the
// offending row with the typed violation.
func TestCheckForeignKeysNamesTheRow(t *testing.T) {
	db := openRestoreTestDB(t, "fk-check")
	mustExec(t, db, `PRAGMA foreign_keys = OFF`)
	mustExec(t, db, `INSERT INTO procedure_prices (id, procedure_id, price) VALUES ('orphan-price', 'missing-procedure', 10)`)
	mustExec(t, db, `PRAGMA foreign_keys = ON`)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	err = syncpkg.CheckForeignKeys(tx)
	var v syncpkg.FKViolation
	if err == nil || !errors.As(err, &v) {
		t.Fatalf("CheckForeignKeys error = %v, want an FKViolation", err)
	}
	if v != (syncpkg.FKViolation{
		Table: "procedure_prices", RowID: "orphan-price", Parent: "procedures",
	}) {
		t.Errorf("violation = %+v, want orphan-price referencing procedures", v)
	}
}

// A snapshot can list a rescheduled appointment before its original (a node
// that applied the two across batches stores them in that row order); the
// restore defers its foreign-key checks, so the copy succeeds.
func TestApplySnapshot_RestoresRescheduledAppointmentBeforeOriginal(t *testing.T) {
	source := openRestoreTestDB(t, "source-reschedule")
	target := openRestoreTestDB(t, "target-reschedule")
	mustExec(t, source, `INSERT INTO patients (id, first_name, last_name, date_of_birth, contact) VALUES ('p-1', 'First', 'Last', '1990-01-01', '')`)
	mustExec(t, source, `INSERT INTO rooms (id, name, type) VALUES ('r-1', 'Room', 'General')`)
	mustExec(t, source, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
		VALUES ('apt-second', 'p-1', 'r-1', '2026-02-02T08:00:00Z', '2026-02-02T09:00:00Z', 'Scheduled')`)
	mustExec(t, source, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
		VALUES ('apt-first', 'p-1', 'r-1', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Scheduled')`)
	mustExec(t, source, `UPDATE appointments SET rescheduled_from = 'apt-first' WHERE id = 'apt-second'`)

	if _, err := ApplySnapshot(target, source); err != nil {
		t.Fatalf("ApplySnapshot: %v", err)
	}
	assertCount(t, target, 2, `SELECT COUNT(*) FROM appointments`)
	assertCount(t, target, 1, `SELECT COUNT(*) FROM appointments WHERE id = 'apt-second' AND rescheduled_from = 'apt-first'`)
}

func TestFinalizeLocal_ClearsRowsAndKeepsMonotonicOutbox(t *testing.T) {
	db := openRestoreTestDB(t, "local-finalize")
	mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('before-finalize', 'Before', 'General')`)
	mustExec(t, db, `INSERT INTO sync_conflicts (id, table_name, row_id) VALUES ('old-conflict', 'rooms', 'before-finalize')`)
	baseline, err := outboxHighWater(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := FinalizeLocal(db, baseline, 91); err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, 0, `SELECT COUNT(*) FROM sync_log`)
	assertCount(t, db, 0, `SELECT COUNT(*) FROM sync_conflicts`)

	var pushed, pulled int64
	if err := db.QueryRow(`SELECT last_pushed_seq, last_pulled_seq FROM sync_state WHERE peer = 'peer'`).Scan(&pushed, &pulled); err != nil {
		t.Fatal(err)
	}
	if pushed != baseline || pulled != 91 {
		t.Fatalf("state = pushed %d pulled %d, want pushed %d pulled 91", pushed, pulled, baseline)
	}

	mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('after-finalize', 'After', 'General')`)
	var next int64
	if err := db.QueryRow(`SELECT seq FROM sync_log WHERE row_id = 'after-finalize'`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next <= baseline {
		t.Fatalf("new outbox seq %d did not advance past baseline %d", next, baseline)
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func assertCount(t *testing.T, db *sql.DB, want int, query string, args ...any) {
	t.Helper()
	var got int
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("count for %q = %d, want %d", query, got, want)
	}
}
