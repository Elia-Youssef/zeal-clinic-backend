package monitor

import (
	"context"
	"log"
	"sync"
	"time"
)

// Action is a named unit of periodic work the Monitor runs on every tick.
type Action struct {
	Name string
	Fn   func() error
}

// Monitor runs a dynamically-extensible list of Actions on a fixed interval
// inside its own goroutine. Register new Actions at any time; they start firing
// on the next tick.
type Monitor struct {
	interval time.Duration
	mu       sync.Mutex
	actions  []Action
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func New(interval time.Duration) *Monitor {
	return &Monitor{interval: interval}
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
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

func (m *Monitor) tick() {
	m.mu.Lock()
	actions := append([]Action(nil), m.actions...)
	m.mu.Unlock()

	for _, a := range actions {
		m.runAction(a)
	}
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
