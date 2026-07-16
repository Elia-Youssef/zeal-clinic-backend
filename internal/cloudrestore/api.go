// Package cloudrestore implements the explicit, local-authoritative disaster
// recovery path. It is deliberately separate from incremental sync: both
// nodes enter maintenance, local uploads a consistent encrypted snapshot, and
// cloud replaces only the tables declared syncable.
package cloudrestore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
	"clinic-api/internal/updater"

	"github.com/labstack/echo/v4"
)

const (
	maxDumpBytes   = int64(2 << 30) // 2 GiB
	restoreTimeout = 30 * time.Minute
)

type Config struct {
	PeerURL    string
	Secret     string
	Invalidate func()
}

type API struct {
	cfg    Config
	gate   *maintenanceGate
	client *http.Client
}

type response struct {
	Success bool   `json:"success,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func New(cfg Config) *API {
	cfg.PeerURL = strings.TrimRight(cfg.PeerURL, "/")
	return &API{
		cfg:    cfg,
		gate:   newMaintenanceGate(),
		client: &http.Client{Timeout: restoreTimeout},
	}
}

func (a *API) Middleware() echo.MiddlewareFunc { return a.gate.middleware() }

// HandleLocal is the JWT/scope-protected endpoint mounted only by local builds.
func (a *API) HandleLocal(c echo.Context) error {
	if buildmode.Cloud {
		return c.JSON(http.StatusNotFound, response{Error: "Not found"})
	}
	if a.cfg.PeerURL == "" || a.cfg.Secret == "" {
		return c.JSON(http.StatusConflict, response{Error: "Cloud sync is not configured"})
	}
	if err := a.gate.begin(); err != nil {
		return c.JSON(http.StatusConflict, response{Error: err.Error()})
	}
	reportLocalProgress(1, "running", "preparing", "Preparing both instances")
	if updater.IsInstalling() {
		reportLocalProgress(1, "failed", "failed", "Cloud restore could not start")
		a.gate.end()
		return c.JSON(http.StatusConflict, response{Error: "An update started before maintenance mode was acquired"})
	}
	runtime := currentRuntime()
	if runtime != nil {
		runtime.Pause()
	}
	released := false
	release := func() {
		if released {
			return
		}
		if runtime != nil {
			runtime.Resume(store.DB)
		}
		a.gate.end()
		released = true
	}
	defer release()

	result, err := a.restoreCloud(c.Request().Context())
	if err != nil {
		log.Printf("[cloud-restore] local restore failed: %v", err)
		reportProgress("failed", "failed", "Cloud restore did not complete; verify cloud status before retrying")
		return c.JSON(http.StatusBadGateway, response{Error: err.Error()})
	}
	if a.cfg.Invalidate != nil {
		a.cfg.Invalidate()
	}
	release()
	realtime.Broadcast(realtime.Event{Type: "data_changed"})
	reportLocalProgress(5, "success", "completed", "Cloud restore completed")
	log.Printf("[cloud-restore] completed restore=%s rows=%d", result.RestoreID, result.Rows)
	return c.JSON(http.StatusOK, response{Success: true, Data: result})
}

func (a *API) restoreCloud(ctx context.Context) (ApplyResult, error) {
	reportLocalProgress(2, "running", "exporting", "Creating a local database snapshot")
	dir := filepath.Join(config.DataDir(), "cloud-restore", "outgoing")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ApplyResult{}, fmt.Errorf("create export directory: %w", err)
	}
	exportDir, err := os.MkdirTemp(dir, "restore-")
	if err != nil {
		return ApplyResult{}, fmt.Errorf("create export staging: %w", err)
	}
	defer os.RemoveAll(exportDir)
	dumpPath, err := database.Snapshot(exportDir)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("snapshot local database: %w", err)
	}
	dump, err := database.OpenStandalone(dumpPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("open local snapshot: %w", err)
	}
	localBaseline, baselineErr := outboxHighWater(dump)
	closeErr := dump.Close()
	if baselineErr != nil {
		return ApplyResult{}, fmt.Errorf("read local snapshot baseline: %w", baselineErr)
	}
	if closeErr != nil {
		return ApplyResult{}, fmt.Errorf("close local snapshot: %w", closeErr)
	}
	checksum, err := fileSHA256(dumpPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("hash local snapshot: %w", err)
	}
	info, err := os.Stat(dumpPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("stat local snapshot: %w", err)
	}
	file, err := os.Open(dumpPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("open local snapshot: %w", err)
	}
	defer file.Close()

	restoreID, err := newRestoreID()
	if err != nil {
		return ApplyResult{}, fmt.Errorf("create restore id: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.PeerURL+"/api/cloud-restore", file)
	if err != nil {
		return ApplyResult{}, err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Sync-Secret", a.cfg.Secret)
	req.Header.Set("X-Sync-Version", buildmode.Version)
	req.Header.Set("X-Restore-SHA256", checksum)
	req.Header.Set("X-Restore-ID", restoreID)

	reportUploadProgress(3, info.Size())
	peerResponse, err := a.client.Do(req)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("upload cloud restore: %w", err)
	}
	defer peerResponse.Body.Close()
	var envelope struct {
		Success bool        `json:"success"`
		Data    ApplyResult `json:"data"`
		Error   string      `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(peerResponse.Body, 1<<20)).Decode(&envelope); err != nil {
		return ApplyResult{}, fmt.Errorf("decode cloud restore response (status %d): %w", peerResponse.StatusCode, err)
	}
	if peerResponse.StatusCode != http.StatusOK || !envelope.Success {
		if envelope.Error == "" {
			envelope.Error = fmt.Sprintf("cloud status %d", peerResponse.StatusCode)
		}
		return ApplyResult{}, errors.New(envelope.Error)
	}
	if envelope.Data.LocalBaseline != localBaseline {
		return ApplyResult{}, fmt.Errorf("cloud acknowledged local baseline %d, expected %d", envelope.Data.LocalBaseline, localBaseline)
	}
	reportLocalProgress(4, "running", "finalizing", "Finalizing local sync state")
	if err := FinalizeLocal(store.DB, localBaseline, envelope.Data.CloudBaseline); err != nil {
		return ApplyResult{}, fmt.Errorf("finalize local sync state: %w", err)
	}
	return envelope.Data, nil
}

// HandleCloud is key-authenticated and mounted only by cloud builds.
func (a *API) HandleCloud(c echo.Context) error {
	if !buildmode.Cloud {
		return c.JSON(http.StatusNotFound, response{Error: "Not found"})
	}
	if !a.authorized(c.Request()) {
		return c.JSON(http.StatusUnauthorized, response{Error: "Not authorized"})
	}
	if peerVersion := c.Request().Header.Get("X-Sync-Version"); peerVersion != buildmode.Version {
		return c.JSON(http.StatusConflict, response{Error: "Version mismatch"})
	}
	if c.Request().ContentLength > maxDumpBytes {
		return c.JSON(http.StatusRequestEntityTooLarge, response{Error: "Database dump is too large"})
	}
	expectedChecksum := c.Request().Header.Get("X-Restore-SHA256")
	if len(expectedChecksum) != sha256.Size*2 {
		return c.JSON(http.StatusBadRequest, response{Error: "Missing or invalid dump checksum"})
	}
	restoreID := c.Request().Header.Get("X-Restore-ID")
	if restoreID == "" || len(restoreID) > 128 {
		return c.JSON(http.StatusBadRequest, response{Error: "Missing or invalid restore id"})
	}

	if err := a.gate.begin(); err != nil {
		return c.JSON(http.StatusConflict, response{Error: err.Error()})
	}
	reportCloudProgress(1, "running", "receiving", "Receiving the local database snapshot")
	if updater.IsInstalling() {
		reportCloudProgress(1, "failed", "failed", "Cloud restore could not start")
		a.gate.end()
		return c.JSON(http.StatusConflict, response{Error: "An update started before maintenance mode was acquired"})
	}
	runtime := currentRuntime()
	if runtime != nil {
		runtime.Pause()
	}
	healthy := true
	released := false
	release := func() {
		if released || !healthy {
			return
		}
		if runtime != nil {
			runtime.Resume(store.DB)
		}
		a.gate.end()
		released = true
	}
	defer release()

	result, operationErr, databaseHealthy := a.acceptDump(c, expectedChecksum, restoreID)
	healthy = databaseHealthy
	if operationErr != nil {
		log.Printf("[cloud-restore] cloud restore %s failed: %v", restoreID, operationErr)
		release()
		reportCloudProgress(6, "failed", "failed", "Cloud restore failed; check cloud logs before retrying")
		return c.JSON(http.StatusInternalServerError, response{Error: operationErr.Error()})
	}
	if a.cfg.Invalidate != nil {
		a.cfg.Invalidate()
	}
	release()
	realtime.Broadcast(realtime.Event{Type: "data_changed"})
	reportCloudProgress(6, "success", "completed", "Cloud restore completed")
	return c.JSON(http.StatusOK, response{Success: true, Data: result})
}

func (a *API) acceptDump(c echo.Context, expectedChecksum, restoreID string) (ApplyResult, error, bool) {
	stageDir := filepath.Join(config.DataDir(), "cloud-restore", "incoming")
	if err := os.MkdirAll(stageDir, 0700); err != nil {
		return ApplyResult{}, fmt.Errorf("create restore staging: %w", err), true
	}
	staged, err := os.CreateTemp(stageDir, "restore-*.db")
	if err != nil {
		return ApplyResult{}, fmt.Errorf("create staged dump: %w", err), true
	}
	stagedPath := staged.Name()
	defer func() {
		_ = os.Remove(stagedPath)
		_ = os.Remove(stagedPath + "-wal")
		_ = os.Remove(stagedPath + "-shm")
	}()
	hash := sha256.New()
	limited := http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxDumpBytes)
	_, copyErr := io.Copy(io.MultiWriter(staged, hash), limited)
	closeErr := staged.Close()
	if copyErr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(copyErr, &maxErr) {
			return ApplyResult{}, fmt.Errorf("database dump exceeds %s", byteCount(maxDumpBytes)), true
		}
		return ApplyResult{}, fmt.Errorf("receive database dump: %w", copyErr), true
	}
	if closeErr != nil {
		return ApplyResult{}, fmt.Errorf("close staged dump: %w", closeErr), true
	}
	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(actualChecksum)), []byte(strings.ToLower(expectedChecksum))) != 1 {
		return ApplyResult{}, fmt.Errorf("database dump checksum mismatch"), true
	}

	reportCloudProgress(2, "running", "validating", "Validating the uploaded snapshot")
	source, err := database.OpenStandalone(stagedPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("open uploaded dump: %w", err), true
	}
	if err := validateSnapshot(source, store.DB); err != nil {
		_ = source.Close()
		return ApplyResult{}, err, true
	}
	if err := source.Close(); err != nil {
		return ApplyResult{}, fmt.Errorf("close uploaded dump: %w", err), true
	}

	reportCloudProgress(3, "running", "backing_up", "Backing up the current cloud database")
	backup, err := database.Snapshot(config.BackupDir())
	if err != nil {
		return ApplyResult{}, fmt.Errorf("backup cloud database: %w", err), true
	}
	if err := database.Close(); err != nil {
		return ApplyResult{}, a.rollbackCloud(backup, fmt.Errorf("close cloud database: %w", err)), store.DB != nil
	}

	reportCloudProgress(4, "running", "applying", "Replacing cloud sync data from the local snapshot")
	result, applyErr := applyStagedDump(stagedPath)
	if applyErr != nil {
		rollbackErr := a.rollbackCloud(backup, applyErr)
		return ApplyResult{}, rollbackErr, store.DB != nil
	}
	reportCloudProgress(5, "running", "reopening", "Reopening the restored cloud database")
	if _, err := database.Open(""); err != nil {
		rollbackErr := a.rollbackCloud(backup, fmt.Errorf("reopen restored cloud database: %w", err))
		return ApplyResult{}, rollbackErr, store.DB != nil
	}
	result.Backup = filepath.Base(backup)
	result.RestoreID = restoreID
	log.Printf("[cloud-restore] applied restore=%s tables=%d rows=%d backup=%s", restoreID, result.Tables, result.Rows, backup)
	return result, nil, true
}

func applyStagedDump(stagedPath string) (ApplyResult, error) {
	target, err := database.OpenStandalone(database.DefaultPath())
	if err != nil {
		return ApplyResult{}, fmt.Errorf("open cloud database for restore: %w", err)
	}
	source, err := database.OpenStandalone(stagedPath)
	if err != nil {
		_ = target.Close()
		return ApplyResult{}, fmt.Errorf("reopen uploaded dump: %w", err)
	}
	result, applyErr := ApplySnapshot(target, source)
	if applyErr == nil {
		if _, err := target.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			applyErr = fmt.Errorf("checkpoint restored cloud database: %w", err)
		}
	}
	sourceCloseErr := source.Close()
	targetCloseErr := target.Close()
	if applyErr != nil {
		return ApplyResult{}, applyErr
	}
	if sourceCloseErr != nil {
		return ApplyResult{}, fmt.Errorf("close uploaded dump: %w", sourceCloseErr)
	}
	if targetCloseErr != nil {
		return ApplyResult{}, fmt.Errorf("close restored cloud database: %w", targetCloseErr)
	}
	return result, nil
}

func (a *API) rollbackCloud(backup string, cause error) error {
	reportCloudProgress(5, "running", "rolling_back", "Restoring the previous cloud database")
	if store.DB != nil || store.RDB != nil {
		_ = database.Close()
	}
	if err := database.RestoreSnapshot(backup, database.DefaultPath()); err != nil {
		return errors.Join(cause, fmt.Errorf("rollback cloud backup: %w", err))
	}
	if _, err := database.Open(""); err != nil {
		return errors.Join(cause, fmt.Errorf("reopen rolled-back cloud database: %w", err))
	}
	return fmt.Errorf("%w (cloud database rolled back)", cause)
}

func (a *API) authorized(request *http.Request) bool {
	provided := request.Header.Get("X-Sync-Secret")
	return a.cfg.Secret != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(a.cfg.Secret)) == 1
}

func newRestoreID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func byteCount(n int64) string {
	return strconv.FormatInt(n/(1<<20), 10) + " MiB"
}
