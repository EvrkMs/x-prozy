// Package ws provides a WebSocket broadcast hub for real-time node updates.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

const (
	writeTimeout = 5 * time.Second
	pingInterval = 25 * time.Second
)

// Hub manages WebSocket clients and broadcasts JSON messages.
type Hub struct {
	log     *slog.Logger
	mu      sync.RWMutex
	clients map[*client]struct{}
}

type client struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
}

// NewHub creates a new WebSocket hub.
func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		log:     log.With("component", "ws"),
		clients: make(map[*client]struct{}),
	}
}

// Accept upgrades an HTTP connection to WebSocket and registers the client.
// Blocks until the client disconnects.
func (h *Hub) Accept(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allow all origins in dev; in prod you'd restrict this.
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.log.Warn("ws accept failed", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	c := &client{conn: conn, ctx: ctx, cancel: cancel}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	count := len(h.clients)
	h.mu.Unlock()

	h.log.Info("ws client connected", "clients", count)

	// Keep connection alive: read messages (we ignore them, they're for keepalive).
	// The loop exits when the client disconnects or ctx is cancelled.
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		count := len(h.clients)
		h.mu.Unlock()
		cancel()
		conn.Close(websocket.StatusNormalClosure, "")
		h.log.Info("ws client disconnected", "clients", count)
	}()

	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return // client disconnected
		}
	}
}

// Broadcast serializes data to JSON and sends to all connected clients.
func (h *Hub) Broadcast(msgType string, data any) {
	h.mu.RLock()
	count := len(h.clients)
	h.mu.RUnlock()

	if count == 0 {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"type": msgType,
		"data": data,
	})
	if err != nil {
		h.log.Warn("ws marshal error", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		go func(c *client) {
			ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
			defer cancel()
			if err := c.conn.Write(ctx, websocket.MessageText, payload); err != nil {
				// Client gone — will be cleaned up by read loop.
				c.cancel()
			}
		}(c)
	}
}

// ClientCount returns the number of connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
