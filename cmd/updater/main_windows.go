//go:build windows

// Command updater (ZealUpdater.exe) is the Windows self-update swapper: it waits
// for the app to exit, backs up the exe + DB, swaps in the new build, health-
// checks it, and rolls back on failure. The app runs it from a throwaway copy.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"clinic-api/internal/updater/updatestate"

	"golang.org/x/sys/windows"
)

const (
	healthWindow = 30 * time.Second
	healthStreak = 3
)

func main() {
	if len(os.Args) < 2 {
		return
	}
	statePath := os.Args[1]

	if f, err := os.OpenFile(filepath.Join(filepath.Dir(statePath), "updater.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}

	st, err := updatestate.Read(statePath)
	if err != nil || st.Phase != updatestate.Applying {
		log.Printf("[updater] nothing to do (phase=%q err=%v)", st.Phase, err)
		return
	}
	log.Printf("[updater] start: %s -> %s (pid %d)", st.From, st.Target, st.PID)

	waitForExit(st.PID, 60*time.Second)

	if err := updatestate.CopyFile(st.DBPath, st.DBSnapshot); err != nil {
		finishFail(statePath, st, fmt.Errorf("snapshot db: %w", err))
		return
	}
	if err := updatestate.CopyFile(st.AppExe, st.BackupExe); err != nil {
		finishFail(statePath, st, fmt.Errorf("back up exe: %w", err))
		return
	}
	if err := updatestate.CopyFile(filepath.Join(st.StagingDir, "ZealClinic.exe"), st.AppExe); err != nil {
		_ = updatestate.CopyFile(st.BackupExe, st.AppExe)
		relaunch(st.AppExe)
		finishFail(statePath, st, fmt.Errorf("swap exe: %w", err))
		return
	}

	cmd, err := launch(st.AppExe)
	if err != nil {
		rollback(st)
		relaunch(st.AppExe)
		finishFail(statePath, st, fmt.Errorf("launch new build: %w", err))
		return
	}

	if waitHealthy(st.Port, st.Target) {
		succeed(statePath, st)
		return
	}

	log.Printf("[updater] new build unhealthy; rolling back")
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	rollback(st)
	relaunch(st.AppExe)
	finishFail(statePath, st, errors.New("new build failed health check"))
}

// succeed swaps in the new updater and cleans up, leaving only runner.exe
// (itself) for the app to remove on its next boot.
func succeed(statePath string, st updatestate.State) {
	updaterDst := filepath.Join(filepath.Dir(st.AppExe), "ZealUpdater.exe")
	if err := updatestate.CopyFile(filepath.Join(st.StagingDir, "ZealUpdater.exe"), updaterDst); err != nil {
		log.Printf("[updater] swap updater: %v", err)
	}
	st.Phase = updatestate.Success
	_ = updatestate.Write(statePath, st)
	_ = os.Remove(st.BackupExe)
	_ = os.Remove(st.DBSnapshot)
	_ = os.RemoveAll(st.StagingDir)
	_ = os.Remove(st.ZipPath)
	log.Printf("[updater] %s applied", st.Target)
}

func finishFail(statePath string, st updatestate.State, cause error) {
	log.Printf("[updater] failed: %v", cause)
	st.Phase = updatestate.Failed
	st.Error = cause.Error()
	_ = updatestate.Write(statePath, st)
}

func rollback(st updatestate.State) {
	if err := updatestate.CopyFile(st.BackupExe, st.AppExe); err != nil {
		log.Printf("[updater] restore exe: %v", err)
	}
	if err := updatestate.CopyFile(st.DBSnapshot, st.DBPath); err != nil {
		log.Printf("[updater] restore db: %v", err)
	}
	_ = os.Remove(st.DBPath + "-wal")
	_ = os.Remove(st.DBPath + "-shm")
}

func launch(appExe string) (*exec.Cmd, error) {
	cmd := exec.Command(appExe, "--post-update")
	cmd.Dir = filepath.Dir(appExe)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func relaunch(appExe string) {
	if _, err := launch(appExe); err != nil {
		log.Printf("[updater] relaunch previous build: %v", err)
	}
}

func waitHealthy(port, target string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(healthWindow)
	streak := 0
	for time.Now().Before(deadline) {
		if probeVersion(client, port) == target {
			if streak++; streak >= healthStreak {
				return true
			}
		} else {
			streak = 0
		}
		time.Sleep(time.Second)
	}
	return false
}

func probeVersion(client *http.Client, port string) string {
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Version
}

func waitForExit(pid int, timeout time.Duration) {
	if pid <= 0 {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
}
