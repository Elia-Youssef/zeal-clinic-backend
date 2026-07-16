package database

import (
	"clinic-api/internal/config"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
)

const encryptionKey = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

var closeMu sync.Mutex

func Open(pathOverride string) (*sql.DB, error) {
	path := pathOverride
	if path == "" {
		path = defaultDBPath()
	}
	dbPath := resolveSQLitePath(path)
	dsn := buildDSN(dbPath, encryptionKey)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open write db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	rdb, err := sql.Open("sqlite3", dsn)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open read db: %w", err)
	}
	rdb.SetMaxOpenConns(4)

	if err := rdb.Ping(); err != nil {
		_ = rdb.Close()
		_ = db.Close()
		return nil, fmt.Errorf("ping read db: %w", err)
	}

	closeMu.Lock()
	store.DB = db
	store.RDB = rdb
	closeMu.Unlock()

	log.Println("Database initialized at", dbPath)
	return db, nil
}

// OpenStandalone opens one encrypted SQLite pool without running migrations or
// installing it into store.DB/store.RDB. It is intended for maintenance tasks
// that must keep the normal application pools closed while inspecting or
// repairing the database.
func OpenStandalone(path string) (*sql.DB, error) {
	dbPath := resolveSQLitePath(path)
	db, err := sql.Open("sqlite3", buildDSN(dbPath, encryptionKey))
	if err != nil {
		return nil, fmt.Errorf("open standalone db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping standalone db: %w", err)
	}
	return db, nil
}

// DefaultPath returns the database file used by the current build.
func DefaultPath() string { return defaultDBPath() }

// Close checkpoints SQLite and closes both the read and write connection
// pools installed by Open. It is safe to call more than once.
func Close() error {
	closeMu.Lock()
	rdb := store.RDB
	db := store.DB
	store.RDB = nil
	store.DB = nil
	closeMu.Unlock()

	var errs []error
	if rdb != nil {
		if err := rdb.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close read db: %w", err))
		}
	}
	if db != nil {
		if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil && err != sql.ErrConnDone {
			errs = append(errs, fmt.Errorf("checkpoint db: %w", err))
		}
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close write db: %w", err))
		}
	}
	return errors.Join(errs...)
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

// defaultDBPath uses the per-user LocalAppData Data\ dir when installed, otherwise ./tmp/clinic.db.
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
