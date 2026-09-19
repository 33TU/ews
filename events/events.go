// Package events serves connections through a Handler interface instead of
// a read loop: a thin layer over transport and ws.Serve.
package events

import (
	"net"
	"net/http"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

// Conn is the connection the events receive. Value is free for the
// handler's per-connection state.
type Conn struct {
	*ws.Conn
	Transport net.Conn
	Request   transport.Request
	Value     any
}

// Handler receives a connection's events on its read goroutine. Payloads
// are borrowed until the call returns; an error ends the connection and is
// passed to OnClose, as is the *ws.CloseError of a peer's close.
// One Handler serves every connection. Embed Base for the defaults.
type Handler interface {
	OnOpen(c *Conn)
	OnMessage(c *Conn, op codec.Opcode, payload []byte) error
	OnPing(c *Conn, payload []byte) error // Must send the pong.
	OnPong(c *Conn, payload []byte) error
	OnClose(c *Conn, err error)
}

// Base is a Handler that answers pings and ignores the rest.
type Base struct{}

func (Base) OnOpen(*Conn)                         {}
func (Base) OnPing(c *Conn, payload []byte) error { return c.Pong(payload) }
func (Base) OnPong(*Conn, []byte) error           { return nil }
func (Base) OnClose(*Conn, error)                 {}

// Serve returns a transport.Server handler that runs h on each connection.
// cfg's Role, Compression and ControlHandler are set per connection.
func Serve(h Handler, cfg ws.Config) func(net.Conn, handshake.Result, *transport.Request) {
	return func(conn net.Conn, res handshake.Result, req *transport.Request) {
		run(h, cfg, conn, res, *req)
	}
}

// HTTP returns an http.Handler that upgrades each request and runs h on a
// goroutine of its own, so net/http frees the request.
func HTTP(h Handler, opts handshake.Options, cfg ws.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, res, err := transport.Upgrade(w, r, opts)
		if err != nil {
			return // Upgrade wrote the HTTP error.
		}
		req := transport.Request{Method: r.Method, Path: r.URL.Path, Host: r.Host, RemoteAddr: conn.RemoteAddr()}
		go func() {
			defer conn.Close()
			run(h, cfg, conn, res, req)
		}()
	})
}

func run(h Handler, cfg ws.Config, conn net.Conn, res handshake.Result, req transport.Request) {
	ec := &Conn{Transport: conn, Request: req}
	cfg.Role, cfg.Compression = ws.Server, res.Compression
	cfg.ControlHandler = controls{h: h, c: ec}

	c, err := ws.NewConn(conn, cfg)
	if err != nil {
		return
	}
	ec.Conn = c

	h.OnOpen(ec)
	h.OnClose(ec, ws.Serve(c, ws.MessageFunc(func(_ *ws.Conn, op codec.Opcode, payload []byte) error {
		return h.OnMessage(ec, op, payload)
	})))
}

// controls routes pings and pongs to the Handler; close frames keep the
// default echo.
type controls struct {
	ws.DefaultControlHandler
	h Handler
	c *Conn
}

func (k controls) OnPing(_ *ws.Conn, payload []byte) error { return k.h.OnPing(k.c, payload) }
func (k controls) OnPong(_ *ws.Conn, payload []byte) error { return k.h.OnPong(k.c, payload) }
