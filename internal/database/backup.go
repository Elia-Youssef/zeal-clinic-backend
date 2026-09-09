package database

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clinic-api/internal/database/store"
)

func Snapshot(dir string) (string, error) {
	key, err := currentKey()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	// Nanoseconds avoid a collision when a manual restore starts in the same
	// second as the periodic backup monitor.
	dest := filepath.Join(dir, "clinic-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".db")
	target := "file:" + filepath.ToSlash(dest) + "?vfs=adiantum&hexkey=" + key
	stmt := "VACUUM INTO '" + strings.ReplaceAll(target, "'", "''") + "'"
	if _, err := store.DB.Exec(stmt); err != nil {
		return "", fmt.Errorf("vacuum into backup: %w", err)
	}
	return dest, nil
}

// RestoreSnapshot replaces the database file with a standalone snapshot.
// All pools and maintenance connections for target must already be closed.
func RestoreSnapshot(snapshot, target string) error {
	in, err := os.Open(snapshot)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer in.Close()

	tmp := target + ".restore-tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create restore temp: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy snapshot: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("sync restore temp: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close restore temp: %w", err)
	}

	_ = os.Remove(target + "-wal")
	_ = os.Remove(target + "-shm")
	// Windows does not replace an existing destination with os.Rename. The
	// cloud build runs on Linux, but removing first keeps this helper portable.
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return fmt.Errorf("remove damaged database: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install snapshot: %w", err)
	}
	return nil
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
