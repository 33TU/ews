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

// Dial connects an ews client to url, negotiating compression when asked,
// and fails the test on any handshake problem.
func Dial(tb testing.TB, url string, compress bool) *ws.Conn {
	tb.Helper()
	var opts handshake.Options
	if compress {
		opts.Compression = &handshake.Compress{Level: flate.BestSpeed, MinSize: 1, ContextTakeover: true}
	}
	req, err := handshake.NewRequest(opts)
	if err != nil {
		tb.Fatal(err)
	}
	addr := strings.TrimPrefix(url, "http://")
	nc, err := net.Dial("tcp", addr)
	if err != nil {
		tb.Fatal(err)
	}
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
	if compress && res.Compression == nil {
		tb.Fatal("compression not negotiated")
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
// context takeover, level 1, compressing every message.
func GwsUpgrader(compress bool, handler gws.Event) *gws.Upgrader {
	return gws.NewUpgrader(handler, &gws.ServerOption{
		ReadMaxPayloadSize: 64 << 20,
		PermessageDeflate: gws.PermessageDeflate{
			Enabled:               compress,
			ServerContextTakeover: true,
			ClientContextTakeover: true,
			ServerMaxWindowBits:   15, // Match the 32 KB window ews uses.
			ClientMaxWindowBits:   15,
			Level:                 flate.BestSpeed,
		},
	})
}
