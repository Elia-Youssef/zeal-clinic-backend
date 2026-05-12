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

// sseClient is a long-lived client with no timeout. SSE connections are
// open until the peer closes them or ctx cancels; the default http.Client
// timeout would tear them down prematurely.
var sseClient = &http.Client{}

// cloudConnected mirrors streamOnce's live state: true while the SSE
// session is held open, false during reconnect backoff or in builds with
// no peer to dial. Read by IsCloudConnected so the HTTP layer can hand a
// snapshot to freshly-registered realtime subscribers.
var cloudConnected atomic.Bool

// IsCloudConnected reports whether the local server is currently holding a
// live SSE connection to the cloud peer.
func IsCloudConnected() bool { return cloudConnected.Load() }

// setCloudConnected updates the flag and, on a true transition, fans the
// new state out to every realtime client so frontends can flip their
// online/offline indicator without polling.
func setCloudConnected(v bool) {
	if cloudConnected.Swap(v) == v {
		return
	}
	realtime.Broadcast(realtime.Event{Type: "cloud_connection", Data: v})
}

// runSSEListener opens a long-lived GET to /api/sync/events on the peer and
// translates "sync_pending" events into engine signals. Reconnects with
// exponential backoff (1s to 30s) on error; backoff resets after any
// successful connect so a single hiccup doesn't penalise later reconnects.
//
// This is a textbook SSE flow: the local (client) dials out, the cloud
// (server) holds the connection and streams events down. Local being on a
// private network is fine because it's the one initiating.
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
			// We had a working connection; treat the disconnect as fresh.
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

// streamOnce holds a single SSE connection until it errors or ctx cancels.
// Returns connected=true once the HTTP 200 lands so the caller knows the
// session was real (and worth resetting backoff for).
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
	// On (re)connect we may have missed events while down, so pull once.
	e.Notify()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		// We only care about the event type; the data payload is advisory.
		if strings.HasPrefix(line, "event: sync_pending") {
			e.Notify()
		}
	}
	return true, sc.Err()
}
