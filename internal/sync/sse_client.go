package sync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/realtime"
	"clinic-api/internal/tracking"
)

var sseClient = &http.Client{}

var cloudConnected atomic.Bool

type readyRequest struct {
	SessionToken string `json:"session_token"`
}

func (e *Engine) setSessionToken(token string) {
	e.sessionMu.Lock()
	e.sessionToken = token
	e.sessionMu.Unlock()
}

func (e *Engine) clearSessionToken(token string) {
	e.sessionMu.Lock()
	if e.sessionToken == token {
		e.sessionToken = ""
	}
	e.sessionMu.Unlock()
}

func (e *Engine) acknowledgeReady(ctx context.Context) error {
	e.sessionMu.RLock()
	token := e.sessionToken
	e.sessionMu.RUnlock()
	if token == "" {
		return nil
	}
	body, _ := json.Marshal(readyRequest{SessionToken: token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.PeerURL+"/api/sync/ready", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sync-Secret", e.cfg.Secret)
	req.Header.Set("X-Sync-Version", buildmode.Version)
	resp, err := e.httpC.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return nil
}

func IsCloudConnected() bool { return cloudConnected.Load() }

func setCloudConnected(v bool) {
	if cloudConnected.Swap(v) == v {
		return
	}
	realtime.Broadcast(realtime.Event{Type: "cloud_connection", Data: v})
	if v {
		tracking.Info(nil, "[sync] connected to peer")
	} else {
		tracking.Info(nil, "[sync] peer disconnected")
	}
}

// runSSEListener turns cloud sync_pending events into engine wake-ups.
func runSSEListener(ctx context.Context, e *Engine) {
	const minBackoff = time.Second
	const maxBackoff = 30 * time.Second
	backoff := minBackoff

	for {
		if ctx.Err() != nil {
			return
		}
		connected, err := streamOnce(ctx, e)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = minBackoff
		}
		if err != nil {
			log.Printf("[sync] SSE listener: %v (retry in %s)", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if !connected {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// streamOnce holds one SSE connection.
func streamOnce(ctx context.Context, e *Engine) (connected bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		e.cfg.PeerURL+"/api/sync/events", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Sync-Secret", e.cfg.Secret)
	req.Header.Set("X-Sync-Version", buildmode.Version)
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := sseClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	log.Printf("[sync] SSE listener connected to %s", e.cfg.PeerURL)
	setCloudConnected(true)
	defer setCloudConnected(false)

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	var sessionToken string
	defer func() { e.clearSessionToken(sessionToken) }()
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: sync_pending") {
			e.Notify()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			var hello readyRequest
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &hello) == nil && hello.SessionToken != "" {
				sessionToken = hello.SessionToken
				e.setSessionToken(sessionToken)
				e.Notify()
			}
		}
	}
	return true, sc.Err()
}
