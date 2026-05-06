package realtime

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// drainOne reads one event with a short timeout. Fails the test on timeout.
func drainOne(t *testing.T, c *Client) Event {
	t.Helper()
	select {
	case ev := <-c.Events():
		return ev
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for event for user %s", c.UserID())
		return Event{}
	}
}

func TestHub_RegisterAndUserID(t *testing.T) {
	h := newHub()
	c := h.Register("u1")
	if c.UserID() != "u1" {
		t.Errorf("UserID = %q want u1", c.UserID())
	}
	if h.ClientCount() != 1 {
		t.Errorf("ClientCount = %d want 1", h.ClientCount())
	}
	if c.Events() == nil {
		t.Errorf("Events channel must be non-nil")
	}
}

func TestHub_ClientCount(t *testing.T) {
	h := newHub()
	if h.ClientCount() != 0 {
		t.Errorf("empty hub should have 0 clients")
	}
	c1 := h.Register("u1")
	c2 := h.Register("u1") // same user, second tab
	c3 := h.Register("u2")
	if h.ClientCount() != 3 {
		t.Errorf("count = %d want 3", h.ClientCount())
	}
	c1.Close()
	if h.ClientCount() != 2 {
		t.Errorf("count after close = %d want 2", h.ClientCount())
	}
	c2.Close()
	c3.Close()
	if h.ClientCount() != 0 {
		t.Errorf("count after all closed = %d want 0", h.ClientCount())
	}
}

func TestHub_SendTo_DeliversToAllClientsForUser(t *testing.T) {
	h := newHub()
	c1 := h.Register("u1")
	c2 := h.Register("u1") // second tab
	c3 := h.Register("u2") // unrelated user

	h.SendTo("u1", Event{Type: "ping"})

	got1 := drainOne(t, c1)
	got2 := drainOne(t, c2)
	if got1.Type != "ping" || got2.Type != "ping" {
		t.Errorf("both u1 clients should receive 'ping'")
	}
	// u2 must not receive.
	select {
	case ev := <-c3.Events():
		t.Errorf("u2 unexpectedly received %v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_SendTo_UnknownUserDoesNothing(t *testing.T) {
	h := newHub()
	c := h.Register("u1")
	h.SendTo("nobody", Event{Type: "x"})
	select {
	case ev := <-c.Events():
		t.Errorf("unrelated user must not receive: %v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_Broadcast_DeliversToEveryone(t *testing.T) {
	h := newHub()
	cs := []*Client{
		h.Register("u1"),
		h.Register("u2"),
		h.Register("u3"),
	}
	h.Broadcast(Event{Type: "all", Data: 42})
	for _, c := range cs {
		got := drainOne(t, c)
		if got.Type != "all" {
			t.Errorf("client %s got type %q", c.UserID(), got.Type)
		}
	}
}

func TestHub_Broadcast_EmptyHubIsNoop(t *testing.T) {
	h := newHub()
	// Should not panic.
	h.Broadcast(Event{Type: "x"})
}

func TestHub_Close_RemovesAndClosesChannel(t *testing.T) {
	h := newHub()
	c := h.Register("u1")
	c.Close()
	if h.ClientCount() != 0 {
		t.Errorf("client should be removed from hub")
	}
	// Reading from a closed channel should return zero value with !ok.
	select {
	case ev, ok := <-c.Events():
		if ok {
			t.Errorf("expected channel closed, got %v", ev)
		}
	case <-time.After(50 * time.Millisecond):
		t.Errorf("read from closed chan should not block")
	}
}

func TestHub_Close_Idempotent(t *testing.T) {
	h := newHub()
	c := h.Register("u1")
	c.Close()
	// Second close must not panic and must not double-close the channel.
	c.Close()
	if h.ClientCount() != 0 {
		t.Errorf("count should remain 0")
	}
}

func TestHub_Close_AfterAlreadyRemovedDoesNotDoubleCloseChan(t *testing.T) {
	h := newHub()
	c := h.Register("u1")
	// Manually remove from hub map without closing the channel, then call Close().
	// This mimics any future code path that bypasses Close; Close should be a no-op.
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	// Close should now be a no-op (no double-close panic).
	c.Close()
}

func TestHub_FullBuffer_DropsWithoutBlocking(t *testing.T) {
	h := newHub()
	c := h.Register("u1")
	// Buffer size is 16. Fill it without reading.
	for i := 0; i < 16; i++ {
		h.SendTo("u1", Event{Type: "fill"})
	}
	// 17th send must not block; it should be dropped silently.
	done := make(chan struct{})
	go func() {
		h.SendTo("u1", Event{Type: "overflow"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("SendTo on full buffer blocked — should drop")
	}
	// Drain and confirm only the original 16 were delivered (none of them
	// should have type 'overflow' if drop happened on the last attempt).
	count := 0
	overflows := 0
	for {
		select {
		case ev, ok := <-c.Events():
			if !ok {
				goto done
			}
			count++
			if ev.Type == "overflow" {
				overflows++
			}
			if count >= 16 {
				goto done
			}
		case <-time.After(100 * time.Millisecond):
			goto done
		}
	}
done:
	if count != 16 {
		t.Errorf("expected 16 delivered, got %d", count)
	}
	if overflows != 0 {
		t.Errorf("17th 'overflow' event should have been dropped")
	}
}

func TestHub_ConcurrentRegisterCloseBroadcast(t *testing.T) {
	// Race detector should catch any unsynchronized access.
	h := newHub()

	const workers = 20
	const iters = 50

	var wg sync.WaitGroup
	var sent int64
	stop := make(chan struct{})

	// Broadcaster.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				h.Broadcast(Event{Type: "tick"})
				atomic.AddInt64(&sent, 1)
			}
		}
	}()

	// Workers register and close many clients.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				c := h.Register("u")
				// Drain a few events so deliver doesn't always drop.
				go func(cl *Client) {
					for range cl.Events() {
					}
				}(c)
				time.Sleep(time.Microsecond)
				c.Close()
			}
		}(i)
	}

	// Let it churn briefly.
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()

	// All registered clients were closed, hub should be empty.
	if h.ClientCount() != 0 {
		t.Errorf("hub leaked clients: %d", h.ClientCount())
	}
	if atomic.LoadInt64(&sent) == 0 {
		t.Errorf("broadcaster sent nothing")
	}
}

func TestPackageLevelHelpers_UseDefaultHub(t *testing.T) {
	// Default() must return a stable singleton.
	if Default() != defaultHub {
		t.Errorf("Default() should return defaultHub")
	}
	c := Register("pkg-user")
	defer c.Close()
	if c.UserID() != "pkg-user" {
		t.Errorf("got %q", c.UserID())
	}

	SendTo("pkg-user", Event{Type: "hello"})
	got := drainOne(t, c)
	if got.Type != "hello" {
		t.Errorf("got %v", got)
	}

	Broadcast(Event{Type: "world"})
	got2 := drainOne(t, c)
	if got2.Type != "world" {
		t.Errorf("got %v", got2)
	}
}
