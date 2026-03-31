package database

import (
	"clinic-api/internal/database/models"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(rawPath string) (*sql.DB, error) {
	dbPath := resolveSQLitePath(rawPath)

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}

	models.DB = db

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
