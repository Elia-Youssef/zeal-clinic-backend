package sync

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/realtime"

	"github.com/labstack/echo/v4"
)

type API struct {
	DB     *sql.DB
	Secret string
}

func nodeLabel() string {
	if buildmode.Cloud {
		return "cloud"
	}
	return "local"
}

// RegisterRoutes mounts /api/sync/* only on cloud builds with SYNC_SECRET.
func (a *API) RegisterRoutes(e *echo.Echo) {
	if !buildmode.Cloud || a.Secret == "" {
		return
	}
	g := e.Group("/api/sync", a.requireSecret)
	// pull/push/events are version-gated: a peer on a different build is refused
	// so data only ever flows between identical versions. status stays open for
	// diagnostics.
	g.GET("/pull", a.handlePull, a.requireVersion)
	g.POST("/push", a.handlePush, a.requireVersion)
	g.GET("/events", a.handleEvents, a.requireVersion)
	g.GET("/status", a.handleStatus)
	log.Printf("[sync] API mounted under /api/sync (self=%s)", nodeLabel())
}

func (a *API) requireSecret(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		got := c.Request().Header.Get("X-Sync-Secret")
		if got == "" {
			got = c.QueryParam("sync_secret")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.Secret)) != 1 {
			log.Printf("[sync] unauthorized %s %s from %s", c.Request().Method, c.Path(), c.RealIP())
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		return next(c)
	}
}

// requireVersion refuses a peer on a different build (X-Sync-Version), so data
// never syncs across a schema change.
func (a *API) requireVersion(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if peerVer := c.Request().Header.Get("X-Sync-Version"); peerVer != buildmode.Version {
			log.Printf("[sync] %s %s rejected: peer version %q != %q",
				c.Request().Method, c.Path(), peerVer, buildmode.Version)
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "version mismatch",
				"peer":  peerVer,
				"cloud": buildmode.Version,
			})
		}
		return next(c)
	}
}

func errResp(c echo.Context, status int, err error) error {
	log.Printf("[sync] %s %s -> %d: %v", c.Request().Method, c.Path(), status, err)
	return c.JSON(status, map[string]string{"error": err.Error()})
}

// handlePull returns rows after since and prunes rows the caller has confirmed.
func (a *API) handlePull(c echo.Context) error {
	since, _ := strconv.ParseInt(c.QueryParam("since"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 2000 {
		limit = pullBatchSize
	}

	rows, err := LoadBatch(a.DB, since, limit)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, fmt.Errorf("load batch (since=%d limit=%d): %w", since, limit, err))
	}
	if err := enrichBatch(a.DB, rows); err != nil {
		return errResp(c, http.StatusInternalServerError, fmt.Errorf("enrich batch: %w", err))
	}
	maxSeq, _ := MaxLogSeq(a.DB)

	if since > 0 {
		go func(s int64) {
			if _, err := PruneOutgoing(a.DB, s); err != nil {
				log.Printf("[sync] prune outgoing (through seq=%d): %v", s, err)
			}
		}(since)
	}

	return c.JSON(http.StatusOK, PullResponse{Rows: rows, MaxSeq: maxSeq})
}

// handlePush applies rows from the caller and broadcasts when data lands.
func (a *API) handlePush(c echo.Context) error {
	var req PushRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return errResp(c, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
	}
	if len(req.Rows) == 0 {
		return c.JSON(http.StatusOK, PushResponse{})
	}

	applied, conflicts, err := Apply(a.DB, req.Rows)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, fmt.Errorf("apply %d rows: %w", len(req.Rows), err))
	}
	if len(req.Rows) > len(conflicts) {
		realtime.Broadcast(realtime.Event{Type: "data_changed"})
	}
	return c.JSON(http.StatusOK, PushResponse{AppliedSeq: applied, Conflicts: conflicts})
}

func (a *API) handleEvents(c echo.Context) error {
	res := c.Response()
	h := res.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	res.WriteHeader(http.StatusOK)
	res.Flush()

	ch := peers.subscribe()
	defer peers.unsubscribe(ch)

	peer := c.RealIP()
	log.Printf("[sync] events peer=%s connected", peer)
	defer log.Printf("[sync] events peer=%s disconnected", peer)

	if _, err := res.Write([]byte("event: hello\n\n")); err != nil {
		return nil
	}
	res.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ch:
			if !ok {
				return nil
			}
			if _, err := res.Write([]byte("event: sync_pending\n\n")); err != nil {
				return nil
			}
			res.Flush()
		case <-heartbeat.C:
			if _, err := res.Write([]byte(": ping\n\n")); err != nil {
				return nil
			}
			res.Flush()
		}
	}
}

func (a *API) handleStatus(c echo.Context) error {
	maxSeq, _ := MaxLogSeq(a.DB)
	pushed, pulled, _ := GetState(a.DB, SyncedPeer)
	return c.JSON(http.StatusOK, map[string]any{
		"self_id":         nodeLabel(),
		"max_log_seq":     maxSeq,
		"last_pushed_seq": pushed,
		"last_pulled_seq": pulled,
	})
}
