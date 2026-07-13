package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clinic-api/internal/database/store"
)

func Snapshot(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	dest := filepath.Join(dir, "clinic-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	target := "file:" + filepath.ToSlash(dest) + "?vfs=adiantum&hexkey=" + encryptionKey
	stmt := "VACUUM INTO '" + strings.ReplaceAll(target, "'", "''") + "'"
	if _, err := store.DB.Exec(stmt); err != nil {
		return "", fmt.Errorf("vacuum into backup: %w", err)
	}
	return dest, nil
}

// LiveModTime is the newest mtime across the live DB file and its WAL — i.e.
// when the database was last written, by any path (HTTP, monitor, sync-apply).
// The backup job uses it to skip snapshots when nothing has changed.
func LiveModTime() (time.Time, error) {
	base := defaultDBPath()
	newest, err := os.Stat(base)
	if err != nil {
		return time.Time{}, err
	}
	t := newest.ModTime()
	if wal, err := os.Stat(base + "-wal"); err == nil && wal.ModTime().After(t) {
		t = wal.ModTime()
	}
	return t, nil
}
