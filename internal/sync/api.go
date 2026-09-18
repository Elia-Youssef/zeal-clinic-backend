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
	"clinic-api/internal/tracking"

	"github.com/labstack/echo/v4"
)

type API struct {
	DB     *sql.DB
	DBFunc func() *sql.DB
	Secret string
}

func (a *API) activeDB() *sql.DB {
	if a.DBFunc != nil {
		return a.DBFunc()
	}
	return a.DB
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
	g.POST("/ready", a.handleReady, a.requireVersion)
	g.POST("/failed", a.handleFailed, a.requireVersion)
	g.GET("/events", a.handleEvents, a.requireVersion)
	g.GET("/status", a.handleStatus)
	log.Printf("[sync] API mounted under /api/sync (self=%s)", nodeLabel())
}

// requireSecret reads the shared secret from the X-Sync-Secret header only: a
// query parameter would end up in request logs.
func (a *API) requireSecret(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		got := c.Request().Header.Get("X-Sync-Secret")
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

func (a *API) inboundSyncFailure(c echo.Context, err error) error {
	failCurrentCriticalSession()
	tracking.CaptureError(c, err)
	notifySyncFailure(a.activeDB())
	return errResp(c, http.StatusInternalServerError, err)
}

// handlePull returns rows after since and prunes rows the caller has confirmed.
func (a *API) handlePull(c echo.Context) error {
	db := a.activeDB()
	if db == nil {
		return errResp(c, http.StatusServiceUnavailable, fmt.Errorf("database unavailable"))
	}
	since, _ := strconv.ParseInt(c.QueryParam("since"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 2000 {
		limit = pullBatchSize
	}

	rows, err := LoadBatch(db, since, limit)
	if err != nil {
		return a.inboundSyncFailure(c, fmt.Errorf("[sync] load pull batch (since=%d limit=%d): %w", since, limit, err))
	}
	if err := enrichBatch(db, rows); err != nil {
		return a.inboundSyncFailure(c, fmt.Errorf("[sync] enrich pull batch: %w", err))
	}
	maxSeq, _ := MaxLogSeq(db)

	if since > 0 {
		if _, err := PruneOutgoing(db, since); err != nil {
			log.Printf("[sync] prune outgoing (through seq=%d): %v", since, err)
		}
	}

	return c.JSON(http.StatusOK, PullResponse{Rows: rows, MaxSeq: maxSeq})
}

// handlePush applies rows from the caller and broadcasts when data lands.
func (a *API) handlePush(c echo.Context) error {
	db := a.activeDB()
	if db == nil {
		return errResp(c, http.StatusServiceUnavailable, fmt.Errorf("database unavailable"))
	}
	var req PushRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return errResp(c, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
	}
	if len(req.Rows) == 0 {
		return c.JSON(http.StatusOK, PushResponse{})
	}

	applied, conflicts, err := Apply(db, req.Rows)
	if err != nil {
		return a.inboundSyncFailure(c, fmt.Errorf("[sync] apply push batch (%d rows): %w", len(req.Rows), err))
	}
	if len(req.Rows) > len(conflicts) {
		realtime.Broadcast(realtime.Event{Type: "data_changed"})
	}
	return c.JSON(http.StatusOK, PushResponse{AppliedSeq: applied, Conflicts: conflicts})
}

func (a *API) handleReady(c echo.Context) error {
	var req readyRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return errResp(c, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
	}
	if !markCriticalSessionReady(req.SessionToken) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "stale sync session"})
	}
	markSyncRecovered()
	return c.JSON(http.StatusOK, map[string]bool{"ready": true})
}

func (a *API) handleFailed(c echo.Context) error {
	var req readyRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return errResp(c, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
	}
	if !failCriticalSession(req.SessionToken) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "stale sync session"})
	}
	notifySyncFailure(a.activeDB())
	return c.JSON(http.StatusOK, map[string]bool{"ready": false})
}

func (a *API) handleEvents(c echo.Context) error {
	token, err := beginCriticalSession()
	if err != nil {
		return errResp(c, http.StatusInternalServerError, fmt.Errorf("create sync session: %w", err))
	}
	defer endCriticalSession(token)

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

	hello, _ := json.Marshal(readyRequest{SessionToken: token})
	if _, err := fmt.Fprintf(res, "event: hello\ndata: %s\n\n", hello); err != nil {
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
	db := a.activeDB()
	if db == nil {
		return errResp(c, http.StatusServiceUnavailable, fmt.Errorf("database unavailable"))
	}
	maxSeq, _ := MaxLogSeq(db)
	pushed, pulled, _ := GetState(db, SyncedPeer)
	return c.JSON(http.StatusOK, map[string]any{
		"self_id":         nodeLabel(),
		"max_log_seq":     maxSeq,
		"last_pushed_seq": pushed,
		"last_pulled_seq": pulled,
	})
}
