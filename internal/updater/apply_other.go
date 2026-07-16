//go:build !windows

package updater

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/tracking"
	"clinic-api/internal/updater/updatestate"
)

const confirmDelay = 12 * time.Second

// applyUpdate snapshots the DB, swaps the binary (keeping a .bak), and re-execs into the trial boot.
func applyUpdate(st updatestate.State) error {
	exe := st.AppExe
	newBin := filepath.Join(st.StagingDir, "ZealClinic")

	checkpointAndCloseDB()
	if err := updatestate.CopyFile(st.DBPath, st.DBSnapshot); err != nil {
		return fmt.Errorf("snapshot db: %w", err)
	}

	_ = os.Remove(st.BackupExe)
	if err := os.Rename(exe, st.BackupExe); err != nil {
		return fmt.Errorf("back up current binary: %w", err)
	}
	if err := updatestate.CopyFile(newBin, exe); err != nil {
		_ = os.Rename(st.BackupExe, exe)
		return fmt.Errorf("install new binary: %w", err)
	}
	if err := os.Chmod(exe, 0700); err != nil {
		_ = os.Rename(st.BackupExe, exe)
		return fmt.Errorf("chmod new binary: %w", err)
	}

	args := append([]string{exe}, postUpdateArgs()...)
	return syscall.Exec(exe, args, os.Environ())
}

// recoverFromFailedApply exits so systemd restarts clean; reopening the closed
// pools in-process would strand the sync engine on a dead handle.
func recoverFromFailedApply() { os.Exit(1) }

func reconcileBoot() {
	sp := statePath()
	st, err := updatestate.Read(sp)
	if err != nil {
		log.Printf("[update] read state: %v", err)
		return
	}
	switch st.Phase {
	case updatestate.Applying:
		if buildmode.Version == st.Target {
			st.Phase = updatestate.Trial
			_ = updatestate.Write(sp, st)
			log.Printf("[update] trial boot of %s", buildmode.Version)
		}
	case updatestate.Trial:
		log.Printf("[update] %s failed to confirm; rolling back to %s", st.Target, st.From)
		rollback(st)
		st.Phase = updatestate.Failed
		st.Error = "trial boot did not confirm healthy"
		_ = updatestate.Write(sp, st)
		args := append([]string{st.AppExe}, postUpdateArgs()...)
		if err := syscall.Exec(st.AppExe, args, os.Environ()); err != nil {
			log.Printf("[update] re-exec after rollback failed: %v", err)
		}
	}
}

func rollback(st updatestate.State) {
	if st.BackupExe != "" {
		// Rename, not copy: can't open the running exe for write (ETXTBSY).
		if err := os.Rename(st.BackupExe, st.AppExe); err != nil {
			log.Printf("[update] restore binary: %v", err)
		}
	}
	if st.DBSnapshot != "" {
		if err := updatestate.CopyFile(st.DBSnapshot, st.DBPath); err != nil {
			log.Printf("[update] restore db: %v", err)
		}
		_ = os.Remove(st.DBPath + "-wal")
		_ = os.Remove(st.DBPath + "-shm")
	}
}

func confirmStartup() {
	sp := statePath()
	st, err := updatestate.Read(sp)
	if err != nil || st.Phase != updatestate.Trial {
		return
	}
	time.Sleep(confirmDelay)
	if !selfHealthy(st.Port) {
		log.Printf("[update] trial build unhealthy after %s; exiting to roll back", confirmDelay)
		os.Exit(1) // systemd restarts it and reconcileBoot rolls back
	}
	st.Phase = updatestate.Success
	_ = updatestate.Write(sp, st)
	_ = os.Remove(st.BackupExe)
	_ = os.Remove(st.DBSnapshot)
	_ = updatestate.Clear(sp)
	installing.Store(false)
	log.Printf("[update] update to %s confirmed", buildmode.Version)
	tracking.Info(nil, "[update] applied "+buildmode.Version)
}

func selfHealthy(port string) bool {
	if port == "" {
		return true
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for range 5 {
		resp, err := client.Get("http://127.0.0.1:" + port + "/health")
		if err == nil {
			ok := resp.StatusCode == http.StatusOK
			resp.Body.Close()
			if ok {
				return true
			}
		}
		time.Sleep(time.Second)
	}
	return false
}
