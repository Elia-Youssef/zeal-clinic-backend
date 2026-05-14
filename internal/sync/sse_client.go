package sync

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"clinic-api/internal/realtime"
)

var sseClient = &http.Client{}

var cloudConnected atomic.Bool

func IsCloudConnected() bool { return cloudConnected.Load() }

func setCloudConnected(v bool) {
	if cloudConnected.Swap(v) == v {
		return
	}
	realtime.Broadcast(realtime.Event{Type: "cloud_connection", Data: v})
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
	e.Notify()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: sync_pending") {
			e.Notify()
		}
	}
	return true, sc.Err()
}
