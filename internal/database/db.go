package database

import (
	"clinic-api/internal/database/store"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func Open(rawPath string) (*sql.DB, error) {
	dbPath := resolveSQLitePath(rawPath)
	dsn := dbPath + "?_foreign_keys=1&_journal_mode=WAL&_busy_timeout=5000"

	// Write connection: single conn, serialises all writes.
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open write db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
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
