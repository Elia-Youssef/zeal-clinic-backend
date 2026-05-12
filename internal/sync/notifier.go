package sync

import (
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"clinic-api/internal/buildmode"

	"github.com/labstack/echo/v4"
)

// peerHub is a one-channel-per-listener fan-out for "something landed in
// sync_log" notifications, used by the cloud-side /api/sync/events SSE
// handler. Separate from internal/realtime to avoid leaking user-scoped
// events into the machine-to-machine sync channel and vice versa.
//
// In practice the cloud has at most one listener at a time (the local
// clinic server), but the map keeps the design symmetric and lets us run
// integration tests that connect twice.
type peerHub struct {
	mu  sync.RWMutex
	chs map[chan struct{}]struct{}
}

var peers = &peerHub{chs: make(map[chan struct{}]struct{})}

func (h *peerHub) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.chs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *peerHub) unsubscribe(ch chan struct{}) {
	h.mu.Lock()
	if _, ok := h.chs[ch]; ok {
		delete(h.chs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *peerHub) broadcast() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.chs {
		select {
		case ch <- struct{}{}:
		default:
			// Listener already has a pending notification.
		}
	}
}

// notifyPeers fires every connected peer SSE listener.
func notifyPeers() { peers.broadcast() }

// writeDebounceWait is the trailing-edge debounce window. The first write
// schedules a fire after this delay; further writes within the window
// reset the timer. Bursts of writes coalesce into one sync poke.
const writeDebounceWait = time.Second

// writeDebouncer coalesces "something probably wrote to sync_log" signals
// into one delayed fire, so a flurry of HTTP writes triggers at most one
// sync cycle.
type writeDebouncer struct {
	mu     sync.Mutex
	timer  *time.Timer
	engine *Engine // populated by Engine.Start
}

var debounce = &writeDebouncer{}

func (d *writeDebouncer) fire() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(writeDebounceWait, func() {
		notifyPeers()
		d.mu.Lock()
		eng := d.engine
		d.mu.Unlock()
		if eng != nil {
			eng.Notify()
		}
	})
}

func (d *writeDebouncer) bind(e *Engine) {
	d.mu.Lock()
	d.engine = e
	d.mu.Unlock()
}

// OnWrite signals that something likely just wrote to a synced table.
// Debounced: bursts collapse into one delayed fan-out. Callable from HTTP
// middleware, monitor jobs, or any code path that mutates synced state.
func OnWrite() { debounce.fire() }

// lastObservedLogSeq tracks the highest sync_log seq seen by PollLog so
// quiet systems stay quiet: we only fire when something actually advanced.
// sync_log.seq is AUTOINCREMENT, so it grows monotonically even after
// pruning; a strict-greater-than check is sufficient.
var lastObservedLogSeq atomic.Int64

// PollLog checks whether sync_log has advanced since the last call. On
// advancement it fires one side-appropriate signal:
//   - cloud: notifyPeers, so subscribed locals come pull
//   - local: engine.Notify, to push our local changes
//
// Used as the periodic safety-net poke by the monitor, in place of an
// unconditional OnWrite fire. It covers writes that bypass the HTTP sync
// middleware (monitor actions, future internal jobs) without paying the
// pull/push cost on a quiet minute. Cheap and idempotent; no-op when no
// engine is bound or sync_log hasn't moved.
func PollLog() {
	debounce.mu.Lock()
	eng := debounce.engine
	debounce.mu.Unlock()
	if eng == nil {
		return
	}
	seq, err := MaxLogSeq(eng.db)
	if err != nil {
		log.Printf("[sync] poll log: %v", err)
		return
	}
	prev := lastObservedLogSeq.Load()
	if seq <= prev {
		return
	}
	if !lastObservedLogSeq.CompareAndSwap(prev, seq) {
		return // another caller raced ahead and already fired
	}
	if buildmode.Cloud {
		notifyPeers()
	} else {
		eng.Notify()
	}
}

// Middleware fires OnWrite after every non-read-only HTTP request. The
// trailing-edge debounce in OnWrite ensures bursts (e.g. ten POSTs in a
// second) trigger only a single sync cycle.
func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)
			switch c.Request().Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// read-only, skip
			default:
				OnWrite()
			}
			return err
		}
	}
}
