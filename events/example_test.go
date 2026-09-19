package events_test

import (
	"log"
	"net"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/events"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

// echo is a gws-style handler, one instance shared by every connection. A
// pong from the peer refreshes the deadline set on open: the usual
// keepalive, with pings from one ticker elsewhere. Base answers pings.
type echo struct{ events.Base }

func (echo) OnOpen(c *events.Conn) {
	log.Printf("%s connected to %s", c.Request.RemoteAddr, c.Request.Path)
	c.Transport.SetReadDeadline(time.Now().Add(time.Minute))
}

func (echo) OnMessage(c *events.Conn, op codec.Opcode, payload []byte) error {
	return c.Write(op, payload)
}

func (echo) OnPong(c *events.Conn, _ []byte) error {
	return c.Transport.SetReadDeadline(time.Now().Add(time.Minute))
}

func (echo) OnClose(c *events.Conn, err error) { log.Printf("closed: %v", err) }

func Example() {
	server := &transport.Server{Handler: events.Serve(echo{}, ws.Config{ValidateUTF8: true})}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go server.Serve(ln)
	server.Close()
	// Output:
}
