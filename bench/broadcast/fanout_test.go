package broadcast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/33TU/ews/bench/internal/harness"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

// BenchmarkFanout measures a server answering each request with a burst of
// small messages, sent one Write at a time or through a Queue, whose writer
// coalesces the burst into one write. Throughput is in requests per second;
// each request is one message in and fanout messages out.
func BenchmarkFanout(b *testing.B) {
	const fanout, size = 16, 128
	reply := harness.Payload(size, true)
	for _, queued := range []bool{false, true} {
		for _, conns := range []int{1, 32, 512} {
			name := fmt.Sprintf("mode=write/conns=%d", conns)
			if queued {
				name = fmt.Sprintf("mode=queue/conns=%d", conns)
			}
			b.Run(name, func(b *testing.B) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, _, err := transport.Upgrade(w, r, handshake.Options{})
					if err != nil {
						return
					}
					defer conn.Close()
					c, _ := ws.NewConn(conn, ws.Config{Role: ws.Server})
					var q *ws.Queue
					if queued {
						q = c.NewQueue(0)
					}
					for {
						if _, _, err := c.ReadMessage(); err != nil {
							return
						}
						for i := 0; i < fanout; i++ {
							var err error
							if queued {
								err = q.Send(codec.Binary, reply)
							} else {
								err = c.Write(codec.Binary, reply)
							}
							if err != nil {
								return
							}
						}
					}
				}))
				defer srv.Close()
				clients := make([]*ws.Conn, conns)
				for i := range clients {
					clients[i] = harness.Dial(b, srv.URL, harness.Plain)
				}
				request := []byte("go")
				b.ReportAllocs()
				b.ResetTimer()
				var wg sync.WaitGroup
				for i, c := range clients {
					per := b.N / conns
					if i < b.N%conns {
						per++
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						for range per {
							if err := c.Write(codec.Binary, request); err != nil {
								b.Error(err)
								return
							}
							for j := 0; j < fanout; j++ {
								if _, p, err := c.ReadMessage(); err != nil || len(p) != size {
									b.Errorf("reply %d: %v", j, err)
									return
								}
							}
						}
					}()
				}
				wg.Wait()
				b.StopTimer()
				b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "req/s")
			})
		}
	}
}
