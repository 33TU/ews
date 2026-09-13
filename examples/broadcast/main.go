// Command broadcast runs a chat-style hub: every message a client sends is
// relayed to all connected clients. Try it with two terminals:
//
//	go run ./examples/broadcast
//	websocat ws://localhost:8080/
//	websocat ws://localhost:8080/
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/33TU/ews/handshake"
	"github.com/klauspost/compress/flate"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	compress := flag.Bool("compress", true, "offer permessage-deflate")
	flag.Parse()

	var opts handshake.Options
	if *compress {
		opts.Compression = &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}
	}
	hub := NewHub(opts)
	log.Printf("broadcast hub on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, hub))
}
