package transport_test

import (
	"context"
	"errors"
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
	"github.com/klauspost/compress/flate"
)

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func exchange(t *testing.T, conn net.Conn, res handshake.Result) {
	t.Helper()
	c, err := ws.NewConn(conn, ws.Config{Role: ws.Client, Compression: res.Compression})
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

func TestDial(t *testing.T) {
	srv := echoServer(t, handshake.Options{Protocols: []string{"echo"}, Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}})
	conn, res, err := transport.Dial(context.Background(), wsURL(srv), transport.DialOptions{
		Handshake: handshake.Options{Protocols: []string{"echo"}, Compression: &handshake.Compress{Level: flate.BestSpeed}},
		Header:    http.Header{"Origin": {"http://example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if res.Protocol != "echo" || res.Compression == nil || res.Compression.SendContextTakeover || res.Compression.ReceiveContextTakeover {
		t.Fatalf("result %+v", res)
	}
	exchange(t, conn, res)
}

func TestDialTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, res, err := transport.Upgrade(w, r, handshake.Options{})
		if err != nil {
			return
		}
		defer conn.Close()
		c, _ := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression})
		op, p, err := c.ReadMessage()
		if err == nil {
			c.Write(op, p)
		}
	}))
	defer srv.Close()
	cfg := srv.Client().Transport.(*http.Transport).TLSClientConfig
	conn, res, err := transport.Dial(context.Background(), "wss"+strings.TrimPrefix(srv.URL, "https"), transport.DialOptions{TLSConfig: cfg})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	exchange(t, conn, res)
}

func TestDialErrors(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Reason", "nope")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer plain.Close()
	_, _, err := transport.Dial(context.Background(), wsURL(plain), transport.DialOptions{})
	if he, ok := errors.AsType[*transport.HandshakeError](err); !ok || he.Status != 403 || he.Header.Get("X-Reason") != "nope" || !errors.Is(err, handshake.ErrBadStatus) {
		t.Fatalf("plain HTTP: %v", err)
	}

	if _, _, err := transport.Dial(context.Background(), "ftp://example.com", transport.DialOptions{}); err == nil {
		t.Fatal("bad scheme accepted")
	}

	// A server that never answers must respect the context.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err = transport.Dial(ctx, "ws://"+ln.Addr().String(), transport.DialOptions{})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("silent server: %v after %v", err, time.Since(start))
	}
}
