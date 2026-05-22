//go:build !windows

package updater

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"clinic-api/internal/database/store"
)

// applyUpdate swaps the running binary in place and re-execs it, keeping the old
// binary as <exe>.old for rollback (removed by FinalizeOnBoot).
func applyUpdate(newBin string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	backup := exe + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(exe, backup); err != nil {
		return fmt.Errorf("back up current binary: %w", err)
	}
	if err := replaceFile(newBin, exe); err != nil {
		_ = os.Rename(backup, exe) // roll back
		return fmt.Errorf("install new binary: %w", err)
	}
	if err := os.Chmod(exe, 0700); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	// Exec skips defers, so checkpoint WAL by closing the DB handles first.
	if store.DB != nil {
		_ = store.DB.Close()
	}
	if store.RDB != nil {
		_ = store.RDB.Close()
	}

	args := append([]string{exe}, postUpdateArgs()...)
	return syscall.Exec(exe, args, os.Environ())
}

// replaceFile renames src over dst, falling back to copy+remove across devices.
func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}
