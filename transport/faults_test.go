package transport_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
)

func TestDialMoreErrors(t *testing.T) {
	ctx := context.Background()
	if _, _, err := transport.Dial(ctx, "://bad", transport.DialOptions{}); err == nil {
		t.Fatal("bad URL accepted")
	}
	if _, _, err := transport.Dial(ctx, "ftp://example.com/", transport.DialOptions{}); err == nil {
		t.Fatal("unsupported scheme accepted")
	}
	// A refused connection, through a dialer with a short timeout; the
	// default port is filled in for a URL without one.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	if _, _, err := transport.Dial(ctx, "ws://"+addr, transport.DialOptions{Dialer: &net.Dialer{Timeout: time.Second}}); err == nil {
		t.Fatal("dial to a closed port succeeded")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := transport.Dial(cancelled, "ws://127.0.0.1", transport.DialOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: %v", err)
	}

	// A server that answers without upgrading.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no websockets here", http.StatusForbidden)
	}))
	defer srv.Close()
	_, _, err = transport.Dial(ctx, wsURL(srv), transport.DialOptions{})
	var he *transport.HandshakeError
	if !errors.As(err, &he) || he.Status != http.StatusForbidden || !strings.Contains(he.Error(), "403") {
		t.Fatalf("non-101 response: %v", err)
	}
	// A server that speaks nonsense.
	nonsense, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer nonsense.Close()
	go func() {
		c, err := nonsense.Accept()
		if err != nil {
			return
		}
		c.Write([]byte("not http at all\r\n\r\n"))
		c.Close()
	}()
	if _, _, err := transport.Dial(ctx, "ws://"+nonsense.Addr().String(), transport.DialOptions{}); err == nil {
		t.Fatal("garbage response accepted")
	}
}

// TestDialTLSUntrusted covers the TLS handshake failing: TestDialTLS has the
// trusted case.
func TestDialTLSUntrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	url := "wss://" + strings.TrimPrefix(srv.URL, "https://")
	if _, _, err := transport.Dial(context.Background(), url, transport.DialOptions{}); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	// A TLS config without a server name gets the URL's host.
	cfg := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	cfg.ServerName = ""
	if _, _, err := transport.Dial(context.Background(), url, transport.DialOptions{TLSConfig: cfg}); err == nil {
		t.Fatal("a server that never upgrades was accepted")
	} else if _, ok := err.(*transport.HandshakeError); !ok {
		t.Fatalf("TLS with the host filled in should reach the handshake: %v", err)
	}
}

func TestServeMoreRejects(t *testing.T) {
	var seen *transport.Request
	s := &transport.Server{
		MaxHeaderBytes: 300,
		Accept: func(req *transport.Request) int {
			seen = req
			if req.Path == "/teapot" {
				return 418
			}
			return 0
		},
		Handler: func(conn net.Conn, res handshake.Result, req *transport.Request) { conn.Close() },
	}
	url := startServer(t, s)
	key := "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
	for _, tc := range []struct{ name, request, want string }{
		{"bad request line", "GARBAGE\r\n\r\n", "400"},
		{"http/1.0", "GET / HTTP/1.0\r\nHost: x\r\n\r\n", "400"},
		{"bad header", "GET / HTTP/1.1\r\nHost: x\r\nNo Colon Here\r\n\r\n", "400"},
		{"space in name", "GET / HTTP/1.1\r\nHost: x\r\nBad Name: v\r\n\r\n", "400"},
		{"head too large", "GET / HTTP/1.1\r\nHost: x\r\nX-Pad: " + strings.Repeat("p", 400) + "\r\n\r\n", "431"},
		{"accept hook status without a text", "GET /teapot HTTP/1.1\r\nHost: x\r\n" + key + "\r\n", "418 Error"},
		{"not an upgrade", "GET / HTTP/1.1\r\nHost: x\r\n\r\n", "400"},
		{"wrong version", "GET / HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 12\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", "426"},
	} {
		got := rawRequest(t, url, tc.request)
		if !strings.Contains(strings.SplitN(got, "\r\n", 2)[0], tc.want) {
			t.Fatalf("%s: %q", tc.name, strings.SplitN(got, "\r\n", 2)[0])
		}
	}
	// Duplicate headers are joined, as net/http would report them.
	rawRequest(t, url, "GET /teapot HTTP/1.1\r\nHost: x\r\nX-Dup: a\r\nX-Dup: b\r\n"+key+"\r\n")
	if seen == nil || seen.Header["X-Dup"] != "a, b" || seen.Host != "x" {
		t.Fatalf("request not parsed as expected: %+v", seen)
	}
}

func TestUpgradeEdges(t *testing.T) {
	// A ResponseWriter that cannot hijack.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	rec := httptest.NewRecorder()
	if _, _, err := transport.Upgrade(rec, req, handshake.Options{}); err != transport.ErrNotHijackable || rec.Code != http.StatusInternalServerError {
		t.Fatalf("recorder upgrade: %v, %d", err, rec.Code)
	}
	// Handler headers pass through the 101 except the ones Upgrade owns.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Transfer-Encoding", "chunked")
		conn, _, err := transport.Upgrade(w, r, handshake.Options{})
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer srv.Close()
	got := rawHead(t, wsURL(srv), "GET / HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	if !strings.HasPrefix(got, "HTTP/1.1 101") || !strings.Contains(got, "X-Custom: yes") || strings.Contains(got, "Content-Type") || strings.Contains(got, "Transfer-Encoding") {
		t.Fatalf("101 headers: %q", got)
	}
}

// rawHead sends a request and returns the whole response head.
func rawHead(t *testing.T, url, request string) string {
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
	br := bufio.NewReader(nc)
	var head strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		head.WriteString(line)
		if line == "\r\n" {
			return head.String()
		}
	}
}
