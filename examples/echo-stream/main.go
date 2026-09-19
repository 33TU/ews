// Command echo-stream echoes messages of any size in bounded memory. Each
// message is relayed frame by frame as it arrives instead of being
// assembled first, so a gigabyte message costs the same as a kilobyte one:
// two FragmentSize buffers in flight, and no MaxMessageSize to hit.
//
//	go run ./examples/echo-stream
//	go run ./examples/client -url ws://localhost:9002 -file big.bin
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
	addr := flag.String("addr", ":9002", "listen address")
	flag.Parse()

	server := &transport.Server{
		Handshake: handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}},
		Handler: func(conn net.Conn, res handshake.Result, _ *transport.Request) {
			c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression, FragmentSize: 64 << 10})
			if err != nil {
				log.Print(err)
				return
			}

			for {
				conn.SetDeadline(time.Now().Add(time.Minute))
				op, err := c.NextMessage()
				if err != nil {
					if _, ok := errors.AsType[*ws.CloseError](err); !ok {
						log.Printf("%s: %v", conn.RemoteAddr(), err)
					}
					return
				}

				// c is the reader: Read hands out the message's frames as they
				// arrive, inflated if compressed, and WriteFrom sends them on as
				// fragments of a new message until Read reports io.EOF.
				if _, err := c.WriteFrom(op, c); err != nil {
					log.Printf("%s: %v", conn.RemoteAddr(), err)
					return
				}
			}
		},
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("streaming echo server on %s", *addr)
	log.Fatal(server.Serve(ln))
}
