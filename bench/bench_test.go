// Package bench compares ews with other Go WebSocket libraries end to end:
// echo servers over loopback TCP driven by the same ews client. It is a
// separate module so the root module stays free of those dependencies.
//
//	cd bench && go test -run ^$ -bench . -benchtime=1s
package bench

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/33TU/ews"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
	"github.com/lxzan/gws"
)

// ---- servers -------------------------------------------------------------

func ewsServer(compress bool) *httptest.Server {
	var opts handshake.Options
	if compress {
		opts.Compression = &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, res, err := ews.Upgrade(w, r, opts)
		if err != nil {
			return
		}
		defer conn.Close()
		c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, MaxMessageSize: 64 << 20, Compression: res.Compression})
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
	}))
}

type gwsEcho struct{ gws.BuiltinEventHandler }

func (gwsEcho) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	socket.WriteMessage(message.Opcode, message.Bytes())
}

func gwsServer(compress bool) *httptest.Server {
	up := gws.NewUpgrader(gwsEcho{}, &gws.ServerOption{
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
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := up.Upgrade(w, r)
		if err != nil {
			return
		}
		socket.ReadLoop()
	}))
}

// ---- client (ews for both servers) ----------------------------------------

func dial(tb testing.TB, url string, compress bool) *ws.Conn {
	tb.Helper()
	var opts handshake.Options
	if compress {
		opts.Compression = &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}
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

// ---- benchmark -------------------------------------------------------------

func payload(size int, compressible bool) []byte {
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

func BenchmarkEcho(b *testing.B) {
	servers := []struct {
		name  string
		start func(bool) *httptest.Server
	}{{"ews", ewsServer}, {"gws", gwsServer}}
	for _, compress := range []bool{false, true} {
		for _, size := range []int{64, 1024, 16 << 10, 256 << 10} {
			for _, conns := range []int{1, 32} {
				for _, s := range servers {
					name := fmt.Sprintf("compress=%t/size=%d/conns=%d/%s", compress, size, conns, s.name)
					b.Run(name, func(b *testing.B) {
						srv := s.start(compress)
						defer srv.Close()
						msg := payload(size, compress)
						clients := make([]*ws.Conn, conns)
						for i := range clients {
							clients[i] = dial(b, srv.URL, compress)
						}
						// Warm up pools and negotiate once before measuring.
						for _, c := range clients {
							c.Write(codec.Binary, msg)
							c.ReadMessage()
						}
						per := b.N / conns
						b.ReportAllocs()
						b.SetBytes(int64(size))
						b.ResetTimer()
						var wg sync.WaitGroup
						errs := make(chan error, conns)
						for _, c := range clients {
							wg.Add(1)
							go func() {
								defer wg.Done()
								for range per {
									if err := c.Write(codec.Binary, msg); err != nil {
										errs <- err
										return
									}
									if _, p, err := c.ReadMessage(); err != nil || len(p) != size {
										errs <- fmt.Errorf("read: %v (%d bytes)", err, len(p))
										return
									}
								}
							}()
						}
						wg.Wait()
						b.StopTimer()
						select {
						case err := <-errs:
							b.Fatal(err)
						default:
						}
						for _, c := range clients {
							c.Close(1000, "")
						}
					})
				}
			}
		}
	}
}
