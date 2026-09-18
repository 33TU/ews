// Command client talks to an echo server. Each line typed on stdin is sent
// as a text message and every reply is printed, until stdin ends and the
// connection is closed with code 1000. With -file, the file is streamed as
// one binary message straight from disk and the reply streamed back into a
// hash, which exercises the fragmenting paths in both directions without
// holding the file in memory.
//
//	go run ./examples/client -url ws://localhost:9001
//	go run ./examples/client -url ws://localhost:9002 -file big.bin
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
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
		err = sendFile(c, *file)
	} else {
		err = chat(c)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// closed reports the end of the connection: the peer's close frame arrives
// as a *ws.CloseError from the read that meets it, and anything else is a
// failure.
func closed(err error) error {
	if ce, ok := errors.AsType[*ws.CloseError](err); ok {
		log.Printf("closed, code %d", ce.Code)
		return nil
	}
	return err
}

// chat sends stdin lines and prints replies until stdin ends, then closes.
// One goroutine reads and one writes, as on any connection; the reader is
// the one that sees the peer's close frame.
func chat(c *ws.Conn) error {
	readErr := make(chan error, 1)
	go func() {
		for {
			op, p, err := c.ReadMessage()
			if err != nil {
				readErr <- err
				return
			}
			if op == codec.Text {
				fmt.Printf("< %s\n", p)
			} else {
				fmt.Printf("< %d bytes binary\n", len(p))
			}
		}
	}()
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if err := c.Write(codec.Text, sc.Bytes()); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// A clean close is a close frame each way: send ours, then the reader
	// returns when the peer's arrives.
	if err := c.Close(1000, ""); err != nil {
		return err
	}
	select {
	case err := <-readErr:
		return closed(err)
	case <-time.After(5 * time.Second):
		return errors.New("no close from peer")
	}
}

// sendFile streams the file out and the echo back in at the same time,
// comparing hashes. Sending and receiving must overlap: an echo server
// cannot take the whole message before replying, so a client that sent
// everything first would deadlock once the socket buffers filled.
func sendFile(c *ws.Conn, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	start := time.Now()
	out := sha256.New()
	sent := make(chan int64, 1)
	sendErr := make(chan error, 1)
	go func() {
		n, err := c.WriteFrom(codec.Binary, io.TeeReader(f, out)) // Fragmented as it is read.
		sent <- n
		sendErr <- err
	}()
	if _, err := c.NextMessage(); err != nil {
		return err
	}
	in := sha256.New()
	got, err := c.WriteTo(in) // Frame by frame into the hash, no message buffer.
	if err != nil {
		return err
	}
	if err := <-sendErr; err != nil {
		return err
	}
	elapsed := time.Since(start)
	if n := <-sent; got != n || !bytes.Equal(out.Sum(nil), in.Sum(nil)) {
		return fmt.Errorf("echo mismatch: sent %d bytes, got %d", n, got)
	}
	log.Printf("%d bytes round trip in %v, %.0f MB/s", got, elapsed.Round(time.Millisecond), 2*float64(got)/elapsed.Seconds()/1e6)
	if err := c.Close(1000, ""); err != nil {
		return err
	}
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return closed(err)
		}
	}
}
