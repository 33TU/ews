package transport_test

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

// echoServer upgrades every request and echoes one message.
func echoServer(t *testing.T, opts handshake.Options) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "session=1")
		conn, res, err := transport.Upgrade(w, r, opts)
		if err != nil {
			return
		}
		defer conn.Close()
		c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression})
		if err != nil {
			t.Error(err)
			return
		}
		op, p, err := c.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		if err := c.Write(op, p); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUpgrade(t *testing.T) {
	srv := echoServer(t, handshake.Options{Protocols: []string{"echo"}})
	opts := handshake.Options{Protocols: []string{"echo", "other"}}
	hreq, err := handshake.NewRequest(opts)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(hreq.Method, srv.URL, nil)
	req.Header.Set("Upgrade", hreq.Upgrade)
	req.Header.Set("Connection", hreq.Connection)
	req.Header.Set("Sec-WebSocket-Version", hreq.Version)
	req.Header.Set("Sec-WebSocket-Key", hreq.Key)
	req.Header.Set("Sec-WebSocket-Protocol", hreq.Protocols)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	res, err := handshake.Confirm(hreq, handshake.Response{
		Status:     resp.StatusCode,
		Upgrade:    resp.Header.Get("Upgrade"),
		Connection: resp.Header.Get("Connection"),
		Accept:     resp.Header.Get("Sec-WebSocket-Accept"),
		Extensions: resp.Header.Get("Sec-WebSocket-Extensions"),
		Protocol:   resp.Header.Get("Sec-WebSocket-Protocol"),
	}, opts)
	if err != nil || res.Protocol != "echo" || res.Compression != nil {
		t.Fatalf("%+v %v", res, err)
	}
	if resp.Header.Get("Set-Cookie") != "session=1" {
		t.Fatal("extra header not forwarded")
	}
	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		t.Fatal("no protocol switch body")
	}
	c, err := ws.NewConn(bodyConn{rwc}, ws.Config{Role: ws.Client})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(codec.Text, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	op, p, err := c.ReadMessage()
	if err != nil || op != codec.Text || string(p) != "hello" {
		t.Fatalf("%d %q %v", op, p, err)
	}
}

func TestUpgradeRejects(t *testing.T) {
	srv := echoServer(t, handshake.Options{})
	for _, tt := range []struct {
		name    string
		headers map[string]string
		status  int
	}{
		{"plain GET", nil, 400},
		{"old version", map[string]string{"Upgrade": "websocket", "Connection": "Upgrade", "Sec-WebSocket-Version": "8", "Sec-WebSocket-Key": handshake.NewKey()}, 426},
		{"bad key", map[string]string{"Upgrade": "websocket", "Connection": "Upgrade", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "x"}, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", srv.URL, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Fatalf("status %d, want %d", resp.StatusCode, tt.status)
			}
			if tt.status == 426 && resp.Header.Get("Sec-WebSocket-Version") != "13" {
				t.Fatal("426 without supported version")
			}
		})
	}
}

// TestUpgradeBufferedInput sends a frame in the same write as the request, so
// the HTTP server buffers it before Upgrade hands over the connection.
func TestUpgradeBufferedInput(t *testing.T) {
	srv := echoServer(t, handshake.Options{})
	nc, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	nc.SetDeadline(time.Now().Add(5 * time.Second))

	key := handshake.NewKey()
	var enc codec.Encoder
	if err := enc.Encode(true, codec.Binary, []byte("early"), &[4]byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	wire.WriteString(upgradeRequest(key))
	wire.Write(enc.HeaderBytes())
	wire.Write(enc.PayloadBytes())
	if _, err := nc.Write(wire.Bytes()); err != nil {
		t.Fatal(err)
	}

	br := bufio.NewReader(nc)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 || resp.Header.Get("Sec-WebSocket-Accept") != handshake.Accept(key) {
		t.Fatalf("%v %v", resp, err)
	}
	c, err := ws.NewConn(bufConn{nc, br}, ws.Config{Role: ws.Client})
	if err != nil {
		t.Fatal(err)
	}
	op, p, err := c.ReadMessage()
	if err != nil || op != codec.Binary || string(p) != "early" {
		t.Fatalf("%d %q %v", op, p, err)
	}
}

// upgradeRequest is a minimal client handshake for key, written raw so the
// test controls what follows it on the wire.
func upgradeRequest(key string) string {
	return "GET / HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " + key + "\r\n\r\n"
}
