package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	syncpkg "clinic-api/internal/sync"

	"github.com/pressly/goose/v3"
)

// openTestDB opens a migrated database in a temp dir through Open. Both
// connection pools are closed when the test ends.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "clinic.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return db
}

// TestMigrate_SyncTriggers brings up a temp database through the full
// migration chain and checks that every synced table got its three
// triggers, the apply-guard table is seeded, and the old peer_pull_cursor
// column is gone.
func TestMigrate_SyncTriggers(t *testing.T) {
	db := openTestDB(t)

	wantTriggers := len(syncpkg.SyncedTables) * 3
	var gotTriggers int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name LIKE '_sync_log_%'`,
	).Scan(&gotTriggers); err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if gotTriggers != wantTriggers {
		t.Errorf("triggers: got %d, want %d", gotTriggers, wantTriggers)
	}

	var applying int
	if err := db.QueryRow(`SELECT applying FROM _sync_applying WHERE rowid=1`).Scan(&applying); err != nil {
		t.Fatalf("_sync_applying: %v", err)
	}
	if applying != 0 {
		t.Errorf("_sync_applying.applying = %d, want 0", applying)
	}

	rows, err := db.Query(`PRAGMA table_info(sync_state)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "peer_pull_cursor" {
			t.Errorf("peer_pull_cursor column still present in sync_state")
		}
	}
}

// TestMigrate_TriggersFire writes a row into a synced table and confirms
// sync_log captures it: end-to-end sanity that the generated triggers and
// the apply guard play together correctly.
func TestMigrate_TriggersFire(t *testing.T) {
	db := openTestDB(t)

	var before int
	db.QueryRow(`SELECT COUNT(*) FROM sync_log WHERE table_name='rooms'`).Scan(&before)

	if _, err := db.Exec(`INSERT INTO rooms (id, name, type) VALUES ('t-1', 'Test', 'General')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var after int
	db.QueryRow(`SELECT COUNT(*) FROM sync_log WHERE table_name='rooms' AND row_id='t-1' AND op='insert'`).Scan(&after)
	if after-before != 1 {
		t.Errorf("expected 1 new sync_log row for rooms/t-1, got delta %d", after-before)
	}

	// Now raise the guard, write, and confirm logging is suppressed.
	if _, err := db.Exec(`UPDATE _sync_applying SET applying=1 WHERE rowid=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rooms (id, name, type) VALUES ('t-2', 'NotLogged', 'General')`); err != nil {
		t.Fatalf("guarded insert: %v", err)
	}
	if _, err := db.Exec(`UPDATE _sync_applying SET applying=0 WHERE rowid=1`); err != nil {
		t.Fatal(err)
	}

	var t2 int
	db.QueryRow(`SELECT COUNT(*) FROM sync_log WHERE row_id='t-2'`).Scan(&t2)
	if t2 != 0 {
		t.Errorf("guard failed: t-2 was logged %d times", t2)
	}
}

// TestMigrate_TriggersCollapse verifies that repeated writes to the same
// row collapse to a single sync_log entry, and the final op reflects the
// last write (e.g. insert, update, delete leaves exactly one 'delete' row).
func TestMigrate_TriggersCollapse(t *testing.T) {
	db := openTestDB(t)

	// Insert, then update 5 times. Should leave exactly one row, op=update.
	if _, err := db.Exec(`INSERT INTO rooms (id, name, type) VALUES ('c-1', 'A', 'General')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := db.Exec(`UPDATE rooms SET name = ? WHERE id = 'c-1'`, "n"+string(rune('0'+i))); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var op string
	db.QueryRow(`SELECT COUNT(*), MAX(op) FROM sync_log WHERE table_name='rooms' AND row_id='c-1'`).Scan(&count, &op)
	if count != 1 || op != "update" {
		t.Errorf("after insert+5 updates: got count=%d op=%q, want 1/update", count, op)
	}

	// Now delete: should still be one row, op=delete.
	if _, err := db.Exec(`DELETE FROM rooms WHERE id = 'c-1'`); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT COUNT(*), MAX(op) FROM sync_log WHERE row_id='c-1'`).Scan(&count, &op)
	if count != 1 || op != "delete" {
		t.Errorf("after delete: got count=%d op=%q, want 1/delete", count, op)
	}

	// Re-insert same id (rare but legal). Should collapse the delete.
	if _, err := db.Exec(`INSERT INTO rooms (id, name, type) VALUES ('c-1', 'B', 'General')`); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT COUNT(*), MAX(op) FROM sync_log WHERE row_id='c-1'`).Scan(&count, &op)
	if count != 1 || op != "insert" {
		t.Errorf("after re-insert: got count=%d op=%q, want 1/insert", count, op)
	}
}

// TestMigrate_RolesSyncByName verifies the PK-aware trigger path for the
// roles table, whose PK is `name` rather than `id`. The trigger should
// record `name` in sync_log.row_id, and repeated updates should collapse.
func TestMigrate_RolesSyncByName(t *testing.T) {
	db := openTestDB(t)

	// Seeds run before the trigger migration, so no sync_log spam from
	// seeding; both nodes seed identically anyway.

	if _, err := db.Exec(`UPDATE roles SET label = 'Administrator' WHERE name = 'admin'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE roles SET scopes = 'x' WHERE name = 'admin'`); err != nil {
		t.Fatal(err)
	}

	var count int
	var op string
	db.QueryRow(`SELECT COUNT(*), MAX(op) FROM sync_log WHERE table_name='roles' AND row_id='admin'`).Scan(&count, &op)
	if count != 1 || op != "update" {
		t.Errorf("after two role updates: got count=%d op=%q, want 1/update", count, op)
	}
}

// TestMigrate_AlandIslands checks that a fresh database holds "Åland Islands"
// correctly encoded and that migration 00014 repairs the mis-encoded name an
// older database may carry: after planting the old value, rolling back to
// 00013 and migrating up again fixes the row, and a second round changes
// nothing.
func TestMigrate_AlandIslands(t *testing.T) {
	db := openTestDB(t)

	const alandID = "a5fa8f1e-72e3-41a5-8ef1-22f3395e194d"
	const want = "Åland Islands"
	name := func() string {
		t.Helper()
		var n string
		if err := db.QueryRow(`SELECT name FROM countries WHERE id = ?`, alandID).Scan(&n); err != nil {
			t.Fatalf("query country %s: %v", alandID, err)
		}
		return n
	}
	if got := name(); got != want {
		t.Errorf("fresh database: name = %q, want %q", got, want)
	}

	if _, err := db.Exec(`UPDATE countries SET name = 'Ã…land Islands' WHERE id = ?`, alandID); err != nil {
		t.Fatalf("plant the old name: %v", err)
	}
	rerun := func() {
		t.Helper()
		if err := goose.DownTo(db, ".", 13); err != nil {
			t.Fatalf("roll back to 00013: %v", err)
		}
		if err := goose.Up(db, "."); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
	}
	rerun()
	if got := name(); got != want {
		t.Errorf("after 00014: name = %q, want %q", got, want)
	}
	rerun()
	if got := name(); got != want {
		t.Errorf("after running 00014 twice: name = %q, want %q", got, want)
	}
}
