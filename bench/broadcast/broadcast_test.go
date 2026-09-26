package broadcast

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/33TU/ews/bench/internal/harness"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	gorilla "github.com/gorilla/websocket"
	"github.com/klauspost/compress/flate"
	"github.com/lxzan/gws"
)

// BenchmarkBroadcast delivers one message to every connected client and
// times the round until all clients have received it. Each server reads
// through its own ReadMessage and broadcasts through its own once-encoded
// frame: ews with Prepare and a Queue per connection, ews-sync with Prepare
// and WritePrepared in a loop, gws with NewBroadcaster and its per-connection
// worker. Throughput is in messages delivered. Servers run in a seeded
// shuffled order per cell and each gets a warm-up round before timing, so
// no server always follows the connection setup.
func BenchmarkBroadcast(b *testing.B) {
	rng := rand.New(rand.NewPCG(7, 11))
	for _, mode := range []harness.Mode{harness.Plain, harness.Takeover, harness.NoTakeover} {
		for _, size := range []int{256, 4 << 10, 64 << 10, 256 << 10, 2 << 20, 6 << 20} {
			for _, conns := range []int{128, 512, 2048, 8192} {
				// Every client holds a message of this size, so the large ones
				// stop at 128 connections: past that they measure memory.
				if size > 1<<20 && conns > 128 {
					continue
				}
				libs := []string{"ews", "ews-sync", "gws", "gorilla"}
				if mode == harness.Takeover {
					libs = libs[:3] // gorilla supports only no_context_takeover.
				}
				rng.Shuffle(len(libs), func(i, j int) { libs[i], libs[j] = libs[j], libs[i] })
				for _, lib := range libs {
					b.Run(fmt.Sprintf("compress=%s/size=%d/conns=%d/%s", mode, size, conns, lib), func(b *testing.B) {
						var mu sync.Mutex
						var ewsConns []*ws.Conn
						var gwsConns []*gws.Conn
						var gorillaConns []*gorilla.Conn
						var srv *httptest.Server
						if lib == "gorilla" {
							up := &gorilla.Upgrader{EnableCompression: mode.Compressed(), CheckOrigin: func(*http.Request) bool { return true }}
							srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								c, err := up.Upgrade(w, r, nil)
								if err != nil {
									return
								}
								c.SetCompressionLevel(flate.BestSpeed)
								mu.Lock()
								gorillaConns = append(gorillaConns, c)
								mu.Unlock()
								for {
									if _, _, err := c.ReadMessage(); err != nil {
										return
									}
								}
							}))
						} else if lib == "gws" {
							up := harness.GwsUpgrader(mode, gws.BuiltinEventHandler{})
							srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								socket, err := up.Upgrade(w, r)
								if err != nil {
									return
								}
								mu.Lock()
								gwsConns = append(gwsConns, socket)
								mu.Unlock()
								for {
									msg, err := socket.ReadMessage()
									if err != nil {
										return
									}
									msg.Close()
								}
							}))
						} else {
							opts := mode.Options()
							srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								conn, res, err := transport.Upgrade(w, r, opts)
								if err != nil {
									return
								}
								defer conn.Close()
								c, _ := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression, CompressionShared: true})
								if lib == "ews" {
									c.NewQueue(0)
								}
								mu.Lock()
								ewsConns = append(ewsConns, c)
								mu.Unlock()
								for {
									if _, _, err := c.ReadMessage(); err != nil {
										return
									}
								}
							}))
						}
						defer srv.Close()

						received := make(chan struct{}, conns)
						for range conns {
							c := harness.Dial(b, srv.URL, mode)
							go func() {
								for {
									if _, p, err := c.ReadMessage(); err != nil || len(p) != size {
										return
									}
									received <- struct{}{}
								}
							}()
						}
						for {
							mu.Lock()
							ready := len(ewsConns) == conns || len(gwsConns) == conns || len(gorillaConns) == conns
							mu.Unlock()
							if ready {
								break
							}
						}
						var queues []*ws.Queue
						if lib == "ews" {
							for _, c := range ewsConns {
								queues = append(queues, c.NewQueue(0)) // Returns the queue made in the handler.
							}
						}

						msg := harness.Payload(size, mode.Compressed())
						round := func() {
							switch lib {
							case "ews":
								p, _ := ws.Prepare(codec.Binary, msg)
								for _, q := range queues {
									if err := q.SendPrepared(p); err != nil {
										b.Fatal(err)
									}
								}
							case "ews-sync":
								p, _ := ws.Prepare(codec.Binary, msg)
								for _, c := range ewsConns {
									if err := c.WritePrepared(p); err != nil {
										b.Fatal(err)
									}
								}
							case "gorilla":
								// PreparedMessage encodes once per configuration; gorilla
								// writes synchronously, like ews-sync.
								pm, err := gorilla.NewPreparedMessage(gorilla.BinaryMessage, msg)
								if err != nil {
									b.Fatal(err)
								}
								for _, c := range gorillaConns {
									if err := c.WritePreparedMessage(pm); err != nil {
										b.Fatal(err)
									}
								}
							case "gws":
								bc := gws.NewBroadcaster(gws.OpcodeBinary, msg)
								for _, c := range gwsConns {
									if err := bc.Broadcast(c, nil); err != nil {
										b.Fatal(err)
									}
								}
								bc.Close()
							}
							for range conns {
								<-received
							}
						}
						for range 20 {
							round() // Warm up: pools, goroutines, and socket buffers settle.
						}
						b.ReportAllocs()
						b.SetBytes(int64(size * conns))
						b.ResetTimer()
						cpu := harness.CPUTime()
						for b.Loop() {
							round()
						}
						b.StopTimer()
						cpu = harness.CPUTime() - cpu
						delivered := float64(b.N) * float64(conns)
						b.ReportMetric(delivered/b.Elapsed().Seconds(), "msgs/s")
						b.ReportMetric(float64(cpu.Nanoseconds())/delivered, "cpu-ns/msg")
					})
				}
			}
		}
	}
}
