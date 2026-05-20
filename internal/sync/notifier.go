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

type peerHub struct {
	mu  sync.RWMutex
	chs map[chan struct{}]struct{}
}

var peers = &peerHub{chs: make(map[chan struct{}]struct{})}

func (h *peerHub) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.chs[ch] = struct{}{}
	first := len(h.chs) == 1
	h.mu.Unlock()
	if first {
		setCloudConnected(true)
	}
	return ch
}

func (h *peerHub) unsubscribe(ch chan struct{}) {
	h.mu.Lock()
	last := false
	if _, ok := h.chs[ch]; ok {
		delete(h.chs, ch)
		close(ch)
		last = len(h.chs) == 0
	}
	h.mu.Unlock()
	if last {
		setCloudConnected(false)
	}
}

func (h *peerHub) broadcast() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.chs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func notifyPeers() { peers.broadcast() }

const writeDebounceWait = time.Second

type writeDebouncer struct {
	mu     sync.Mutex
	timer  *time.Timer
	engine *Engine
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

// OnWrite debounces write-triggered sync wake-ups.
func OnWrite() { debounce.fire() }

var lastObservedLogSeq atomic.Int64

// PollLog wakes sync only when sync_log advanced since the last poll.
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
		return
	}
	if buildmode.Cloud {
		notifyPeers()
	} else {
		eng.Notify()
	}
}

// Middleware fires OnWrite after mutating HTTP requests.
func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)
			switch c.Request().Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				OnWrite()
			}
			return err
		}
	}
}
