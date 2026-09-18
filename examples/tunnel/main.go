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
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tunnel server|client [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "server":
		server(os.Args[2:])
	case "client":
		client(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q; want server or client\n", os.Args[1])
		os.Exit(2)
	}
}

// server accepts WebSockets and connects each to the upstream TCP address.
func server(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", ":9004", "listen address")
	upstream := fs.String("upstream", "127.0.0.1:22", "TCP address each tunnel connects to")
	fs.Parse(args)

	srv := &transport.Server{
		Handler: func(conn net.Conn, _ handshake.Result, _ *transport.Request) {
			c, err := ws.NewConn(conn, ws.Config{Role: ws.Server})
			if err != nil {
				log.Print(err)
				return
			}
			up, err := net.DialTimeout("tcp", *upstream, 10*time.Second)
			if err != nil {
				log.Printf("%s: %v", conn.RemoteAddr(), err)
				c.Close(1011, "upstream unavailable")
				return
			}
			pipe(ws.NetConn(c, codec.Binary), up)
		},
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("tunnel server on %s, upstream %s", *addr, *upstream)
	log.Fatal(srv.Serve(ln))
}

// client accepts TCP connections and opens a WebSocket to the server for
// each one.
func client(args []string) {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:2222", "local TCP address to accept on")
	url := fs.String("url", "ws://localhost:9004", "tunnel server URL")
	fs.Parse(args)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("tunnel client on %s, server %s", *listen, *url)
	for {
		tcp, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			conn, _, err := transport.Dial(ctx, *url, transport.DialOptions{})
			cancel()
			if err != nil {
				log.Printf("%s: %v", tcp.RemoteAddr(), err)
				tcp.Close()
				return
			}
			c, err := ws.NewConn(conn, ws.Config{Role: ws.Client})
			if err != nil {
				log.Print(err)
				conn.Close()
				tcp.Close()
				return
			}
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
