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

// nodeLabel is a short identifier for the running binary, used only in
// the startup log line and /status JSON. Behaviour never branches on it.
func nodeLabel() string {
	if buildmode.Cloud {
		return "cloud"
	}
	return "local"
}

// RegisterRoutes mounts /api/sync/* on cloud builds only. The local
// clinic binary never receives sync calls (it dials the cloud), so
// exposing the endpoints would be both pointless and a needless attack
// surface. No-op also when Secret is unset.
func (a *API) RegisterRoutes(e *echo.Echo) {
	if !buildmode.Cloud || a.Secret == "" {
		return
	}
	g := e.Group("/api/sync", a.requireSecret)
	g.GET("/pull", a.handlePull)
	g.POST("/push", a.handlePush)
	g.GET("/events", a.handleEvents)
	g.GET("/status", a.handleStatus)
	log.Printf("[sync] API mounted under /api/sync (self=%s)", nodeLabel())
}

func (a *API) requireSecret(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		got := c.Request().Header.Get("X-Sync-Secret")
		if got == "" {
			got = c.QueryParam("sync_secret") // SSE/EventSource fallback
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.Secret)) != 1 {
			log.Printf("[sync] unauthorized %s %s from %s", c.Request().Method, c.Path(), c.RealIP())
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		return next(c)
	}
}

func errResp(c echo.Context, status int, err error) error {
	log.Printf("[sync] %s %s -> %d: %v", c.Request().Method, c.Path(), status, err)
	return c.JSON(status, map[string]string{"error": err.Error()})
}

// handlePull returns sync_log rows authored by this node with seq > since,
// up to limit rows. Side effect: prunes the outgoing log up to `since`,
// since the caller has confirmed it received everything through that point.
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
	// The pulled rows need to be enriched with the live row state before we
	// hand them to the caller, same as the push path. Without this the
	// caller would see (table, id, op) entries with empty row_json.
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

// handlePush receives rows authored by the caller and applies them. Cloud
// writes land unconditionally: local is the source of truth. If real
// rows landed, tell connected frontends so open UIs refetch.
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

// handleEvents is the outbound notification stream the local server
// subscribes to. We send a "sync_pending" event whenever sync_log advances
// (driven by StartLogWatcher), plus periodic heartbeats so the peer can
// detect a stale connection.
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

	// Hello so the peer knows the channel is live. The sync handshake has
	// no payload (our parser only looks at the `event:` line), so we skip
	// the otherwise-spec-mandatory `data:` line.
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

// handleStatus is a lightweight health/inspection endpoint useful for ops
// checks: confirms sync is configured and reports current cursors.
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
