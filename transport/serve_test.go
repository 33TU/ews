package transport_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

// startServer runs an echo Server on a fresh listener and returns its ws URL.
func startServer(t *testing.T, s *transport.Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if s.Handler == nil {
		s.Handler = func(conn net.Conn, res handshake.Result, req *transport.Request) {
			c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression})
			if err != nil {
				return
			}
			for {
				op, p, err := c.ReadMessage()
				if err != nil {
					return
				}
				if err := c.Write(op, p); err != nil {
					return
				}
			}
		}
	}
	go s.Serve(ln)
	t.Cleanup(func() { s.Close() })
	return "ws://" + ln.Addr().String()
}

func TestServe(t *testing.T) {
	var seen *transport.Request
	s := &transport.Server{
		Handshake: handshake.Options{Protocols: []string{"echo"}, Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}},
		Accept: func(req *transport.Request) int {
			seen = req
			if req.Path != "/socket" {
				return 404
			}
			return 0
		},
	}
	url := startServer(t, s)

	opts := handshake.Options{Protocols: []string{"echo"}, Compression: &handshake.Compress{Level: flate.BestSpeed}}
	conn, res, err := transport.Dial(context.Background(), url+"/socket", transport.DialOptions{Handshake: opts, Header: http.Header{"Origin": {"http://example.com"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if res.Protocol != "echo" || res.Compression == nil {
		t.Fatalf("negotiated %+v", res)
	}
	if seen == nil || seen.Method != "GET" || seen.Path != "/socket" || seen.Header["Origin"] != "http://example.com" || seen.Host == "" || seen.RemoteAddr == nil {
		t.Fatalf("request seen by Accept: %+v", seen)
	}
	c, _ := ws.NewConn(conn, ws.Config{Role: ws.Client, Compression: res.Compression})
	payload := bytes.Repeat([]byte("served without net/http "), 200)
	for i := range 3 {
		if err := c.Write(codec.Text, payload); err != nil {
			t.Fatal(err)
		}
		if op, p, err := c.ReadMessage(); err != nil || op != codec.Text || !bytes.Equal(p, payload) {
			t.Fatalf("round %d: %v", i, err)
		}
	}

	// Accept can refuse with a status.
	_, _, err = transport.Dial(context.Background(), url+"/elsewhere", transport.DialOptions{})
	if he, ok := errors.AsType[*transport.HandshakeError](err); !ok || he.Status != 404 {
		t.Fatalf("refused path: %v", err)
	}
}

// rawRequest sends bytes to the server and returns the status line and
// whether the connection was closed after the response.
func rawRequest(t *testing.T, url, request string) string {
	t.Helper()
	nc, err := net.Dial("tcp", strings.TrimPrefix(url, "ws://"))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	nc.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := nc.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(nc).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(line)
}

func TestServeRejects(t *testing.T) {
	url := startServer(t, &transport.Server{HandshakeTimeout: 2 * time.Second, MaxHeaderBytes: 512})
	key := handshake.NewKey()
	upgrade := "Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\n"
	tests := []struct {
		name, request, want string
	}{
		{"plain GET", "GET / HTTP/1.1\r\nHost: x\r\n\r\n", "HTTP/1.1 400 Bad Request"},
		{"old version", "GET / HTTP/1.1\r\nHost: x\r\n" + upgrade + "Sec-WebSocket-Version: 8\r\n\r\n", "HTTP/1.1 426 Upgrade Required"},
		{"bad request line", "GET /\r\n\r\n", "HTTP/1.1 400 Bad Request"},
		{"http/1.0", "GET / HTTP/1.0\r\nHost: x\r\n\r\n", "HTTP/1.1 400 Bad Request"},
		{"bad header", "GET / HTTP/1.1\r\nHost x\r\n\r\n", "HTTP/1.1 400 Bad Request"},
		{"head too large", "GET / HTTP/1.1\r\nHost: x\r\nX-Pad: " + strings.Repeat("a", 600) + "\r\n\r\n", "HTTP/1.1 431 Request Header Fields Too Large"},
		{"lowercase headers", "GET / HTTP/1.1\r\nhost: x\r\nupgrade: websocket\r\nconnection: keep-alive, upgrade\r\nsec-websocket-key: " + key + "\r\nsec-websocket-version: 13\r\n\r\n", "HTTP/1.1 101 Switching Protocols"},
		{"lf only", "GET / HTTP/1.1\nHost: x\n" + strings.ReplaceAll(upgrade, "\r\n", "\n") + "Sec-WebSocket-Version: 13\n\n", "HTTP/1.1 101 Switching Protocols"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rawRequest(t, url, tt.request); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}

	// A client that never finishes its request is dropped at the timeout.
	nc, err := net.Dial("tcp", strings.TrimPrefix(url, "ws://"))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	nc.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n"))
	nc.SetReadDeadline(time.Now().Add(10 * time.Second))
	start := time.Now()
	if _, err := nc.Read(make([]byte, 1)); err == nil || time.Since(start) > 8*time.Second {
		t.Fatalf("slow client not dropped: %v after %v", err, time.Since(start))
	}
}

// TestServeBufferedInput sends a frame in the same write as the request.
func TestServeBufferedInput(t *testing.T) {
	url := startServer(t, &transport.Server{})
	nc, err := net.Dial("tcp", strings.TrimPrefix(url, "ws://"))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	nc.SetDeadline(time.Now().Add(5 * time.Second))
	key := handshake.NewKey()
	var enc codec.Encoder
	enc.Encode(true, codec.Binary, []byte("early"), &[4]byte{1, 2, 3, 4})
	var wire bytes.Buffer
	wire.WriteString("GET / HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " + key + "\r\n\r\n")
	wire.Write(enc.HeaderBytes())
	wire.Write(enc.PayloadBytes())
	nc.Write(wire.Bytes())
	br := bufio.NewReader(nc)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 || resp.Header.Get("Sec-WebSocket-Accept") != handshake.Accept(key) {
		t.Fatalf("%v %v", resp, err)
	}
	c, _ := ws.NewConn(struct {
		io.Reader
		io.Writer
	}{br, nc}, ws.Config{Role: ws.Client})
	if op, p, err := c.ReadMessage(); err != nil || op != codec.Binary || string(p) != "early" {
		t.Fatalf("%d %q %v", op, p, err)
	}
}

func TestServeClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &transport.Server{Handler: func(net.Conn, handshake.Result, *transport.Request) {}}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve after Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
	if err := s.Serve(ln); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Serve on a closed server: %v", err)
	}
	if err := (&transport.Server{}).Serve(ln); err == nil {
		t.Fatal("nil handler accepted")
	}
}
