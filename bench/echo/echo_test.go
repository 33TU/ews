// Package bench compares ews with other Go WebSocket libraries end to end:
// echo servers over loopback TCP driven by the same ews client. It is a
// separate module so the root module stays free of those dependencies.
//
//	cd bench && go test -run ^$ -bench . -benchtime=1s | go run ./cmd/results > RESULTS.md
package echo

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/33TU/ews"
	"github.com/33TU/ews/bench/internal/harness"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/coder/websocket"
	"github.com/klauspost/compress/flate"
	"github.com/lxzan/gws"
)

// Echo servers for each library, all driven by the same ews client.

func ewsServer(compress bool) *httptest.Server { return ewsServerWith(compress, false) }

// ewsSharedServer borrows a pooled compressor per message like gws and coder
// do, instead of keeping one attached per connection.
func ewsSharedServer(compress bool) *httptest.Server { return ewsServerWith(compress, true) }

func ewsServerWith(compress, shared bool) *httptest.Server {
	var opts handshake.Options
	if compress {
		opts.Compression = &handshake.Compress{Level: flate.BestSpeed, MinSize: 1, ContextTakeover: true}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, res, err := ews.Upgrade(w, r, opts)
		if err != nil {
			return
		}
		defer conn.Close()
		c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, MaxMessageSize: 64 << 20, Compression: res.Compression, CompressionShared: shared})
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

// gwsEcho is the event handler for the interop test's ReadLoop server.
type gwsEcho struct{ gws.BuiltinEventHandler }

func (gwsEcho) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	socket.WriteMessage(message.Opcode, message.Bytes())
}

// gwsServer echoes through gws's ReadMessage and WriteMessage, the
// like-for-like shape against ews. gws's ReadLoop shares the whole frame path
// and measured the same within noise.
func gwsServer(compress bool) *httptest.Server {
	up := harness.GwsUpgrader(compress, gws.BuiltinEventHandler{})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := up.Upgrade(w, r)
		if err != nil {
			return
		}
		for {
			msg, err := socket.ReadMessage()
			if err != nil {
				return
			}
			socket.WriteMessage(msg.Opcode, msg.Bytes())
			msg.Close()
		}
	}))
}

// gwsStreamServer pipes gws's NextReader into WriteFile, its streaming shape:
// no message is held whole.
func gwsStreamServer(compress bool) *httptest.Server {
	up := harness.GwsUpgrader(compress, gws.BuiltinEventHandler{})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := up.Upgrade(w, r)
		if err != nil {
			return
		}
		for {
			typ, rd, err := socket.NextReader()
			if err != nil {
				return
			}
			if err := socket.WriteFile(typ, rd); err != nil {
				return
			}
		}
	}))
}

func coderServer(compress bool) *httptest.Server {
	mode := websocket.CompressionDisabled
	if compress {
		mode = websocket.CompressionContextTakeover
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Threshold 1 compresses every message, as the other servers do.
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: mode, CompressionThreshold: 1})
		if err != nil {
			return
		}
		defer c.CloseNow()
		c.SetReadLimit(64 << 20)
		ctx := r.Context()
		for {
			typ, p, err := c.Read(ctx)
			if err != nil {
				return
			}
			if err := c.Write(ctx, typ, p); err != nil {
				return
			}
		}
	}))
}

// coderStreamServer echoes through coder's streaming Reader and Writer with a
// reusable buffer, its most efficient shape: no message is held whole.
func coderStreamServer(compress bool) *httptest.Server {
	mode := websocket.CompressionDisabled
	if compress {
		mode = websocket.CompressionContextTakeover
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: mode, CompressionThreshold: 1})
		if err != nil {
			return
		}
		defer c.CloseNow()
		c.SetReadLimit(64 << 20)
		ctx := r.Context()
		buf := make([]byte, 32<<10)
		for {
			typ, rd, err := c.Reader(ctx)
			if err != nil {
				return
			}
			wr, err := c.Writer(ctx, typ)
			if err != nil {
				return
			}
			if _, err := io.CopyBuffer(wr, rd, buf); err != nil {
				return
			}
			if err := wr.Close(); err != nil {
				return
			}
		}
	}))
}

// ---- benchmark -------------------------------------------------------------

func BenchmarkEcho(b *testing.B) {
	servers := []struct {
		name         string
		start        func(bool) *httptest.Server
		compressOnly bool // Identical to another server without compression.
	}{{"ews", ewsServer, false}, {"ews-shared", ewsSharedServer, true}, {"gws", gwsServer, false}, {"gws-stream", gwsStreamServer, false}, {"coder", coderServer, false}, {"coder-stream", coderStreamServer, false}}
	for _, compress := range []bool{false, true} {
		for _, size := range []int{64, 1024, 16 << 10, 256 << 10} {
			for _, conns := range []int{1, 32, 128, 512, 1024, 2048} {
				for _, s := range servers {
					if s.compressOnly && !compress {
						continue
					}
					name := fmt.Sprintf("compress=%t/size=%d/conns=%d/%s", compress, size, conns, s.name)
					b.Run(name, func(b *testing.B) {
						srv := s.start(compress)
						defer srv.Close()
						msg := harness.Payload(size, compress)
						clients := make([]*ws.Conn, conns)
						for i := range clients {
							clients[i] = harness.Dial(b, srv.URL, compress)
						}
						// Warm up pools and negotiate once before measuring.
						for _, c := range clients {
							c.Write(codec.Binary, msg)
							c.ReadMessage()
						}
						b.ReportAllocs()
						b.SetBytes(int64(size))
						b.ResetTimer()
						var wg sync.WaitGroup
						errs := make(chan error, conns)
						for i, c := range clients {
							// Spread b.N messages exactly across connections.
							per := b.N / conns
							if i < b.N%conns {
								per++
							}
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
