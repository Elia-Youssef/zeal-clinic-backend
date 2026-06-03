package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"
	"clinic-api/internal/database/store"
	syncpkg "clinic-api/internal/sync"
	"clinic-api/internal/updater"

	"github.com/labstack/echo/v4"
)

func GetUpdateStatus(c echo.Context) error {
	st, err := updater.GetStatus()
	if err != nil {
		log.Println("Error: [GetUpdateStatus] failed to read update status:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't check for updates"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: st})
}

// StartUpdate updates this node, and first triggers the cloud peer (releases
// target both). A failed peer trigger is a warning, not a stop.
func StartUpdate(c echo.Context) error {
	cfg := config.Current()

	// Flush while both still share a version: once either updates, push/pull are
	// version-gated, so both must start from a synced slate. Abort if it fails.
	if err := syncpkg.RunNow(c.Request().Context()); err != nil {
		log.Println("Error: [StartUpdate] pre-update sync failed:", err)
		return c.JSON(http.StatusBadGateway, httpx.Response{Error: "Sync failed, update canceled"})
	}

	var peerWarn string
	if cfg.PeerURL != "" {
		if err := triggerPeerUpdate(cfg); err != nil {
			peerWarn = "cloud update trigger failed: " + err.Error()
			log.Println("Error: [StartUpdate]", peerWarn)
		}
	}

	if err := updater.Start(); err != nil {
		switch {
		case errors.Is(err, updater.ErrNoUpdate):
			return c.JSON(http.StatusConflict, httpx.Response{Error: "No update available"})
		case errors.Is(err, updater.ErrAlreadyInstalling):
			return c.JSON(http.StatusConflict, httpx.Response{Error: "Update already in progress"})
		default:
			log.Println("Error: [StartUpdate] failed to start update:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't start update"})
		}
	}

	data := map[string]any{"status": "installing"}
	if peerWarn != "" {
		data["warning"] = peerWarn
	}
	return c.JSON(http.StatusAccepted, httpx.Response{Success: true, Data: data})
}

// PeerStartUpdate is the cloud-only endpoint a local node calls to update the cloud.
// If the cloud has no update but the caller is on a different version, report
// success: sync is version-gated, so the caller proceeding to its own update
// is enough to reconcile.
func PeerStartUpdate(c echo.Context) error {
	if err := updater.Start(); err != nil {
		switch {
		case errors.Is(err, updater.ErrNoUpdate):
			if peerVer := c.Request().Header.Get("X-Sync-Version"); peerVer != "" && peerVer != buildmode.Version {
				return c.JSON(http.StatusAccepted, httpx.Response{Success: true, Data: map[string]string{"status": "no update needed"}})
			}
			return c.JSON(http.StatusConflict, httpx.Response{Error: "No update available"})
		case errors.Is(err, updater.ErrAlreadyInstalling):
			return c.JSON(http.StatusAccepted, httpx.Response{Success: true, Data: map[string]string{"status": "already installing"}})
		default:
			log.Println("Error: [PeerStartUpdate] failed to start update:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't start update"})
		}
	}
	return c.JSON(http.StatusAccepted, httpx.Response{Success: true, Data: map[string]string{"status": "installing"}})
}

// PublishVersion inserts a new advertised build (cloud-only); it syncs to locals.
func PublishVersion(c echo.Context) error {
	var v store.Version
	if err := c.Bind(&v); err != nil {
		log.Println("Error: [PublishVersion] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := v.IsValid(); err != nil {
		log.Println("Error: [PublishVersion] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	if err := v.Create(); err != nil {
		log.Println("Error: [PublishVersion] failed to save version:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't save version"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: v})
}

var peerClient = &http.Client{Timeout: 15 * time.Second}

func triggerPeerUpdate(cfg *config.Config) error {
	url := strings.TrimRight(cfg.PeerURL, "/") + "/api/update/peer"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Sync-Secret", cfg.SyncSecret)
	req.Header.Set("X-Sync-Version", buildmode.Version)

	resp, err := peerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("peer status %d", resp.StatusCode)
	}
	return nil
}
