// Command echo-events is the echo server as an events.Handler, the shape a
// gws handler takes: one method per event and no read loop of your own.
// Every event is written out here to show the whole interface; a handler
// that wants only some of them embeds events.Base for the rest.
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

// echo serves every connection. The read deadline set on open is refreshed
// by each message and pong, so a silent peer is dropped after a minute.
type echo struct{}

func (echo) OnOpen(c *events.Conn) {
	log.Printf("%s connected", c.Request.RemoteAddr)
	c.SetReadDeadline(time.Now().Add(time.Minute))
	c.UserData = time.Now() // Anything you want to keep per connection.
}

func (echo) OnMessage(c *events.Conn, op codec.Opcode, payload []byte) error {
	c.SetReadDeadline(time.Now().Add(time.Minute))
	return c.Write(op, payload)
}

// OnPing owns the reply: a handler that takes pings must send the pong.
func (echo) OnPing(c *events.Conn, payload []byte) error {
	return c.Pong(payload)
}

func (echo) OnPong(c *events.Conn, _ []byte) error {
	return c.SetReadDeadline(time.Now().Add(time.Minute))
}

// OnClose is the last event: a *ws.CloseError when the peer closed
// cleanly, a *events.PanicError when a handler panicked, or the read error.
func (echo) OnClose(c *events.Conn, err error) {
	since := time.Since(c.UserData.(time.Time)).Round(time.Second)
	if _, ok := errors.AsType[*ws.CloseError](err); ok {
		log.Printf("%s closed after %s", c.Request.RemoteAddr, since)
		return
	}
	log.Printf("%s lost after %s: %v", c.Request.RemoteAddr, since, err)
}

func main() {
	addr := flag.String("addr", ":9005", "listen address")
	flag.Parse()

	server := &transport.Server{
		Handshake: handshake.Options{
			Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true},
		},
		Handler: events.Serve(echo{}, ws.Config{MaxMessageSize: 32 << 20, ValidateUTF8: true}),
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("event echo server on %s", *addr)
	log.Fatal(server.Serve(ln))
}
