package cloudrestore

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	syncpkg "clinic-api/internal/sync"
)

type columnInfo struct {
	Name    string
	Type    string
	NotNull int
	PK      int
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func schemaVersion(db *sql.DB) (int64, error) {
	var version sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version); err != nil {
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return version.Int64, nil
}

func outboxHighWater(db queryRower) (int64, error) {
	var seq int64
	err := db.QueryRow(`SELECT IFNULL((SELECT seq FROM sqlite_sequence WHERE name = 'sync_log'), 0)`).Scan(&seq)
	return seq, err
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func validateSnapshot(source, target *sql.DB) error {
	if err := checkDatabase(source); err != nil {
		return fmt.Errorf("uploaded dump: %w", err)
	}
	sourceVersion, err := schemaVersion(source)
	if err != nil {
		return fmt.Errorf("uploaded schema version: %w", err)
	}
	targetVersion, err := schemaVersion(target)
	if err != nil {
		return fmt.Errorf("cloud schema version: %w", err)
	}
	if sourceVersion != targetVersion {
		return fmt.Errorf("schema version mismatch: local=%d cloud=%d", sourceVersion, targetVersion)
	}

	for _, table := range syncpkg.SyncedTables {
		sourceCols, err := tableColumns(source, table.Name)
		if err != nil {
			return fmt.Errorf("uploaded table %s: %w", table.Name, err)
		}
		targetCols, err := tableColumns(target, table.Name)
		if err != nil {
			return fmt.Errorf("cloud table %s: %w", table.Name, err)
		}
		if !reflect.DeepEqual(sourceCols, targetCols) {
			return fmt.Errorf("table schema mismatch: %s", table.Name)
		}
	}
	return nil
}

func checkDatabase(db *sql.DB) error {
	var result string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
		return fmt.Errorf("quick_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("quick_check: %s", result)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("foreign_key_check reported a violation")
	}
	return rows.Err()
}

func tableColumns(db *sql.DB, table string) ([]columnInfo, error) {
	rows, err := db.Query(`PRAGMA table_info(` + quoteIdent(table) + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []columnInfo
	for rows.Next() {
		var cid int
		var c columnInfo
		var defaultValue any
		if err := rows.Scan(&cid, &c.Name, &c.Type, &c.NotNull, &defaultValue, &c.PK); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("table is missing")
	}
	return out, nil
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
