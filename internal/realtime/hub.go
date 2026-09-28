package realtime

import (
	"log"
	"sync"
	"sync/atomic"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// NotificationsChanged tells a user's open tabs that their notifications
// changed in a way no "notification" event carries (some were deleted for
// them), so the tabs load their unread count and list again.
const NotificationsChanged = "notifications_changed"

type Client struct {
	userID  string
	ch      chan Event
	hub     *Hub
	pending atomic.Bool
}

// Events returns the channel the SSE handler reads from.
func (c *Client) Events() <-chan Event { return c.ch }

// UserID is the authenticated user owning this connection.
func (c *Client) UserID() string { return c.userID }

// TakePending reports-and-clears whether an event was dropped (for SSE resync).
func (c *Client) TakePending() bool { return c.pending.Swap(false) }

// Close removes the client from its hub and closes the event channel.
// Safe to call multiple times.
func (c *Client) Close() {
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if _, ok := c.hub.clients[c]; ok {
		delete(c.hub.clients, c)
		close(c.ch)
	}
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func newHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

// Register creates a Client subscribed to this hub. Buffer is small on
// purpose: a stuck reader should cause drops, not memory growth.
func (h *Hub) Register(userID string) *Client {
	c := &Client{userID: userID, ch: make(chan Event, 16), hub: h}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Broadcast sends ev to every connected client.
func (h *Hub) Broadcast(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		deliver(c, ev)
	}
}

// SendTo sends ev to every connection belonging to userID (a user may have
// multiple tabs open, hence multiple clients).
func (h *Hub) SendTo(userID string, ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.userID == userID {
			deliver(c, ev)
		}
	}
}

// ClientCount returns the number of currently-connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func deliver(c *Client, ev Event) {
	select {
	case c.ch <- ev:
	default:
		c.pending.Store(true)
		log.Printf("realtime: client %s buffer full, dropping %q (will resync)", c.userID, ev.Type)
	}
}

var defaultHub = newHub()

func Default() *Hub                  { return defaultHub }
func Broadcast(ev Event)             { defaultHub.Broadcast(ev) }
func SendTo(userID string, ev Event) { defaultHub.SendTo(userID, ev) }
func Register(userID string) *Client { return defaultHub.Register(userID) }
