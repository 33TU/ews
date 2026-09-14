// Package bench compares ews with other Go WebSocket libraries end to end:
// echo servers over loopback TCP driven by the same ews client. It is a
// separate module so the root module stays free of those dependencies.
//
//	cd bench && go test -run ^$ -bench . -benchtime=1s | go run ./cmd/results > RESULTS.md
package echo

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/33TU/ews"
	"github.com/33TU/ews/bench/internal/harness"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/ws"
	"github.com/coder/websocket"
	gorilla "github.com/gorilla/websocket"
	"github.com/klauspost/compress/flate"
	"github.com/lxzan/gws"
)

// Echo servers for each library, all driven by the same ews client.

func ewsServer(mode harness.Mode) *httptest.Server { return ewsServerWith(mode, false, false) }

// ewsSharedServer borrows a pooled compressor per message like gws and coder
// do, instead of keeping one attached per connection.
func ewsSharedServer(mode harness.Mode) *httptest.Server { return ewsServerWith(mode, true, false) }

// ewsStreamServer pipes each message from the connection's own Read into
// WriteFrom, ews's streaming shape: no message is held whole.
func ewsStreamServer(mode harness.Mode) *httptest.Server { return ewsServerWith(mode, false, true) }

func ewsServerWith(mode harness.Mode, shared, stream bool) *httptest.Server {
	opts := mode.Options()
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
			if stream {
				op, err := c.NextMessage()
				if err != nil {
					return
				}
				if _, err := c.WriteFrom(op, c); err != nil {
					return
				}
				continue
			}
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
func gwsServer(mode harness.Mode) *httptest.Server {
	up := harness.GwsUpgrader(mode, gws.BuiltinEventHandler{})
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
func gwsStreamServer(mode harness.Mode) *httptest.Server {
	up := harness.GwsUpgrader(mode, gws.BuiltinEventHandler{})
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

func coderServer(mode harness.Mode) *httptest.Server {
	cmode := coderMode(mode)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Threshold 1 compresses every message, as the other servers do.
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: cmode, CompressionThreshold: 1})
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
func coderStreamServer(mode harness.Mode) *httptest.Server {
	cmode := coderMode(mode)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: cmode, CompressionThreshold: 1})
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

func coderMode(mode harness.Mode) websocket.CompressionMode {
	switch mode {
	case harness.Takeover:
		return websocket.CompressionContextTakeover
	case harness.NoTakeover:
		return websocket.CompressionNoContextTakeover
	}
	return websocket.CompressionDisabled
}

// gorillaUpgrader negotiates compression when asked. gorilla offers only
// no_context_takeover, so it runs in the plain and no-takeover modes.
func gorillaUpgrader(mode harness.Mode) *gorilla.Upgrader {
	return &gorilla.Upgrader{EnableCompression: mode.Compressed(), CheckOrigin: func(*http.Request) bool { return true }}
}

// gorillaServer echoes through gorilla's ReadMessage and WriteMessage.
func gorillaServer(mode harness.Mode) *httptest.Server {
	up := gorillaUpgrader(mode)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		c.SetReadLimit(64 << 20)
		c.SetCompressionLevel(flate.BestSpeed)
		for {
			mt, p, err := c.ReadMessage()
			if err != nil {
				return
			}
			if err := c.WriteMessage(mt, p); err != nil {
				return
			}
		}
	}))
}

// gorillaStreamServer pipes NextReader into NextWriter through a reusable
// buffer, so no message is held whole.
func gorillaStreamServer(mode harness.Mode) *httptest.Server {
	up := gorillaUpgrader(mode)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		c.SetReadLimit(64 << 20)
		c.SetCompressionLevel(flate.BestSpeed)
		buf := make([]byte, 32<<10)
		for {
			mt, rd, err := c.NextReader()
			if err != nil {
				return
			}
			wr, err := c.NextWriter(mt)
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
	all := []harness.Mode{harness.Plain, harness.Takeover, harness.NoTakeover}
	servers := []struct {
		name  string
		start func(harness.Mode) *httptest.Server
		modes []harness.Mode // Modes the server runs in.
	}{
		{"ews", ewsServer, all},
		{"ews-shared", ewsSharedServer, []harness.Mode{harness.Takeover}}, // Identical to ews in the other modes.
		{"ews-stream", ewsStreamServer, all},
		{"gws", gwsServer, all},
		{"gws-stream", gwsStreamServer, all},
		{"coder", coderServer, all},
		{"coder-stream", coderStreamServer, all},
		{"gorilla", gorillaServer, []harness.Mode{harness.Plain, harness.NoTakeover}}, // No takeover support.
		{"gorilla-stream", gorillaStreamServer, []harness.Mode{harness.Plain, harness.NoTakeover}},
	}
	// Servers run in a seeded shuffled order per cell, so no library always
	// measures right after the connection setup.
	rng := rand.New(rand.NewPCG(7, 11))
	for _, mode := range all {
		for _, size := range []int{64, 1024, 16 << 10, 256 << 10} {
			for _, conns := range []int{1, 32, 128, 512, 1024, 2048} {
				order := append([]int(nil), make([]int, len(servers))...)
				for i := range order {
					order[i] = i
				}
				rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
				for _, si := range order {
					s := servers[si]
					if !slices.Contains(s.modes, mode) {
						continue
					}
					name := fmt.Sprintf("compress=%s/size=%d/conns=%d/%s", mode, size, conns, s.name)
					b.Run(name, func(b *testing.B) {
						srv := s.start(mode)
						defer srv.Close()
						msg := harness.Payload(size, mode.Compressed())
						clients := make([]*ws.Conn, conns)
						for i := range clients {
							clients[i] = harness.Dial(b, srv.URL, mode)
						}
						// Warm up: pools, goroutines, and socket buffers settle
						// over several round trips per connection.
						for round := 0; round < 5; round++ {
							for _, c := range clients {
								c.Write(codec.Binary, msg)
								c.ReadMessage()
							}
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
