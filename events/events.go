// Package events serves connections through a Handler interface instead of
// a read loop: a thin layer over transport and ws.Serve.
package events

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

// Conn is the connection the events receive: the WebSocket with its
// deadlines, addresses and UserData for the handler's per-connection state,
// and the upgrade request.
type Conn struct {
	*ws.Conn
	Request transport.Request
}

// Handler receives a connection's events on its read goroutine. Payloads
// are borrowed until the call returns; an error ends the connection and is
// passed to OnClose, as is the *ws.CloseError of a peer's close and, as a
// *PanicError, a panic in any other event. One Handler serves every
// connection; per-connection state goes in Conn.UserData. Embed Base for
// the defaults.
type Handler interface {
	OnOpen(c *Conn)
	OnMessage(c *Conn, op codec.Opcode, payload []byte) error
	OnPing(c *Conn, payload []byte) error // Must send the pong.
	OnPong(c *Conn, payload []byte) error
	OnClose(c *Conn, err error)
}

// PanicError is the error OnClose receives when another event panicked:
// the connection ends, the process does not. OnClose may re-panic to
// restore that behavior.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("ews/events: handler panicked: %v", e.Value) }

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
		run(h, ws.Server, cfg, conn, res, *req)
	}
}

// Dial connects to rawURL as a client and runs h on the connection until it
// ends, then closes the transport. It returns the error OnClose received,
// so a caller deciding whether to reconnect sees the same thing the handler
// did, or the dial error if no connection was made. Run it on a goroutine
// to keep working while the connection lives. Conn.Request carries the
// URL's host and path and the server's address, and cfg.UserData is in
// place before OnOpen, for state the caller has at dial time.
func Dial(ctx context.Context, rawURL string, opts transport.DialOptions, h Handler, cfg ws.Config) error {
	conn, res, err := transport.Dial(ctx, rawURL, opts)
	if err != nil {
		return err
	}
	defer conn.Close()
	req := transport.Request{Method: "GET", RemoteAddr: conn.RemoteAddr()}
	if u, err := url.Parse(rawURL); err == nil {
		req.Host, req.Path = u.Host, u.Path
	}
	return run(h, ws.Client, cfg, conn, res, req)
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
			run(h, ws.Server, cfg, conn, res, req)
		}()
	})
}

func run(h Handler, role ws.Role, cfg ws.Config, conn net.Conn, res handshake.Result, req transport.Request) error {
	ec := &Conn{Request: req}
	cfg.Role, cfg.Compression = role, res.Compression
	cfg.ControlHandler = controls{h: h, c: ec}

	c, err := ws.NewConn(conn, cfg)
	if err != nil {
		return err
	}
	ec.Conn = c

	err = serve(h, ec)
	h.OnClose(ec, err)
	return err
}

// serve runs OnOpen and the message loop, turning a panic in any event into
// the PanicError that ends the connection.
func serve(h Handler, ec *Conn) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = &PanicError{Value: v, Stack: debug.Stack()}
		}
	}()
	h.OnOpen(ec)
	return ws.Serve(ec.Conn, ws.MessageFunc(func(_ *ws.Conn, op codec.Opcode, payload []byte) error {
		return h.OnMessage(ec, op, payload)
	}))
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
