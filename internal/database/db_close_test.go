package database

import (
	"path/filepath"
	"testing"

	"clinic-api/internal/database/store"
)

func TestCloseClosesReadAndWritePools(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "close.db"))
	if err != nil {
		t.Fatal(err)
	}
	rdb := store.RDB

	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if store.DB != nil || store.RDB != nil {
		t.Fatal("database globals were not cleared")
	}
	if err := db.Ping(); err == nil {
		t.Fatal("write pool remained open")
	}
	if err := rdb.Ping(); err == nil {
		t.Fatal("read pool remained open")
	}
	if err := Close(); err != nil {
		t.Fatalf("second Close returned %v", err)
	}
}
