package broadcast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/33TU/ews"
	"github.com/33TU/ews/bench/internal/harness"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
	"github.com/lxzan/gws"
)

// BenchmarkBroadcast delivers one message to every connected client and
// times the round until all clients have received it. Each server reads
// through its own ReadMessage and broadcasts through its own once-encoded
// frame: ews with Prepare and a Queue per connection, ews-sync with Prepare
// and WritePrepared in a loop, gws with NewBroadcaster and its per-connection
// worker. Throughput is in messages delivered.
func BenchmarkBroadcast(b *testing.B) {
	const size = 256
	for _, compress := range []bool{false, true} {
		for _, conns := range []int{128, 512, 2048} {
			for _, lib := range []string{"ews", "ews-sync", "gws"} {
				b.Run(fmt.Sprintf("compress=%t/conns=%d/%s", compress, conns, lib), func(b *testing.B) {
					var mu sync.Mutex
					var ewsConns []*ws.Conn
					var gwsConns []*gws.Conn
					var srv *httptest.Server
					if lib == "gws" {
						up := harness.GwsUpgrader(compress, gws.BuiltinEventHandler{})
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
						var opts handshake.Options
						if compress {
							opts.Compression = &handshake.Compress{Level: flate.BestSpeed, MinSize: 1, ContextTakeover: true}
						}
						srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							conn, res, err := ews.Upgrade(w, r, opts)
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
					for i := 0; i < conns; i++ {
						c := harness.Dial(b, srv.URL, compress)
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
						ready := len(ewsConns) == conns || len(gwsConns) == conns
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

					msg := harness.Payload(size, compress)
					b.ReportAllocs()
					b.SetBytes(int64(size * conns))
					b.ResetTimer()
					for b.Loop() {
						switch lib {
						case "ews":
							p, _ := ws.Prepare(codec.Binary, msg)
							for _, q := range queues {
								if err := q.SendPrepared(p); err != nil {
									b.Fatal(err)
								}
							}
							p.Release()
						case "ews-sync":
							p, _ := ws.Prepare(codec.Binary, msg)
							for _, c := range ewsConns {
								if err := c.WritePrepared(p); err != nil {
									b.Fatal(err)
								}
							}
							p.Release()
						case "gws":
							bc := gws.NewBroadcaster(gws.OpcodeBinary, msg)
							for _, c := range gwsConns {
								if err := bc.Broadcast(c, nil); err != nil {
									b.Fatal(err)
								}
							}
							bc.Close()
						}
						for i := 0; i < conns; i++ {
							<-received
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(b.N)*float64(conns)/b.Elapsed().Seconds(), "msgs/s")
				})
			}
		}
	}
}
