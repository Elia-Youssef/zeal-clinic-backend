package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	syncpkg "clinic-api/internal/sync"
)

// Action is a named unit of periodic work the Monitor evaluates on every tick.
// Duration is the minimum gap between successive firings; the action runs on
// the first tick where time.Since(lastFired) >= Duration. A zero Duration means
// fire every tick.
type Action struct {
	Name     string
	Duration time.Duration
	Fn       func() error
}

// Monitor runs a dynamically-extensible list of Actions on a fixed interval
// inside its own goroutine. Register new Actions at any time; they start firing
// on the next tick.
type Monitor struct {
	interval  time.Duration
	mu        sync.Mutex
	actions   []Action
	lastFired map[string]time.Time
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func New(interval time.Duration) *Monitor {
	return &Monitor{interval: interval, lastFired: make(map[string]time.Time)}
}

func (m *Monitor) Register(actions ...Action) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions = append(m.actions, actions...)
}

func (m *Monitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.wg.Add(1)
	go m.run(ctx)
	log.Printf("monitor: started with interval %s", m.interval)
}

func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

func (m *Monitor) run(ctx context.Context) {
	defer m.wg.Done()

	// Re-arm a fresh timer each iteration aligned to the next interval
	// boundary (e.g. for a 1-minute interval, ticks fire at HH:MM:00). Doing
	// it per-tick instead of starting a Ticker means drift never accumulates
	// and the schedule self-corrects after clock changes or system suspend.
	for {
		timer := time.NewTimer(m.timeUntilNextTick())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			m.tick()
		}
	}
}

func (m *Monitor) timeUntilNextTick() time.Duration {
	now := time.Now()
	return now.Truncate(m.interval).Add(m.interval).Sub(now)
}

func (m *Monitor) tick() {
	m.mu.Lock()
	now := time.Now()
	var due []Action
	for _, a := range m.actions {
		if a.Duration > 0 && now.Sub(m.lastFired[a.Name]) < a.Duration {
			continue
		}
		m.lastFired[a.Name] = now
		due = append(due, a)
	}
	m.mu.Unlock()

	for _, a := range due {
		m.runAction(a)
	}
	// Monitor actions write outside the HTTP path, so the sync middleware
	// doesn't see them. PollLog only acts when sync_log actually advanced:
	// quiet ticks stay quiet, no unconditional pull/push.
	syncpkg.PollLog()
}

func (m *Monitor) runAction(a Action) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("monitor: action %q panicked: %v", a.Name, r)
		}
	}()
	if err := a.Fn(); err != nil {
		log.Printf("monitor: action %q failed: %v", a.Name, err)
	}
}
