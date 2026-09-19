// Command echo-queue echoes through a Queue. The read loop never waits on
// the socket: Send copies the frame and returns, and a writer goroutine
// that runs only while the queue is nonempty writes what has accumulated,
// coalescing a burst into one writev. A peer that stops reading is dropped
// once its backlog passes the limit instead of stalling the reader, which
// is the shape a server wants when replies are not tied to requests.
//
//	go run ./examples/echo-queue
//	go run ./examples/client -url ws://localhost:9003
package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"time"

	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

func main() {
	addr := flag.String("addr", ":9003", "listen address")
	limit := flag.Int("limit", 1<<20, "bytes a peer may fall behind before it is dropped")
	flag.Parse()

	server := &transport.Server{
		Handshake: handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}},
		Handler: func(conn net.Conn, res handshake.Result, _ *transport.Request) {
			c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression, ValidateUTF8: true})
			if err != nil {
				log.Print(err)
				return
			}

			// From here on every write, including the close echo, goes through
			// the queue's goroutine, so only the read deadline is set: a write
			// deadline would fire on the writer while the peer is merely slow.
			q := c.NewQueue(*limit)

			for {
				conn.SetReadDeadline(time.Now().Add(time.Minute))
				op, p, err := c.ReadMessage()
				if err != nil {
					// A peer close arrives after its echo has gone out behind
					// the queued replies; Close waits for its own frame.
					if _, ok := errors.AsType[*ws.CloseError](err); !ok {
						log.Printf("%s: %v", conn.RemoteAddr(), err)
					}
					return
				}

				if err := q.Send(op, p); err != nil { // Copies p; the borrowed payload is not retained.
					if errors.Is(err, ws.ErrQueueFull) {
						log.Printf("%s: %d bytes behind, dropping", conn.RemoteAddr(), q.Pending())
					}
					return // The server closes conn, which ends the writer.
				}
			}
		},
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("queued echo server on %s", *addr)
	log.Fatal(server.Serve(ln))
}
