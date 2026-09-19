// Command echo-events is the echo server as an events.Handler, the shape a
// gws handler takes: one method per event and no read loop of your own.
// The read deadline set on open is refreshed by every message and pong.
//
//	go run ./examples/echo-events
//	go run ./examples/client -url ws://localhost:9005
package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/events"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

// echo serves every connection; Base answers pings.
type echo struct{ events.Base }

func (echo) OnOpen(c *events.Conn) {
	log.Printf("%s connected", c.Request.RemoteAddr)
	c.SetReadDeadline(time.Now().Add(time.Minute))
}

func (echo) OnMessage(c *events.Conn, op codec.Opcode, payload []byte) error {
	c.SetReadDeadline(time.Now().Add(time.Minute))
	return c.Write(op, payload)
}

func (echo) OnPong(c *events.Conn, _ []byte) error {
	return c.SetReadDeadline(time.Now().Add(time.Minute))
}

func (echo) OnClose(c *events.Conn, err error) {
	if _, ok := errors.AsType[*ws.CloseError](err); !ok {
		log.Printf("%s: %v", c.Request.RemoteAddr, err)
	}
}

func main() {
	addr := flag.String("addr", ":9005", "listen address")
	flag.Parse()

	server := &transport.Server{
		Handshake: handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}},
		Handler:   events.Serve(echo{}, ws.Config{MaxMessageSize: 32 << 20, ValidateUTF8: true}),
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("event echo server on %s", *addr)
	log.Fatal(server.Serve(ln))
}
