// Package updater implements health-gated self-update with rollback. Windows
// hands off to an external swapper (cmd/updater); Linux swaps in place and
// re-execs (trial boot). Recovery state lives in updatestate, not the DB.
package updater

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
	"clinic-api/internal/updater/updatestate"
)

var (
	ErrAlreadyInstalling = errors.New("update already in progress")
	ErrNoUpdate          = errors.New("no update available")
)

// installing gates the API while this node downloads and stages a build.
var installing atomic.Bool

func IsInstalling() bool { return installing.Load() }

var dlClient = &http.Client{Timeout: 10 * time.Minute}

func Platform() string { return runtime.GOOS }

type Status struct {
	Available  bool   `json:"available"`
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	ReleasedAt string `json:"releasedAt"`
	Installing bool   `json:"installing"`
}

func GetStatus() (Status, error) {
	st := Status{Current: buildmode.Version, Installing: IsInstalling()}
	v, err := store.LatestVersion(Platform())
	if err != nil {
		return st, err
	}
	if v.Version == "" {
		return st, nil
	}
	st.Latest = v.Version
	st.ReleasedAt = v.CreatedAt.String()
	st.Available = v.Version != buildmode.Version
	return st, nil
}

// Start runs the download + apply asynchronously; apply normally ends the process.
func Start() error {
	v, err := store.LatestVersion(Platform())
	if err != nil {
		return err
	}
	if v.Version == "" || v.Version == buildmode.Version {
		return ErrNoUpdate
	}
	if !installing.CompareAndSwap(false, true) {
		return ErrAlreadyInstalling
	}
	log.Printf("[update] starting update %s -> %s (%s)", buildmode.Version, v.Version, v.Platform)
	go run(v)
	return nil
}

func run(v store.Version) {
	defer tracking.Recover()

	if v.SHA256 == "" {
		fail("verify", errors.New("missing sha256"))
		return
	}
	if !strings.HasPrefix(v.URL, "https://") {
		fail("download", errors.New("update url must be https"))
		return
	}
	zipPath, err := download(v.URL)
	if err != nil {
		fail("download", err)
		return
	}
	if err := verifySHA256(zipPath, v.SHA256); err != nil {
		fail("verify", err)
		return
	}
	if err := unzip(zipPath, stagingDir()); err != nil {
		fail("unzip", err)
		return
	}

	exe, err := appExePath()
	if err != nil {
		fail("apply", err)
		return
	}
	st := updatestate.State{
		Phase:      updatestate.Applying,
		From:       buildmode.Version,
		Target:     v.Version,
		AppExe:     exe,
		DBPath:     dbPath(),
		StagingDir: stagingDir(),
		BackupExe:  exe + ".bak",
		DBSnapshot: dbPath() + ".snap",
		ZipPath:    zipPath,
		Port:       config.Current().Port,
		PID:        os.Getpid(),
	}
	if err := updatestate.Write(statePath(), st); err != nil {
		fail("state", err)
		return
	}

	log.Printf("[update] staged %s, applying", v.Version)
	if err := applyUpdate(st); err != nil {
		fail("apply", err)
		cleanupArtifacts(st)
		_ = updatestate.Clear(statePath())
		recoverFromFailedApply() // linux exits (systemd restarts); windows no-op
	}
}

func fail(stage string, err error) {
	log.Printf("[update] %s failed: %v", stage, err)
	tracking.CaptureError(nil, fmt.Errorf("[update] %s: %w", stage, err))
	installing.Store(false)
}

// ReconcileBoot (pre-DB, linux) promotes a trial boot or rolls back an unconfirmed one. No-op on Windows.
func ReconcileBoot() { reconcileBoot() }

// ConfirmStartup (post-server, linux) marks a surviving trial build healthy. No-op on Windows.
func ConfirmStartup() { confirmStartup() }

// FinalizeOnBoot reconciles a finished/interrupted update from the state file.
// postUpdate means the swapper just relaunched us, so a still-Applying state is normal.
func FinalizeOnBoot(postUpdate bool) {
	sp := statePath()
	st, err := updatestate.Read(sp)
	if err != nil {
		log.Printf("[update] read state: %v", err)
		return
	}
	switch st.Phase {
	case updatestate.Success:
		log.Printf("[update] update to %s applied", buildmode.Version)
		tracking.Info(nil, "[update] applied "+buildmode.Version)
		cleanupArtifacts(st)
		_ = updatestate.Clear(sp)
	case updatestate.Failed:
		log.Printf("[update] update to %s failed, rolled back to %s: %s", st.Target, st.From, st.Error)
		tracking.CaptureError(nil, fmt.Errorf("[update] rolled back to %s after failed update to %s: %s", st.From, st.Target, st.Error))
		cleanupArtifacts(st)
		_ = updatestate.Clear(sp)
	case updatestate.Applying:
		// Applying on a non-post-update boot means the swapper died mid-update; reconcile so we don't stick.
		if postUpdate {
			return
		}
		if buildmode.Version == st.Target {
			log.Printf("[update] recovered interrupted update to %s", buildmode.Version)
			tracking.Info(nil, "[update] applied "+buildmode.Version+" (recovered)")
		} else {
			log.Printf("[update] update to %s interrupted; still on %s", st.Target, buildmode.Version)
			tracking.CaptureError(nil, fmt.Errorf("[update] interrupted update to %s, running %s", st.Target, buildmode.Version))
		}
		cleanupArtifacts(st)
		_ = updatestate.Clear(sp)
	}
}

func cleanupArtifacts(st updatestate.State) {
	for _, p := range []string{st.BackupExe, st.DBSnapshot, st.ZipPath, runnerPath()} {
		if p != "" {
			_ = os.Remove(p)
		}
	}
	if st.StagingDir != "" {
		_ = os.RemoveAll(st.StagingDir)
	}
}

// checkpointAndCloseDB flushes the WAL and closes the DB so the snapshot is consistent.
func checkpointAndCloseDB() {
	if err := database.Close(); err != nil {
		log.Printf("[update] close database: %v", err)
		tracking.CaptureError(nil, fmt.Errorf("[update] close database: %w", err))
	}
}

func downloadDir() string {
	dir := filepath.Join(config.DataDir(), "update")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

func statePath() string  { return filepath.Join(downloadDir(), "state.json") }
func stagingDir() string { return filepath.Join(downloadDir(), "staging") }
func runnerPath() string { return filepath.Join(downloadDir(), "runner.exe") }
func dbPath() string     { return filepath.Join(config.DataDir(), "clinic.db") }

func appExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

func download(url string) (string, error) {
	dest := filepath.Join(downloadDir(), "ZealClinicUpdate.zip")

	resp, err := dlClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download status %d", resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return dest, nil
}

func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}

// unzip extracts a flat archive into a freshly emptied dest.
func unzip(src, dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0700); err != nil {
		return err
	}
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := extractZipFile(f, filepath.Join(dest, filepath.Base(f.Name))); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func postUpdateArgs() []string {
	args := append([]string(nil), os.Args[1:]...)
	for _, a := range args {
		if a == "--post-update" {
			return args
		}
	}
	return append(args, "--post-update")
}
