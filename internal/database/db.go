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

var closeMu sync.Mutex

// The at-rest encryption key of every database this package opens or writes
// (Open, OpenStandalone, Snapshot), set once at startup by SetKey.
var (
	keyMu  sync.RWMutex
	dbKey  string
	errKey = errors.New("database key not set")
)

// SetKey sets the at-rest encryption key: 64 hex characters (32 bytes). The
// server takes it from the config (DB_ENCRYPTION_KEY); tests use their own.
// The error never contains the key.
func SetKey(hexKey string) error {
	if !isHexKey(hexKey) {
		return errors.New("database key must be 64 hex characters")
	}
	keyMu.Lock()
	dbKey = hexKey
	keyMu.Unlock()
	return nil
}

func currentKey() (string, error) {
	keyMu.RLock()
	defer keyMu.RUnlock()
	if dbKey == "" {
		return "", errKey
	}
	return dbKey, nil
}

func isHexKey(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func Open(pathOverride string) (*sql.DB, error) {
	key, err := currentKey()
	if err != nil {
		return nil, err
	}
	path := pathOverride
	if path == "" {
		path = defaultDBPath()
	}
	dbPath := resolveSQLitePath(path)
	dsn := buildDSN(dbPath, key)

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
	key, err := currentKey()
	if err != nil {
		return nil, err
	}
	dbPath := resolveSQLitePath(path)
	db, err := sql.Open("sqlite3", buildDSN(dbPath, key))
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
