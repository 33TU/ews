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
	"net/http"
	"sync"

	"github.com/33TU/ews"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

// clients maps each connection's queue to its transport, so a client that
// falls behind can be closed.
var clients sync.Map // *ws.Queue -> net.Conn

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	opts := handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, res, err := ews.Upgrade(w, r, opts)
		if err != nil {
			return
		}
		defer conn.Close()
		c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression, CompressionShared: true})
		if err != nil {
			return
		}
		q := c.NewQueue(1 << 20) // A client more than 1 MiB behind is dropped.
		clients.Store(q, conn)
		defer clients.Delete(q)

		for {
			op, payload, err := c.ReadMessage()
			if err != nil {
				return
			}
			broadcast(op, payload)
		}
	})
	log.Printf("broadcast hub on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// broadcast compresses and encodes the message once and queues it to every
// client. It returns as soon as the frames are queued; a client whose queue
// is full or failed is closed, which ends its handler.
func broadcast(op codec.Opcode, payload []byte) {
	p, err := ws.Prepare(op, payload)
	if err != nil {
		return
	}
	defer p.Release() // Queues keep their own references until written.
	clients.Range(func(key, value any) bool {
		if err := key.(*ws.Queue).SendPrepared(p); err != nil {
			value.(net.Conn).Close()
		}
		return true
	})
}
