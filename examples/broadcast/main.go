// Command broadcast runs a chat-style hub: every message a client sends is
// relayed to all connected clients. Try it with two terminals:
//
//	go run ./examples/broadcast
//	websocat ws://localhost:8080/
//	websocat ws://localhost:8080/
package main

import (
	"flag"
	"log"
	"net"
	"sync"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

// A client that sends nothing and answers no ping within idleTimeout is
// dropped. Pings go out from one ticker for every client, so keepalive
// costs no timer per connection.
const (
	pingInterval = 30 * time.Second
	idleTimeout  = 2 * pingInterval
)

// client is one connection: the connection for pings, the queue broadcasts
// go to, and the transport for deadlines and closing.
type client struct {
	c    *ws.Conn
	q    *ws.Queue
	conn net.Conn
}

var clients sync.Map // Set of *client.

// each calls f on every client and closes those it fails for, which ends
// their handlers. No lock is held while f runs, so a ping blocked on one
// slow peer delays nobody else.
func each(f func(*client) error) {
	clients.Range(func(key, _ any) bool {
		if cl := key.(*client); f(cl) != nil {
			cl.conn.Close()
		}
		return true
	})
}

// keepalive refreshes the read deadline when a pong arrives, so a quiet but
// live client stays connected.
type keepalive struct {
	ws.DefaultControlHandler
	conn net.Conn
}

func (k keepalive) OnPong(*ws.Conn, []byte) error {
	return k.conn.SetReadDeadline(time.Now().Add(idleTimeout))
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	server := &transport.Server{
		Handshake: handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}},
		Handler: func(conn net.Conn, res handshake.Result, _ *transport.Request) {
			c, err := ws.NewConn(conn, ws.Config{
				Role:        ws.Server,
				Compression: res.Compression,
				// Broadcasts are compressed once in Prepare, so per-connection
				// compressors would sit idle at 800 KB each; share them.
				CompressionShared: true,
				ControlHandler:    keepalive{conn: conn},
			})
			if err != nil {
				return
			}
			cl := &client{c: c, q: c.NewQueue(1 << 20), conn: conn} // More than 1 MiB behind: dropped.
			clients.Store(cl, nil)
			defer clients.Delete(cl)

			for {
				conn.SetReadDeadline(time.Now().Add(idleTimeout))
				op, payload, err := c.ReadMessage()
				if err != nil {
					return
				}
				broadcast(op, payload)
			}
		},
	}
	go pingAll()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("broadcast hub on %s", *addr)
	log.Fatal(server.Serve(ln))
}

// broadcast compresses and encodes the message once and queues it to every
// client, returning as soon as the frames are queued. A client whose queue
// is full or failed is closed.
func broadcast(op codec.Opcode, payload []byte) {
	p, err := ws.Prepare(op, payload)
	if err != nil {
		return
	}
	each(func(cl *client) error { return cl.q.SendPrepared(p) })
}

// pingAll pings every client from one ticker. Ping writes beside the
// queue's writer, and a failed ping closes the client.
func pingAll() {
	for range time.Tick(pingInterval) {
		each(func(cl *client) error { return cl.c.Ping(nil) })
	}
}
