package echo

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
	"github.com/lxzan/gws"
)

// TestGwsDefaultInterop negotiates compression with a gws server using its
// default 12-bit windows with context takeover, which it imposes on the client
// through client_max_window_bits, and echoes through it.
func TestGwsDefaultInterop(t *testing.T) {
	up := gws.NewUpgrader(gwsEcho{}, &gws.ServerOption{PermessageDeflate: gws.PermessageDeflate{
		Enabled: true, ServerContextTakeover: true, ClientContextTakeover: true,
	}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if socket, err := up.Upgrade(w, r); err == nil {
			socket.ReadLoop()
		}
	}))
	defer srv.Close()

	opts := handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}}
	conn, res, err := transport.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http"), transport.DialOptions{Handshake: opts})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if c := res.Compression; c == nil || c.SendWindowBits != 12 || !c.SendContextTakeover || !c.ReceiveContextTakeover {
		t.Fatalf("negotiated %+v", res.Compression)
	}
	c, err := ws.NewConn(conn, ws.Config{Role: ws.Client, Compression: res.Compression})
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("twelve-bit windows on both sides "), 500)
	for i := 0; i < 3; i++ {
		if err := c.Write(codec.Text, payload); err != nil {
			t.Fatal(err)
		}
		op, p, err := c.ReadMessage()
		if err != nil || op != codec.Text || !bytes.Equal(p, payload) {
			t.Fatalf("message %d: %v", i, err)
		}
	}
}
