// Command tunnel carries TCP connections over WebSockets, the way a
// WebSocket-only ingress or a browser-side proxy needs: NetConn turns a
// connection into a net.Conn, one message per Write, and io.Copy does the
// rest. Run the server next to the service and the client where the TCP
// clients are, for example to reach SSH through a port that only passes
// WebSockets:
//
//	go run ./examples/tunnel server -addr :9004 -upstream 127.0.0.1:22
//	go run ./examples/tunnel client -listen 127.0.0.1:2222 -url ws://localhost:9004
//	ssh -p 2222 localhost
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

func main() {
	addr := flag.String("addr", ":9004", "server: WebSocket listen address")
	upstream := flag.String("upstream", "127.0.0.1:22", "server: TCP address each tunnel connects to")
	listen := flag.String("listen", "127.0.0.1:2222", "client: local TCP listen address")
	url := flag.String("url", "ws://localhost:9004", "client: tunnel server URL")

	mode := ""
	if len(os.Args) > 1 {
		mode = os.Args[1]
		flag.CommandLine.Parse(os.Args[2:])
	}

	switch mode {
	case "server":
		server(*addr, *upstream)
	case "client":
		client(*listen, *url)
	default:
		fmt.Fprintln(os.Stderr, "usage: tunnel server|client [flags]")
		os.Exit(2)
	}
}

// server connects each WebSocket to the upstream TCP address.
func server(addr, upstream string) {
	srv := &transport.Server{
		Handler: func(conn net.Conn, _ handshake.Result, _ *transport.Request) {
			c, _ := ws.NewConn(conn, ws.Config{Role: ws.Server}) // Only an invalid Config fails.

			up, err := net.DialTimeout("tcp", upstream, 10*time.Second)
			if err != nil {
				log.Printf("%s: %v", conn.RemoteAddr(), err)
				c.Close(1011, "upstream unavailable")
				return
			}

			pipe(ws.NetConn(c, codec.Binary), up)
		},
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("tunnel server on %s, upstream %s", addr, upstream)
	log.Fatal(srv.Serve(ln))
}

// client opens a WebSocket to the server for each TCP connection accepted.
func client(listen, url string) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("tunnel client on %s, server %s", listen, url)

	for {
		tcp, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			conn, _, err := transport.Dial(ctx, url, transport.DialOptions{})
			cancel()
			if err != nil {
				log.Printf("%s: %v", tcp.RemoteAddr(), err)
				tcp.Close()
				return
			}

			c, _ := ws.NewConn(conn, ws.Config{Role: ws.Client})
			pipe(tcp, ws.NetConn(c, codec.Binary))
		}()
	}
}

// pipe copies both ways until one side ends, then closes both. Closing the
// NetConn sends a close frame, which the far side's Read reports as io.EOF,
// so the end of a TCP stream travels through the tunnel as a clean close.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()

	<-done
	a.Close()
	b.Close()
	<-done
}
