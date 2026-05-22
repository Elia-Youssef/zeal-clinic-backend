package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"clinic-api/internal/api/httpx"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to read update status"})
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
		return c.JSON(http.StatusBadGateway, httpx.Response{Error: "pre-update sync failed; update aborted"})
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
			return c.JSON(http.StatusConflict, httpx.Response{Error: "no update available"})
		case errors.Is(err, updater.ErrAlreadyInstalling):
			return c.JSON(http.StatusConflict, httpx.Response{Error: "update already in progress"})
		default:
			log.Println("Error: [StartUpdate] failed to start update:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to start update"})
		}
	}

	data := map[string]any{"status": "installing"}
	if peerWarn != "" {
		data["warning"] = peerWarn
	}
	return c.JSON(http.StatusAccepted, httpx.Response{Success: true, Data: data})
}

// PeerStartUpdate is the cloud-only endpoint a local node calls to update the cloud.
func PeerStartUpdate(c echo.Context) error {
	if err := updater.Start(); err != nil {
		switch {
		case errors.Is(err, updater.ErrNoUpdate):
			return c.JSON(http.StatusConflict, httpx.Response{Error: "no update available"})
		case errors.Is(err, updater.ErrAlreadyInstalling):
			return c.JSON(http.StatusAccepted, httpx.Response{Success: true, Data: map[string]string{"status": "already installing"}})
		default:
			log.Println("Error: [PeerStartUpdate] failed to start update:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to start update"})
		}
	}
	return c.JSON(http.StatusAccepted, httpx.Response{Success: true, Data: map[string]string{"status": "installing"}})
}

// PublishVersion inserts a new advertised build (cloud-only); it syncs to locals.
func PublishVersion(c echo.Context) error {
	var v store.Version
	if err := c.Bind(&v); err != nil {
		log.Println("Error: [PublishVersion] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := v.IsValid(); err != nil {
		log.Println("Error: [PublishVersion] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	if err := v.Create(); err != nil {
		log.Println("Error: [PublishVersion] failed to save version:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to save version"})
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
