package bench

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/33TU/ews"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

// BenchmarkFanout measures a server answering each request with a burst of
// small messages, sent one Write at a time or queued in a Batch and flushed
// once. Throughput is in requests per second; each request is one message in
// and fanout messages out.
func BenchmarkFanout(b *testing.B) {
	const fanout, size = 16, 128
	reply := payload(size, true)
	for _, batched := range []bool{false, true} {
		for _, conns := range []int{1, 32, 512} {
			name := fmt.Sprintf("mode=write/conns=%d", conns)
			if batched {
				name = fmt.Sprintf("mode=batch/conns=%d", conns)
			}
			b.Run(name, func(b *testing.B) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, _, err := ews.Upgrade(w, r, handshake.Options{})
					if err != nil {
						return
					}
					defer conn.Close()
					c, _ := ws.NewConn(conn, ws.Config{Role: ws.Server})
					batch := c.NewBatch()
					for {
						if _, _, err := c.ReadMessage(); err != nil {
							return
						}
						for i := 0; i < fanout; i++ {
							if batched {
								batch.Write(codec.Binary, reply)
							} else if err := c.Write(codec.Binary, reply); err != nil {
								return
							}
						}
						if batched {
							if err := batch.Flush(); err != nil {
								return
							}
						}
					}
				}))
				defer srv.Close()
				clients := make([]*ws.Conn, conns)
				for i := range clients {
					clients[i] = dial(b, srv.URL, false)
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

// BenchmarkBroadcast measures one message delivered to every connection:
// encoded per connection with Write, encoded once with WritePrepared, or
// handed to each connection's Queue. Throughput is in messages delivered.
func BenchmarkBroadcast(b *testing.B) {
	const conns, size = 512, 256
	for _, compress := range []bool{false, true} {
		for _, mode := range []string{"write", "prepared", "queue"} {
			b.Run(fmt.Sprintf("compress=%t/conns=%d/%s", compress, conns, mode), func(b *testing.B) {
				var mu sync.Mutex
				var servers []*ws.Conn
				var opts handshake.Options
				if compress {
					opts.Compression = &handshake.Compress{Level: flate.BestSpeed, MinSize: 1}
				}
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, res, err := ews.Upgrade(w, r, opts)
					if err != nil {
						return
					}
					defer conn.Close()
					c, _ := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression, CompressionShared: true})
					mu.Lock()
					servers = append(servers, c)
					mu.Unlock()
					for {
						if _, _, err := c.ReadMessage(); err != nil {
							return
						}
					}
				}))
				defer srv.Close()
				received := make(chan struct{}, conns)
				for i := 0; i < conns; i++ {
					c := dial(b, srv.URL, compress)
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
					ready := len(servers) == conns
					mu.Unlock()
					if ready {
						break
					}
				}
				var queues []*ws.Queue
				if mode == "queue" {
					for _, c := range servers {
						queues = append(queues, c.NewQueue(0))
					}
				}
				msg := payload(size, compress)
				b.ReportAllocs()
				b.SetBytes(int64(size * conns))
				b.ResetTimer()
				for b.Loop() {
					switch mode {
					case "write":
						for _, c := range servers {
							if err := c.Write(codec.Binary, msg); err != nil {
								b.Fatal(err)
							}
						}
					case "prepared":
						p, _ := ws.Prepare(codec.Binary, msg)
						for _, c := range servers {
							if err := c.WritePrepared(p); err != nil {
								b.Fatal(err)
							}
						}
					case "queue":
						p, _ := ws.Prepare(codec.Binary, msg)
						for _, q := range queues {
							if err := q.SendPrepared(p); err != nil {
								b.Fatal(err)
							}
						}
					}
					for i := 0; i < conns; i++ {
						<-received
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(b.N)*conns/b.Elapsed().Seconds(), "msgs/s")
			})
		}
	}
}
