package database

import (
	"clinic-api/internal/config"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
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

// buildDSN returns the encrypted SQLite URI. hexKey must be 64 hex chars.
func buildDSN(dbPath, hexKey string) string {
	if dbPath == ":memory:" || strings.HasPrefix(strings.ToLower(dbPath), "file:") {
		return dbPath
	}
	return "file:" + filepath.ToSlash(dbPath) +
		"?vfs=adiantum" +
		"&hexkey=" + hexKey +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=recursive_triggers(1)"
}

// defaultDBPath uses ProgramData when installed, otherwise ./tmp/clinic.db.
func defaultDBPath() string {
	return filepath.Join(config.DataDir(), "clinic.db")
}

func resolveSQLitePath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	lower := strings.ToLower(trimmed)

	if trimmed == ":memory:" || strings.HasPrefix(lower, "file:") {
		return trimmed
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		tracking.Fatal("resolve sqlite path", err)
	}

	parentDir := filepath.Dir(absPath)
	if err := os.MkdirAll(parentDir, 0700); err != nil {
		tracking.Fatal("create sqlite parent dir", err)
	}

	return absPath
}
