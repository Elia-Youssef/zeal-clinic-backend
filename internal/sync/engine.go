package sync

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Engine runs pull-then-push replication when writes, SSE, or PollLog wake it.
type Engine struct {
	db       *sql.DB
	cfg      Config
	httpC    *http.Client
	signalCh chan struct{}

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	stopWait sync.WaitGroup

	cycleMu sync.Mutex // serializes pull-then-push so RunNow can't race the loop

	sessionMu    sync.RWMutex
	sessionToken string
}

type Config struct {
	PeerURL string
	Secret  string
}

func New(db *sql.DB, cfg Config) *Engine {
	cfg.PeerURL = strings.TrimRight(cfg.PeerURL, "/")
	return &Engine{
		db:       db,
		cfg:      cfg,
		httpC:    &http.Client{Timeout: 30 * time.Second},
		signalCh: make(chan struct{}, 1),
	}
}

// Notify schedules a sync cycle without blocking.
func (e *Engine) Notify() {
	select {
	case e.signalCh <- struct{}{}:
	default:
	}
}

// Start binds wake-ups and starts outbound sync when PeerURL is set.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return
	}

	// Recover from a previous crash mid-apply.
	if _, err := e.db.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		log.Printf("[sync] WARN: reset apply guard: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.running = true

	debounce.bind(e)

	if e.cfg.PeerURL == "" {
		log.Printf("[sync] passive mode (no PEER_URL): accepting pushes only")
		return
	}

	e.stopWait.Add(2)
	go e.run(ctx)
	go func() {
		defer e.stopWait.Done()
		runSSEListener(ctx, e)
	}()
	log.Printf("[sync] engine started: peer=%s", e.cfg.PeerURL)
}

// Stop halts the outbound loop.
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.cancel()
	e.running = false
	e.mu.Unlock()
	debounce.unbind(e)
	e.stopWait.Wait()
	e.httpC.CloseIdleConnections()
	sseClient.CloseIdleConnections()
}

func (e *Engine) run(ctx context.Context) {
	defer e.stopWait.Done()

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

// cycle runs one pull-then-push round.
func (e *Engine) cycle(ctx context.Context) {
	e.cycleMu.Lock()
	defer e.cycleMu.Unlock()
	if err := e.pull(ctx); err != nil {
		if ctx.Err() == nil {
			e.handleCycleFailure(ctx, "pull", err)
		}
		return
	}
	if err := e.push(ctx); err != nil {
		if ctx.Err() == nil {
			e.handleCycleFailure(ctx, "push", err)
		}
		return
	}
	if err := e.acknowledgeReady(ctx); err != nil && ctx.Err() == nil {
		log.Printf("[sync] ready acknowledgement: %v", err)
	}
}

// RunNow runs one synchronous pull-then-push cycle (no-op without a peer).
// Used to flush before a self-update, while both nodes still share a version.
func RunNow(ctx context.Context) error {
	debounce.mu.Lock()
	eng := debounce.engine
	debounce.mu.Unlock()
	if eng == nil || eng.cfg.PeerURL == "" {
		return nil
	}

	eng.cycleMu.Lock()
	defer eng.cycleMu.Unlock()
	if err := eng.pull(ctx); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	if err := eng.push(ctx); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	if err := eng.acknowledgeReady(ctx); err != nil {
		return fmt.Errorf("ready: %w", err)
	}
	return nil
}
