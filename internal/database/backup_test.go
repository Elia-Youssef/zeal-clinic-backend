package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestSnapshot_RoundTrip takes a live snapshot via VACUUM INTO and confirms the
// result is a standalone, encrypted copy: readable with the correct key (data
// intact, no WAL sidecar) and unreadable with a wrong key.
func TestSnapshot_RoundTrip(t *testing.T) {
	tmp, err := os.CreateTemp("", "clinic-bk-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	db, err := Open(tmp.Name())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO rooms (id, name, type) VALUES ('bk-1', 'Backed', 'General')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	dir := t.TempDir()
	path, err := Snapshot(dir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("snapshot path %q not in %q", path, dir)
	}
	if _, err := os.Stat(path + "-wal"); err == nil {
		t.Errorf("snapshot left a WAL sidecar; expected a standalone file")
	}

	// Reopen with the correct key — the row must be there.
	good, err := sql.Open("sqlite3", buildDSN(path, encryptionKey))
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer good.Close()
	var name string
	if err := good.QueryRow(`SELECT name FROM rooms WHERE id = 'bk-1'`).Scan(&name); err != nil {
		t.Fatalf("read from snapshot: %v", err)
	}
	if name != "Backed" {
		t.Errorf("snapshot row = %q, want %q", name, "Backed")
	}

	// A wrong key must not be able to read the snapshot.
	bad, err := sql.Open("sqlite3", buildDSN(path, "0000000000000000000000000000000000000000000000000000000000000000"))
	if err != nil {
		t.Fatalf("open with wrong key: %v", err)
	}
	defer bad.Close()
	if err := bad.Ping(); err == nil {
		t.Errorf("snapshot opened with wrong key; expected failure")
	}
}
