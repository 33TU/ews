// Command client talks to an echo server. Stdin goes out as text messages,
// one per line typed, and every reply is printed, until stdin ends and the
// connection is closed with code 1000. With -file, the file is streamed as
// one binary message straight from disk and the reply streamed back into a
// hash, which exercises the fragmenting paths in both directions without
// holding the file in memory.
//
//	go run ./examples/client -url ws://localhost:9001
//	go run ./examples/client -url ws://localhost:9002 -file big.bin
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

func main() {
	url := flag.String("url", "ws://localhost:9001", "server URL")
	compress := flag.Bool("compress", true, "offer permessage-deflate")
	file := flag.String("file", "", "send this file as one binary message instead of reading stdin")
	flag.Parse()

	var opts handshake.Options
	if *compress {
		opts.Compression = &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	conn, res, err := transport.Dial(ctx, *url, transport.DialOptions{Handshake: opts})
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	c, err := ws.NewConn(conn, ws.Config{Role: ws.Client, Compression: res.Compression})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("connected to %s, compression %v", *url, res.Compression != nil)

	if *file != "" {
		if err := sendFile(c, *file); err != nil {
			log.Fatal(err)
		}
		c.Close(1000, "")
	} else {
		// As a net.Conn, each Write is one text message and each reply is
		// read back as bytes: io.Copy in both directions is the whole chat.
		go func() {
			io.Copy(ws.NetConn(c, codec.Text), os.Stdin)
			c.Close(1000, "") // Stdin ended: our half of the close handshake.
		}()
	}
	// The peer's close with code 1000 reads as io.EOF, so copying to the end
	// waits for the handshake to complete and prints every reply on the way.
	if _, err := io.Copy(os.Stdout, ws.NetConn(c, codec.Text)); err != nil {
		log.Fatal(err)
	}
	log.Print("closed")
}

// sendFile streams the file out and the echo back in at the same time and
// compares hashes. The two must overlap: an echo server cannot take the
// whole message before replying, so a client that sent everything first
// would deadlock once the socket buffers filled.
func sendFile(c *ws.Conn, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	out, in := sha256.New(), sha256.New()
	sent := make(chan error, 1)
	go func() {
		_, err := c.WriteFrom(codec.Binary, io.TeeReader(f, out)) // Fragmented as it is read.
		sent <- err
	}()
	if _, err := c.NextMessage(); err != nil {
		return err
	}
	n, err := c.WriteTo(in) // Frame by frame into the hash, no message buffer.
	if err != nil {
		return err
	}
	if err := <-sent; err != nil {
		return err
	}
	if !bytes.Equal(out.Sum(nil), in.Sum(nil)) {
		return fmt.Errorf("echo differs after %d bytes", n)
	}
	log.Printf("%d bytes echoed intact", n)
	return nil
}
