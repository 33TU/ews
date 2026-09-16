// Package harness holds what the benchmark packages share: an ews client
// dialer, payload generation, and a matching gws upgrader.
package harness

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
	"github.com/lxzan/gws"
)

// Mode is the compression mode a benchmark cell runs in. Its String is the
// value of the compress dimension in benchmark names.
type Mode int

const (
	// Plain negotiates no compression.
	Plain Mode = iota
	// Takeover is permessage-deflate with context takeover in both
	// directions: a 32 KB history per direction that every message extends.
	Takeover
	// NoTakeover is permessage-deflate with no_context_takeover in both
	// directions, each message compressed on its own. It is the only mode
	// gorilla/websocket supports, so it is where gorilla is compared.
	NoTakeover
)

func (m Mode) String() string {
	switch m {
	case Plain:
		return "false"
	case Takeover:
		return "true"
	}
	return "nocontext"
}

// Compressed reports whether the mode negotiates permessage-deflate.
func (m Mode) Compressed() bool { return m != Plain }

// Options returns the ews handshake options for the mode: level 1, every
// message compressed, takeover as the mode says.
func (m Mode) Options() handshake.Options {
	if m == Plain {
		return handshake.Options{}
	}
	return handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, MinSize: 1, ContextTakeover: m == Takeover}}
}

// Dial connects an ews client to url in the given mode and fails the test on
// any handshake problem.
func Dial(tb testing.TB, url string, mode Mode) *ws.Conn {
	tb.Helper()
	opts := mode.Options()
	req, err := handshake.NewRequest(opts)
	if err != nil {
		tb.Fatal(err)
	}
	addr := strings.TrimPrefix(url, "http://")
	nc, err := net.Dial("tcp", addr)
	if err != nil {
		tb.Fatal(err)
	}
	// Closing an httptest server does not close upgraded connections, so
	// each cell closes its own when it ends; reader goroutines then exit.
	tb.Cleanup(func() { nc.Close() })
	var b strings.Builder
	fmt.Fprintf(&b, "GET / HTTP/1.1\r\nHost: %s\r\nUpgrade: %s\r\nConnection: %s\r\nSec-WebSocket-Version: %s\r\nSec-WebSocket-Key: %s\r\n",
		addr, req.Upgrade, req.Connection, req.Version, req.Key)
	if req.Extensions != "" {
		fmt.Fprintf(&b, "Sec-WebSocket-Extensions: %s\r\n", req.Extensions)
	}
	b.WriteString("\r\n")
	if _, err := nc.Write([]byte(b.String())); err != nil {
		tb.Fatal(err)
	}
	br := bufio.NewReader(nc)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		tb.Fatal(err)
	}
	res, err := handshake.Confirm(req, handshake.Response{
		Status: resp.StatusCode, Upgrade: resp.Header.Get("Upgrade"), Connection: resp.Header.Get("Connection"),
		Accept: resp.Header.Get("Sec-WebSocket-Accept"), Extensions: resp.Header.Get("Sec-WebSocket-Extensions"),
	}, opts)
	if err != nil {
		tb.Fatal(err)
	}
	if mode.Compressed() && res.Compression == nil {
		tb.Fatal("compression not negotiated")
	}
	if mode.Compressed() && (res.Compression.SendContextTakeover || res.Compression.ReceiveContextTakeover) != (mode == Takeover) {
		tb.Fatalf("takeover negotiated as %+v in mode %s", res.Compression, mode)
	}
	c, err := ws.NewConn(struct {
		io.Reader
		io.Writer
	}{br, nc}, ws.Config{Role: ws.Client, MaxMessageSize: 64 << 20, Compression: res.Compression})
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

// Payload returns size bytes: repeated JSON-like text when compressible,
// otherwise deterministic random bytes.
func Payload(size int, compressible bool) []byte {
	if compressible {
		return bytes.Repeat([]byte("{\"type\":\"update\",\"value\":42,\"text\":\"hello world\"} "), size/48+1)[:size]
	}
	p := make([]byte, size)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range p {
		p[i] = byte(r.Uint32())
	}
	return p
}

// GwsUpgrader configures gws to match the ews servers: 15-bit windows,
// takeover as the mode says, level 1, compressing every message.
func GwsUpgrader(mode Mode, handler gws.Event) *gws.Upgrader {
	return gws.NewUpgrader(handler, &gws.ServerOption{
		ReadMaxPayloadSize: 64 << 20,
		PermessageDeflate: gws.PermessageDeflate{
			Enabled:               mode.Compressed(),
			ServerContextTakeover: mode == Takeover,
			ClientContextTakeover: mode == Takeover,
			ServerMaxWindowBits:   15, // Match the 32 KB window ews uses.
			ClientMaxWindowBits:   15,
			Level:                 flate.BestSpeed,
		},
	})
}
