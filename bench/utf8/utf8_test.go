package utf8

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/33TU/ews/bench/internal/harness"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/coder/websocket"
	"github.com/lxzan/gws"
)

// Text echo servers with UTF-8 validation enabled where the library offers
// it, so the cost of validating text shows at the connection level.

func ewsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := transport.Upgrade(w, r, handshake.Options{})
		if err != nil {
			return
		}
		defer conn.Close()
		c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, MaxMessageSize: 64 << 20, ValidateUTF8: true})
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

func gwsServer() *httptest.Server {
	// CheckUtf8Enabled validates received text and, in gws, outgoing text too.
	up := gws.NewUpgrader(gws.BuiltinEventHandler{}, &gws.ServerOption{ReadMaxPayloadSize: 64 << 20, CheckUtf8Enabled: true})
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

// coderServer has no UTF-8 validation to enable; it echoes text as bytes.
func coderServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
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

// text returns size bytes of valid UTF-8 of the given kind, cut on a rune boundary.
func text(kind string, size int) []byte {
	var unit []byte
	switch kind {
	case "ascii":
		unit = []byte(`{"id":12345,"type":"update","user":"alice","text":"hello world"} `)
	case "mixed":
		unit = []byte(`{"id":12345,"type":"update","user":"アリス","text":"hello 世界, こんにちは"} `)
	case "multibyte":
		unit = []byte("東京都渋谷区神南一丁目、日本語のテキストメッセージです。")
	}
	b := bytes.Repeat(unit, size/len(unit)+1)[:size]
	for len(b) > 0 && b[len(b)-1]&0xC0 == 0x80 {
		b = b[:len(b)-1] // Drop a trailing partial rune.
	}
	if len(b) > 0 && b[len(b)-1] >= 0xC0 {
		b = b[:len(b)-1]
	}
	return b
}

// BenchmarkUTF8 echoes text messages through servers that validate UTF-8,
// ews and gws with their checks enabled and coder without one to enable.
// Clients do not validate, so the difference is the server's pass.
func BenchmarkUTF8(b *testing.B) {
	servers := []struct {
		name  string
		start func() *httptest.Server
	}{{"ews", ewsServer}, {"gws", gwsServer}, {"coder", coderServer}}
	rng := rand.New(rand.NewPCG(7, 11))
	for _, kind := range []string{"ascii", "mixed", "multibyte"} {
		for _, size := range []int{1024, 16 << 10} {
			for _, conns := range []int{1, 128} {
				order := []int{0, 1, 2}
				rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
				for _, si := range order {
					s := servers[si]
					b.Run(fmt.Sprintf("kind=%s/size=%d/conns=%d/%s", kind, size, conns, s.name), func(b *testing.B) {
						srv := s.start()
						defer srv.Close()
						msg := text(kind, size)
						clients := make([]*ws.Conn, conns)
						for i := range clients {
							clients[i] = harness.Dial(b, srv.URL, harness.Plain)
						}
						for round := 0; round < 5; round++ {
							for _, c := range clients {
								c.Write(codec.Text, msg)
								c.ReadMessage()
							}
						}
						b.ReportAllocs()
						b.SetBytes(int64(len(msg)))
						b.ResetTimer()
						var wg sync.WaitGroup
						errs := make(chan error, conns)
						for i, c := range clients {
							per := b.N / conns
							if i < b.N%conns {
								per++
							}
							wg.Add(1)
							go func() {
								defer wg.Done()
								for range per {
									if err := c.Write(codec.Text, msg); err != nil {
										errs <- err
										return
									}
									if op, p, err := c.ReadMessage(); err != nil || op != codec.Text || len(p) != len(msg) {
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
