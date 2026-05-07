package database

import (
	"clinic-api/internal/config"
	"clinic-api/internal/database/store"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
)

func Open(pathOverride string) (*sql.DB, error) {
	path := pathOverride
	if path == "" {
		path = defaultDBPath()
	}
	dbPath := resolveSQLitePath(path)
	dsn := buildDSN(dbPath, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	// Write connection: single conn, serialises all writes.
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open write db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Read connection: multiple conns, concurrent reads via WAL.
	rdb, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read db: %w", err)
	}
	rdb.SetMaxOpenConns(4)

	if err := rdb.Ping(); err != nil {
		return nil, fmt.Errorf("ping read db: %w", err)
	}

	store.DB = db
	store.RDB = rdb

	log.Println("Database initialized at", dbPath)
	return db, nil
}

// buildDSN composes a file: URI for the ncruces sqlite driver with the
// adiantum VFS for at-rest encryption. hexKey must be 64 hex chars (32 bytes).
// PRAGMA order matters per ncruces docs: busy_timeout before journal_mode.
// Format follows ncruces' own examples ("file:" + slash-converted path)
// rather than RFC-style "file:///C:/...", because ncruces' VFS Abs()-resolves
// the path verbatim and won't strip the leading slash on Windows.
func buildDSN(dbPath, hexKey string) string {
	if dbPath == ":memory:" || strings.HasPrefix(strings.ToLower(dbPath), "file:") {
		return dbPath
	}
	return "file:" + filepath.ToSlash(dbPath) +
		"?vfs=adiantum" +
		"&hexkey=" + hexKey +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)"
}

// defaultDBPath picks where clinic.db lives when launched: %PROGRAMDATA%\Zeal Clinic\clinic.db
// if that directory exists (installed layout; the Inno Setup script creates it), otherwise
// clinic.db relative to cwd (dev fallback).
func defaultDBPath() string {
	if dir := config.SharedDataDir(); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return filepath.Join(dir, "clinic.db")
		}
	}
	return "clinic.db"
}

func resolveSQLitePath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	lower := strings.ToLower(trimmed)

	if trimmed == ":memory:" || strings.HasPrefix(lower, "file:") {
		return trimmed
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		log.Fatal(err.Error())
	}

	parentDir := filepath.Dir(absPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		log.Fatal(err.Error())
	}

	return absPath
}
