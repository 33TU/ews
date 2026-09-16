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

// client is one connection: the queue broadcasts go to, the connection for
// pings, and the transport for deadlines and closing.
type client struct {
	c    *ws.Conn
	conn net.Conn
}

// clients maps each client's queue to it, so a client that falls behind can
// be closed.
var clients sync.Map // *ws.Queue -> *client

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
				Role:              ws.Server,
				Compression:       res.Compression,
				CompressionShared: true,
				ControlHandler:    keepalive{conn: conn},
			})
			if err != nil {
				return
			}
			q := c.NewQueue(1 << 20) // A client more than 1 MiB behind is dropped.
			clients.Store(q, &client{c: c, conn: conn})
			defer clients.Delete(q)

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
// client. It returns as soon as the frames are queued; a client whose queue
// is full or failed is closed, which ends its handler.
func broadcast(op codec.Opcode, payload []byte) {
	p, err := ws.Prepare(op, payload)
	if err != nil {
		return
	}
	clients.Range(func(key, value any) bool {
		if err := key.(*ws.Queue).SendPrepared(p); err != nil {
			value.(*client).conn.Close()
		}
		return true
	})
}

// pingAll pings every client on one ticker. Ping is safe beside the queue's
// writer and a failed ping closes the client.
func pingAll() {
	for range time.Tick(pingInterval) {
		clients.Range(func(_, value any) bool {
			cl := value.(*client)
			if err := cl.c.Ping(nil); err != nil {
				cl.conn.Close()
			}
			return true
		})
	}
}
