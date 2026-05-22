// Package updater implements in-place self-update for the local (Windows) and
// cloud (Linux) builds. New builds are advertised in the synced `versions`
// table; applying one downloads the artifact, verifies its SHA-256, and hands
// off to the platform apply step (Windows: run the installer; Linux: swap the
// binary and re-exec).
package updater

import (
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
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
)

var (
	ErrAlreadyInstalling = errors.New("update already in progress")
	ErrNoUpdate          = errors.New("no update available")
)

// installing gates the API surface while this node downloads and swaps its binary.
var installing atomic.Bool

func IsInstalling() bool { return installing.Load() }

var dlClient = &http.Client{Timeout: 10 * time.Minute}

// Platform matches versions.platform for this build.
func Platform() string { return runtime.GOOS }

type Status struct {
	Available  bool   `json:"available"`
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	ReleasedAt string `json:"releasedAt"`
	Installing bool   `json:"installing"`
}

// GetStatus reports whether a newer build is advertised for this platform.
// Dev builds never report an update.
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

// Start raises the gate, records the target, and runs the download + apply
// asynchronously. The apply step normally ends the process.
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
	if err := store.SetInstalling(v.Version); err != nil {
		installing.Store(false)
		return err
	}
	log.Printf("[update] starting update %s -> %s (%s)", buildmode.Version, v.Version, v.Platform)
	go run(v)
	return nil
}

func run(v store.Version) {
	defer tracking.Recover()

	dest, err := download(v.URL)
	if err != nil {
		fail("download", err)
		return
	}
	if v.SHA256 != "" {
		if err := verifySHA256(dest, v.SHA256); err != nil {
			fail("verify", err)
			return
		}
	}
	log.Printf("[update] downloaded %s, applying", v.Version)
	if err := applyUpdate(dest); err != nil {
		fail("apply", err)
	}
}

// fail clears the flag and reopens the API after an aborted update.
func fail(stage string, err error) {
	log.Printf("[update] %s failed: %v", stage, err)
	tracking.CaptureError(nil, fmt.Errorf("[update] %s: %w", stage, err))
	if cerr := store.ClearInstalling(); cerr != nil {
		log.Printf("[update] clear installing flag: %v", cerr)
	}
	installing.Store(false)
}

// FinalizeOnBoot confirms an update after a restart and clears the flag. It
// always clears, so a failed swap can't leave the API gated.
func FinalizeOnBoot() {
	st, err := store.GetAppState()
	if err != nil {
		log.Printf("[update] read app_state: %v", err)
		return
	}
	if !st.Installing {
		return
	}
	if buildmode.Version == st.TargetVersion {
		log.Printf("[update] update to %s applied", buildmode.Version)
		tracking.Info(nil, "[update] applied "+buildmode.Version)
	} else {
		tracking.CaptureError(nil, fmt.Errorf("[update] post-update mismatch: running %s, expected %s", buildmode.Version, st.TargetVersion))
	}
	if err := store.ClearInstalling(); err != nil {
		log.Printf("[update] clear installing flag: %v", err)
	}
	cleanupBackups()
}

func cleanupBackups() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

func downloadDir() string {
	dir := filepath.Join(config.DataDir(), "tmp")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

func download(url string) (string, error) {
	name := "ZealClinic.new"
	if Platform() == "windows" {
		name = "ZealClinicSetup.exe"
	}
	dest := filepath.Join(downloadDir(), name)

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

// postUpdateArgs returns the relaunch args with --post-update ensured.
func postUpdateArgs() []string {
	args := append([]string(nil), os.Args[1:]...)
	for _, a := range args {
		if a == "--post-update" {
			return args
		}
	}
	return append(args, "--post-update")
}
