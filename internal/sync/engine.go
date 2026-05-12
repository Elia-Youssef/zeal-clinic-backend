package sync

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Engine drives outbound replication on the local side. It's woken by:
//   - the write debouncer (sync.OnWrite fired from HTTP middleware) ~1s
//     after a burst of writes
//   - the SSE listener when the cloud signals "sync_pending"
//   - the monitor's per-minute PollLog tick, which fires Notify only when
//     sync_log actually advanced past the last observed seq; this covers
//     writes that bypass the HTTP middleware (monitor jobs) without
//     paying a pull/push cost on quiet minutes
//
// On the cloud side only the debouncer side fires (notifies peer SSE
// subscribers); the run-loop and SSE listener stay dormant since there's
// no PeerURL to dial.
//
// The pull-first order in cycle() is deliberate: only one side has the
// conflict logic (local), so we fold remote changes into local first,
// then push our reconciled state out.
type Engine struct {
	db       *sql.DB
	cfg      Config
	httpC    *http.Client
	signalCh chan struct{}

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	stopWait sync.WaitGroup
}

// Config wires the engine to its peer. Whether this node is the
// "authoritative" side (local clinic, runs LWW gate) follows from
// buildmode.Cloud, not from this struct.
type Config struct {
	PeerURL string // base URL of the peer's HTTP API
	Secret  string // shared SYNC_SECRET
}

// New constructs an Engine. Call Start to actually do work.
func New(db *sql.DB, cfg Config) *Engine {
	cfg.PeerURL = strings.TrimRight(cfg.PeerURL, "/")
	return &Engine{
		db:       db,
		cfg:      cfg,
		httpC:    &http.Client{Timeout: 30 * time.Second},
		signalCh: make(chan struct{}, 1),
	}
}

// Notify pokes the engine to run a sync cycle now. Cheap and non-blocking;
// drops the signal if one is already pending. Safe to call from triggers,
// handlers, or the SSE listener.
func (e *Engine) Notify() {
	select {
	case e.signalCh <- struct{}{}:
	default:
	}
}

// Start brings the engine online: resets the apply guard from any prior
// crash, binds the write debouncer so HTTP middleware / monitor pokes
// reach this engine, and (when PeerURL is set) spawns the outbound loop
// and SSE listener. Subsequent calls are a no-op while already running.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return
	}

	// Reset the trigger guard in case a previous run was killed mid-apply
	// and left it raised. Nothing else is applying right now (Apply only
	// runs inside this engine), so unconditional clear is safe.
	if _, err := e.db.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		log.Printf("[sync] WARN: reset apply guard: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.running = true

	// Wire OnWrite()'s delayed fan-out to this engine. notifyPeers fires
	// regardless of bind state (cloud-side SSE listeners need it); engine
	// Notify only matters once a peer URL is configured.
	debounce.bind(e)

	if e.cfg.PeerURL == "" {
		log.Printf("[sync] passive mode (no PEER_URL): accepting pushes only")
		return
	}

	e.stopWait.Add(1)
	go e.run(ctx)
	go runSSEListener(ctx, e)
	log.Printf("[sync] engine started: peer=%s", e.cfg.PeerURL)
}

// Stop signals the engine to halt and waits for the outbound goroutine to
// exit. The watcher and SSE listener also exit via the cancelled ctx but
// aren't joined (they're idempotent and process is shutting down anyway).
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.cancel()
	e.running = false
	e.mu.Unlock()
	e.stopWait.Wait()
}

func (e *Engine) run(ctx context.Context) {
	defer e.stopWait.Done()

	// One immediate cycle on startup so we don't wait for a signal to drain
	// whatever accumulated while we were down.
	e.cycle(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.signalCh:
			e.cycle(ctx)
		}
	}
}

// cycle runs one pull-then-push round. If pull fails (typically: peer
// unreachable) we skip push too: same network, same failure, no need to
// log twice. The next signal retries.
func (e *Engine) cycle(ctx context.Context) {
	if err := e.pull(ctx); err != nil {
		log.Printf("[sync] pull error: %v", err)
		return
	}
	if err := e.push(ctx); err != nil {
		log.Printf("[sync] push error: %v", err)
	}
}
