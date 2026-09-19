// Command echo-http serves the echo endpoint from an http.Handler, for
// servers that already run on net/http: routes, middleware, or TLS through
// ListenAndServeTLS. The handler upgrades, starts the read loop on a
// goroutine of its own, and returns, which lets net/http drop the request
// state it would otherwise keep alive for the life of the connection.
//
//	go run ./examples/echo-http
//	go run ./examples/client -url ws://localhost:8080/echo
package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	cert := flag.String("cert", "", "TLS certificate file; with -key, serves wss")
	key := flag.String("key", "", "TLS key file")
	flag.Parse()

	opts := handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}}
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, res, err := transport.Upgrade(w, r, opts)
		if err != nil {
			log.Printf("%s: %v", r.RemoteAddr, err) // The HTTP error response is already written.
			return
		}
		go echo(conn, res)
		// Returning releases the request's buffers; conn is hijacked and ours.
	})
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if *cert != "" {
		log.Printf("echo server on wss://%s/echo", *addr)
		log.Fatal(srv.ListenAndServeTLS(*cert, *key))
	}
	log.Printf("echo server on ws://%s/echo", *addr)
	log.Fatal(srv.ListenAndServe())
}

// echo runs the connection until the peer closes or a read fails. Serve is
// the same loop the echo example writes out by hand.
func echo(conn net.Conn, res handshake.Result) {
	defer conn.Close()
	c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression, ValidateUTF8: true})
	if err != nil {
		log.Print(err)
		return
	}
	conn.SetDeadline(time.Now().Add(time.Minute))
	err = ws.Serve(c, ws.MessageFunc(func(c *ws.Conn, op codec.Opcode, payload []byte) error {
		conn.SetDeadline(time.Now().Add(time.Minute))
		return c.Write(op, payload)
	}))
	if _, ok := errors.AsType[*ws.CloseError](err); !ok {
		log.Printf("%s: %v", conn.RemoteAddr(), err)
	}
}
