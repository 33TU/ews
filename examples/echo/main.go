// Command echo serves a WebSocket echo endpoint, the shape the Autobahn test
// suite expects: go run ./examples/echo, then point wstest at ws://host:9001.
package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/33TU/ews"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

func main() {
	addr := flag.String("addr", ":9001", "listen address")
	compress := flag.Bool("compress", true, "offer permessage-deflate")
	flag.Parse()

	var opts handshake.Options
	if *compress {
		opts.Compression = &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, res, err := ews.Upgrade(w, r, opts)
		if err != nil {
			return
		}
		defer conn.Close()

		c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, MaxMessageSize: 32 << 20, Compression: res.Compression})
		if err != nil {
			log.Print(err)
			return
		}
		for {
			conn.SetDeadline(time.Now().Add(time.Minute))
			op, p, err := c.ReadMessage()
			if err != nil {
				if _, ok := errors.AsType[*ws.CloseError](err); !ok {
					log.Printf("%s: %v", conn.RemoteAddr(), err)
				}
				return
			}
			if err := c.Write(op, p); err != nil {
				return
			}
		}
	})
	log.Printf("echo server on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
