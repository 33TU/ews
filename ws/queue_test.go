package ws_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/ws"
)

// gate is a writer that blocks each Write until released.
type gate struct {
	release chan struct{}
	writes  int
	err     error
}

func (g *gate) Read([]byte) (int, error) { return 0, io.EOF }
func (g *gate) Write(p []byte) (int, error) {
	<-g.release
	g.writes++
	if g.err != nil {
		return 0, g.err
	}
	return len(p), nil
}

func TestQueue(t *testing.T) {
	payload := bytes.Repeat([]byte("queued "), 20)
	p, _ := ws.Prepare(codec.Text, payload)

	// Ordering and compression through real peers, mixed Send and SendPrepared.
	server, client := compressionPair(t, true, true, 1)
	const n = 50
	wait := run(t, func() error {
		for i := 0; i < 2*n; i++ {
			_, got, err := client.ReadMessage()
			if err != nil {
				return fmt.Errorf("message %d: %v", i, err)
			}
			want := payload
			if i%2 == 0 {
				want = fmt.Appendf(nil, "message %d", i)
			}
			if !bytes.Equal(got, want) {
				return fmt.Errorf("message %d out of order: %q", i, got)
			}
		}
		return nil
	})
	q := server.NewQueue(0)
	for i := 0; i < 2*n; i++ {
		var err error
		if i%2 == 0 {
			err = q.Send(codec.Text, fmt.Appendf(nil, "message %d", i))
		} else {
			err = q.SendPrepared(p)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Wait(); err != nil {
		t.Fatal(err)
	}
	wait()

	// Direct writes join the queue in submission order and return once
	// written, so both styles mix, under compression with takeover.
	wait = run(t, func() error {
		for i := 0; i < 3*n; i++ {
			_, got, err := client.ReadMessage()
			if err != nil {
				return fmt.Errorf("mixed %d: %v", i, err)
			}
			var want []byte
			switch i % 3 {
			case 0:
				want = fmt.Appendf(nil, "queued %d", i)
			case 1:
				want = fmt.Appendf(nil, "direct %d", i)
			default:
				want = payload
			}
			if !bytes.Equal(got, want) {
				return fmt.Errorf("mixed %d out of order: %q", i, got)
			}
		}
		return nil
	})
	for i := 0; i < 3*n; i++ {
		var err error
		switch i % 3 {
		case 0:
			err = q.Send(codec.Text, fmt.Appendf(nil, "queued %d", i))
		case 1:
			err = server.Write(codec.Text, fmt.Appendf(nil, "direct %d", i))
		default:
			err = server.WritePrepared(p)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	wait()
	if server.NewQueue(0) != q {
		t.Fatal("NewQueue must return the existing queue")
	}

	// Coalescing, backpressure, and control frames, against a gated writer.
	g := &gate{release: make(chan struct{})}
	c, err := ws.NewConn(g, ws.Config{Role: ws.Server})
	if err != nil {
		t.Fatal(err)
	}
	q = c.NewQueue(100)
	if err := q.Send(codec.Binary, make([]byte, 60)); err != nil { // Taken by the writer, now blocked.
		t.Fatal(err)
	}
	for q.Pending() != 0 {
	}
	if err := q.Send(codec.Binary, make([]byte, 160)); err != nil {
		t.Fatal("an empty queue must accept a message larger than the limit:", err)
	}
	if err := q.Send(codec.Binary, make([]byte, 1)); err != ws.ErrQueueFull {
		t.Fatalf("over the limit: %v", err)
	}
	if err := q.Send(codec.Ping, nil); err != ws.ErrProtocol {
		t.Fatal("control frame queued")
	}
	close(g.release)
	if err := q.Wait(); err != nil || g.writes != 2 || q.Pending() != 0 {
		t.Fatalf("writes %d, pending %d, %v", g.writes, q.Pending(), err)
	}
	if err := c.Ping(nil); err != nil {
		t.Fatal("control frames must bypass the queue:", err)
	}

	// A write error is sticky.
	boom := errors.New("boom")
	g = &gate{release: make(chan struct{}), err: boom}
	close(g.release)
	c, _ = ws.NewConn(g, ws.Config{Role: ws.Server})
	q = c.NewQueue(0)
	if err := q.Send(codec.Binary, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := q.Wait(); err != boom || q.Err() != boom {
		t.Fatalf("sticky error: %v", err)
	}
	if err := q.Send(codec.Binary, []byte("y")); err != boom {
		t.Fatalf("send after failure: %v", err)
	}
}
