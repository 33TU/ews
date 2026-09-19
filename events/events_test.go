package events_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/events"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

// recorder notes every event and echoes messages; its OnPing tags the pong.
type recorder struct {
	opened, ponged int
	closeErr       chan error
}

func (r *recorder) OnOpen(c *events.Conn) {
	r.opened++
	c.UserData = c.Request.Path
}
func (r *recorder) OnMessage(c *events.Conn, op codec.Opcode, payload []byte) error {
	return c.Write(op, append([]byte(c.UserData.(string)+" "), payload...))
}
func (r *recorder) OnPing(c *events.Conn, payload []byte) error {
	return c.Pong(append([]byte("tagged "), payload...))
}
func (r *recorder) OnPong(*events.Conn, []byte) error { r.ponged++; return nil }
func (r *recorder) OnClose(_ *events.Conn, err error) { r.closeErr <- err }

// pongs records pongs on the client side.
type pongs struct {
	ws.DefaultControlHandler
	got chan []byte
}

func (p pongs) OnPong(_ *ws.Conn, payload []byte) error {
	p.got <- append([]byte(nil), payload...)
	return nil
}

// tcpPair connects two ends over loopback, so a write never waits on the
// peer's read as it would on net.Pipe.
func tcpPair(t *testing.T) (server, client net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server = <-accepted
	if server == nil {
		t.Fatal("accept failed")
	}
	t.Cleanup(func() { server.Close(); client.Close() })
	return server, client
}

func TestServe(t *testing.T) {
	sc, cc := tcpPair(t)
	cc.SetDeadline(time.Now().Add(10 * time.Second))
	h := &recorder{closeErr: make(chan error, 1)}
	handler := events.Serve(h, ws.Config{})
	go handler(sc, handshake.Result{}, &transport.Request{Path: "/room"})

	p := pongs{got: make(chan []byte, 1)}
	client, err := ws.NewConn(cc, ws.Config{Role: ws.Client, ControlHandler: p})
	if err != nil {
		t.Fatal(err)
	}

	// A message comes back through OnMessage with the state OnOpen stored.
	if err := client.Write(codec.Text, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if _, got, err := client.ReadMessage(); err != nil || string(got) != "/room hi" {
		t.Fatalf("echo: %q, %v", got, err)
	}

	// A ping reaches OnPing, which answers with its own pong; the client's
	// pong reaches OnPong.
	if err := client.Ping([]byte("p")); err != nil {
		t.Fatal(err)
	}
	if err := client.Pong(nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Write(codec.Text, []byte("x")); err != nil { // Drives the server's reads.
		t.Fatal(err)
	}
	if _, _, err := client.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-p.got:
		if string(got) != "tagged p" {
			t.Fatalf("pong payload %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no pong")
	}

	// A clean close ends with OnClose holding the peer's CloseError.
	if err := client.Close(1000, "bye"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.ReadMessage(); err == nil { // The echoed close.
		t.Fatal("expected the close echo")
	}
	select {
	case err := <-h.closeErr:
		if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1000 || ce.Reason != "bye" {
			t.Fatalf("OnClose got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnClose not called")
	}
	if h.opened != 1 || h.ponged != 1 {
		t.Fatalf("opened %d, ponged %d", h.opened, h.ponged)
	}
}

// panicker panics on the first message; the peer sees the connection end
// and OnClose sees the PanicError.
type panicker struct {
	events.Base
	closeErr chan error
}

func (p *panicker) OnMessage(*events.Conn, codec.Opcode, []byte) error { panic("boom") }
func (p *panicker) OnClose(_ *events.Conn, err error)                  { p.closeErr <- err }

func TestPanicEndsConnection(t *testing.T) {
	sc, cc := tcpPair(t)
	cc.SetDeadline(time.Now().Add(10 * time.Second))
	h := &panicker{closeErr: make(chan error, 1)}
	done := make(chan struct{})
	go func() {
		events.Serve(h, ws.Config{})(sc, handshake.Result{}, &transport.Request{})
		sc.Close() // As transport.Server does when the handler returns.
		close(done)
	}()
	client, err := ws.NewConn(cc, ws.Config{Role: ws.Client})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Write(codec.Text, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-h.closeErr:
		pe, ok := errors.AsType[*events.PanicError](err)
		if !ok || pe.Value != "boom" || len(pe.Stack) == 0 {
			t.Fatalf("OnClose got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnClose not called")
	}
	<-done
	if _, _, err := client.ReadMessage(); err == nil {
		t.Fatal("connection still open after the panic")
	}
}

// greeter is a client handler: it sends one message on open, closes when
// the echo comes back, and records what it saw.
type greeter struct {
	events.Base
	got  chan string
	host string
}

func (g *greeter) OnOpen(c *events.Conn) {
	g.host = c.Request.Host
	c.Write(codec.Text, []byte("hello"))
}

func (g *greeter) OnMessage(c *events.Conn, _ codec.Opcode, payload []byte) error {
	g.got <- string(payload)
	return c.Close(1000, "seen")
}

func TestDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &transport.Server{Handler: events.Serve(echoHandler{}, ws.Config{})}
	go srv.Serve(ln)
	defer srv.Close()

	g := &greeter{got: make(chan string, 1)}
	url := "ws://" + ln.Addr().String() + "/room"
	err = events.Dial(context.Background(), url, transport.DialOptions{}, g, ws.Config{})
	if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1000 {
		t.Fatalf("Dial returned %v", err)
	}
	if got := <-g.got; got != "hello" {
		t.Fatalf("echo %q", got)
	}
	if g.host != ln.Addr().String() {
		t.Fatalf("Request.Host %q", g.host)
	}
	if _, err := net.Dial("tcp", "127.0.0.1:1"); err == nil {
		t.Skip("port 1 answers")
	}
	if err := events.Dial(context.Background(), "ws://127.0.0.1:1", transport.DialOptions{}, g, ws.Config{}); err == nil {
		t.Fatal("dial to a closed port succeeded")
	}
}

// echoHandler echoes and answers pings through Base.
type echoHandler struct{ events.Base }

func (echoHandler) OnMessage(c *events.Conn, op codec.Opcode, payload []byte) error {
	return c.Write(op, payload)
}
