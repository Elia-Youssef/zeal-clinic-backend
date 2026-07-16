package sync

import (
	"database/sql"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestSyncFailureNotificationIsOncePerFailureEpisode(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE users (id TEXT PRIMARY KEY, is_active INTEGER NOT NULL);
		CREATE TABLE notifications (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			action TEXT NOT NULL,
			is_read INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
		INSERT INTO users (id, is_active) VALUES ('user-1', 1)`); err != nil {
		t.Fatal(err)
	}

	markSyncRecovered()
	t.Cleanup(markSyncRecovered)
	notifySyncFailure(db)
	if count := syncFailureNotificationCount(t, db); count != 1 {
		t.Fatalf("first failure notification count = %d, want 1", count)
	}

	if _, err := db.Exec(`DELETE FROM notifications WHERE action = ?`, syncFailureAction); err != nil {
		t.Fatal(err)
	}
	notifySyncFailure(db)
	if count := syncFailureNotificationCount(t, db); count != 0 {
		t.Fatalf("dismissed notification was recreated during the same episode: count = %d", count)
	}

	markSyncRecovered()
	notifySyncFailure(db)
	if count := syncFailureNotificationCount(t, db); count != 1 {
		t.Fatalf("new failure episode notification count = %d, want 1", count)
	}
}

func syncFailureNotificationCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE action = ?`, syncFailureAction).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
