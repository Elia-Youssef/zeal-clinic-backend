//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"clinic-api/internal/updater/updatestate"
)

// applyUpdate launches the swapper from a throwaway copy (so it can overwrite the
// real ZealUpdater.exe), then closes the DB and exits to unlock our files. The
// swapper blocks on our PID, so anything that can fail does so before the DB close.
func applyUpdate(st updatestate.State) error {
	updaterSrc := filepath.Join(filepath.Dir(st.AppExe), "ZealUpdater.exe")
	if err := updatestate.CopyFile(updaterSrc, runnerPath()); err != nil {
		return fmt.Errorf("stage updater: %w", err)
	}

	cmd := exec.Command(runnerPath(), statePath())
	cmd.Dir = filepath.Dir(st.AppExe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch updater: %w", err)
	}

	checkpointAndCloseDB()
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func reconcileBoot()          {}
func confirmStartup()         {}
func recoverFromFailedApply() {}
