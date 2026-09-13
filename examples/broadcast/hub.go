package main

import (
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/33TU/ews"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
)

// Hub relays every message it receives to all connected clients.
//
// Each client has a Queue, so a broadcast encodes the message once with
// Prepare, hands the shared bytes to every queue, and returns without waiting
// on any socket. A client that falls behind fills its queue and is dropped
// rather than slowing the others down.
type Hub struct {
	// QueueLimit is how far behind a client may fall, in queued bytes,
	// before it is dropped. Zero means 1 MiB.
	QueueLimit int

	opts handshake.Options

	mu      sync.RWMutex
	clients map[*ws.Conn]*client
}

type client struct {
	conn  net.Conn
	queue *ws.Queue
}

func NewHub(opts handshake.Options) *Hub {
	return &Hub{opts: opts, clients: map[*ws.Conn]*client{}}
}

// ServeHTTP upgrades the request and reads from the client until it leaves.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, res, err := ews.Upgrade(w, r, h.opts)
	if err != nil {
		return
	}
	c, err := ws.NewConn(conn, ws.Config{
		Role:              ws.Server,
		Compression:       res.Compression,
		CompressionShared: true, // Many connections: share compressors, keep only windows.
	})
	if err != nil {
		conn.Close()
		return
	}
	cl := &client{conn: conn, queue: c.NewQueue(h.QueueLimit)}
	h.add(c, cl)
	defer h.remove(c)

	for {
		conn.SetReadDeadline(time.Now().Add(time.Minute))
		op, payload, err := c.ReadMessage()
		if err != nil {
			var ce *ws.CloseError
			if !errors.As(err, &ce) {
				log.Printf("%s: %v", conn.RemoteAddr(), err)
			}
			return
		}
		h.Broadcast(op, payload)
	}
}

// Broadcast sends one message to every client. It returns once the message
// is queued everywhere; the writes happen on the queues' goroutines.
func (h *Hub) Broadcast(op codec.Opcode, payload []byte) {
	p, err := ws.Prepare(op, payload)
	if err != nil {
		return
	}
	defer p.Release() // Queues hold their own references until written.

	h.mu.RLock()
	var slow []*ws.Conn
	for c, cl := range h.clients {
		if err := cl.queue.SendPrepared(p); err != nil {
			slow = append(slow, c) // ErrQueueFull or a sticky write error.
		}
	}
	h.mu.RUnlock()

	for _, c := range slow {
		h.remove(c)
	}
}

func (h *Hub) add(c *ws.Conn, cl *client) {
	h.mu.Lock()
	h.clients[c] = cl
	h.mu.Unlock()
}

// remove drops a client and closes its transport, which also ends its read loop.
func (h *Hub) remove(c *ws.Conn) {
	h.mu.Lock()
	cl, ok := h.clients[c]
	delete(h.clients, c)
	h.mu.Unlock()
	if ok {
		cl.conn.Close()
	}
}

// Len returns the number of connected clients.
func (h *Hub) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
