package main

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/33TU/ews"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

// TestHub connects several clients, has one of them speak, and checks that
// every client hears it, compressed and not, then drops a client that stops
// reading and checks the others are unaffected.
func TestHub(t *testing.T) {
	opts := handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed, ContextTakeover: true}}
	hub := NewHub(opts)
	hub.QueueLimit = 256 << 10 // Small, so a stalled client is dropped quickly.
	srv := httptest.NewServer(hub)
	defer srv.Close()

	const n = 5
	clients := make([]*ws.Conn, n)
	for i := range clients {
		clientOpts := handshake.Options{}
		if i%2 == 0 {
			clientOpts.Compression = &handshake.Compress{Level: flate.BestSpeed}
		}
		conn, res, err := ews.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http"), ews.DialOptions{Handshake: clientOpts})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(30 * time.Second))
		clients[i], err = ws.NewConn(conn, ws.Config{Role: ws.Client, Compression: res.Compression})
		if err != nil {
			t.Fatal(err)
		}
	}
	for hub.Len() != n {
		time.Sleep(time.Millisecond)
	}

	message := bytes.Repeat([]byte("hello everyone "), 100)
	for round := 0; round < 3; round++ {
		if err := clients[round].Write(codec.Text, message); err != nil {
			t.Fatal(err)
		}
		for i, c := range clients {
			op, p, err := c.ReadMessage()
			if err != nil || op != codec.Text || !bytes.Equal(p, message) {
				t.Fatalf("round %d client %d: %v", round, i, err)
			}
		}
	}

	// Client 0 stops reading; once the socket buffers are full its queue
	// fills and the hub drops it, while the others keep receiving. Large
	// incompressible messages fill the buffers in a few rounds.
	big := make([]byte, 1<<20)
	rand.NewChaCha8([32]byte{1}).Read(big) // Deterministic, incompressible.
	for rounds := 0; hub.Len() == n; rounds++ {
		if rounds == 200 {
			t.Fatal("stalled client was never dropped")
		}
		if err := clients[1].Write(codec.Binary, big); err != nil {
			t.Fatal(err)
		}
		for _, c := range clients[1:] {
			if _, _, err := c.ReadMessage(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if hub.Len() != n-1 {
		t.Fatalf("%d clients, want %d", hub.Len(), n-1)
	}
	// The dropped client drains what was already delivered, then sees the close.
	for {
		if _, _, err := clients[0].ReadMessage(); err != nil {
			break
		}
	}
	if err := clients[2].Write(codec.Text, []byte(fmt.Sprint("after drop"))); err != nil {
		t.Fatal(err)
	}
	for _, c := range clients[1:] {
		if _, p, err := c.ReadMessage(); err != nil || string(p) != "after drop" {
			t.Fatalf("after drop: %q %v", p, err)
		}
	}
}
