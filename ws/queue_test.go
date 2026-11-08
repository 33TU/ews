package ws_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/ws"
)

// gate is a writer that blocks each Write until released.
type gate struct {
	nopConn
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
		for i := range 2 * n {
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
	for i := range 2 * n {
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
		for i := range 3 * n {
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
	for i := range 3 * n {
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
		t.Fatal("ping through the queue:", err)
	}
	if err := q.Wait(); err != nil || g.writes != 3 {
		t.Fatalf("ping not written: writes %d, %v", g.writes, err)
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

// TestNewQueueAdjustsLimit checks that calling NewQueue again returns the same
// queue, keeps its limit for zero, and adjusts it for a nonzero value.
func TestNewQueueAdjustsLimit(t *testing.T) {
	server, _ := pair(t, ws.Config{}, ws.Config{})
	q := server.NewQueue(100)
	if server.NewQueue(0) != q || server.NewQueue(50) != q {
		t.Fatal("NewQueue made a second queue")
	}
	// The peer never reads, so the writer blocks on the first frame and
	// everything after it stays queued, where the limit applies.
	if err := q.Send(codec.Binary, make([]byte, 40)); err != nil {
		t.Fatal(err)
	}
	for q.Pending() != 0 {
		time.Sleep(time.Millisecond) // The writer has taken the first frame.
	}
	if err := q.Send(codec.Binary, make([]byte, 40)); err != nil {
		t.Fatal("an empty queue accepts any message:", err)
	}
	if err := q.Send(codec.Binary, make([]byte, 40)); err != ws.ErrQueueFull {
		t.Fatalf("limit not adjusted down to 50: %v", err)
	}
	server.NewQueue(1000)
	if err := q.Send(codec.Binary, make([]byte, 40)); err != nil {
		t.Fatalf("limit not adjusted up: %v", err)
	}
}

// TestQueueCloseOrder: Close on a connection with a Queue goes out behind
// the data already queued, so the peer reads every message and then the
// close, and nothing follows the close frame on the wire.
func TestQueueCloseOrder(t *testing.T) {
	for range 100 {
		server, client := pair(t, ws.Config{}, ws.Config{})
		wait := run(t, func() error {
			for i := range 3 {
				_, got, err := client.ReadMessage()
				if err != nil {
					return fmt.Errorf("message %d: %v", i, err)
				}
				if want := fmt.Sprintf("message %d", i); string(got) != want {
					return fmt.Errorf("message %d: got %q", i, got)
				}
			}
			_, _, err := client.ReadMessage()
			if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1000 || ce.Reason != "done" {
				return fmt.Errorf("after the data: %v", err)
			}
			return nil
		})
		q := server.NewQueue(0)
		for i := range 3 {
			if err := q.Send(codec.Text, fmt.Appendf(nil, "message %d", i)); err != nil {
				t.Fatal(err)
			}
		}
		if err := server.Close(1000, "done"); err != nil {
			t.Fatal(err)
		}
		if q.Pending() != 0 {
			t.Fatalf("Close returned with %d bytes still queued", q.Pending())
		}
		// The client's close echo needs a reader on the pipe.
		if _, _, err := server.ReadMessage(); err == nil {
			t.Fatalf("expected the close echo, got %v", err)
		}
		wait()
	}
}

// answered is DefaultControlHandler, reporting each ping once its pong has
// been sent.
type answered struct {
	ws.DefaultControlHandler
	pongs chan string
}

func (h answered) OnPing(c *ws.Conn, payload []byte) error {
	err := h.DefaultControlHandler.OnPing(c, payload)
	h.pongs <- string(payload)
	return err
}

// stall gives the server a Queue whose writer is blocked on a client that
// does not read, holding a 60-byte frame, with a 160-byte frame queued
// behind it past the 100-byte limit. The server reads in the background.
func stall(t *testing.T, client ws.Config) (server, peer *ws.Conn, q *ws.Queue, pongs chan string) {
	t.Helper()
	pongs = make(chan string, 1)
	server, peer = pair(t, ws.Config{ControlHandler: answered{pongs: pongs}}, client)
	q = server.NewQueue(100)
	if err := q.Send(codec.Binary, make([]byte, 60)); err != nil {
		t.Fatal(err)
	}
	for q.Pending() != 0 {
		time.Sleep(time.Millisecond) // The writer has taken the frame and blocks on it.
	}
	if err := q.Send(codec.Binary, make([]byte, 160)); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			if _, _, err := server.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return server, peer, q, pongs
}

// TestQueuePongToStalledPeer: answering a ping from a peer that stops
// reading must not wait for the stalled writer, nor block later sends,
// which hold the same write lock as the pong.
func TestQueuePongToStalledPeer(t *testing.T) {
	_, peer, q, pongs := stall(t, ws.Config{})
	if err := peer.Ping([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pongs:
	case <-time.After(time.Second):
		t.Fatal("the pong waited for the stalled writer")
	}
	done := make(chan error, 1)
	go func() { done <- q.Send(codec.Binary, []byte("late")) }()
	select {
	case err := <-done:
		if err != ws.ErrQueueFull {
			t.Fatalf("send past the limit: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Send blocked behind the pong")
	}
}

// collectPongs records the payload of every pong a client reads.
type collectPongs struct {
	ws.DefaultControlHandler
	got *[]string
}

func (h collectPongs) OnPong(_ *ws.Conn, payload []byte) error {
	*h.got = append(*h.got, string(payload))
	return nil
}

// TestQueuePingFlood: a peer that pings without reading adds one pong past
// the limit, not one per ping, and the pong it gets answers its last ping.
func TestQueuePingFlood(t *testing.T) {
	var got []string
	server, peer, q, pongs := stall(t, ws.Config{ControlHandler: collectPongs{got: &got}})
	before := q.Pending()
	for i := range 50 {
		if err := peer.Ping(fmt.Appendf(nil, "ping %02d", i)); err != nil {
			t.Fatal(err)
		}
		<-pongs
	}
	if grew := q.Pending() - before; grew != 2+len("ping 49") {
		t.Fatalf("50 pongs added %d bytes, want one pong", grew)
	}

	// The peer reads again: the two frames, the one pong, then a last message.
	wait := run(t, func() error {
		for i := range 3 {
			if _, _, err := peer.ReadMessage(); err != nil {
				return fmt.Errorf("message %d: %v", i, err)
			}
		}
		return nil
	})
	if err := server.Write(codec.Text, []byte("end")); err != nil {
		t.Fatal(err)
	}
	wait()
	if len(got) != 1 || got[0] != "ping 49" {
		t.Fatalf("pongs %q, want only the last", got)
	}
}

// TestQueueFailureEndsRead: a failed flush closes the transport, so a read
// in progress returns, with the queue's error rather than the closed read's,
// even though the peer is silent and nothing more is sent.
func TestQueueFailureEndsRead(t *testing.T) {
	server, _ := pair(t, ws.Config{}, ws.Config{})
	q := server.NewQueue(0)
	read := make(chan error, 1)
	go func() {
		_, _, err := server.ReadMessage() // The peer never sends.
		read <- err
	}()

	// The peer never reads either, so the flush fails at the deadline.
	server.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
	if err := q.Send(codec.Binary, []byte("never read")); err != nil {
		t.Fatal(err)
	}
	if err := q.Wait(); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("flush: %v", err)
	}
	select {
	case err := <-read:
		if err != q.Err() {
			t.Fatalf("read returned %v, want the queue's %v", err, q.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("the flush failed but the read is still blocked")
	}
}

// TestQueueFailureCloseWritten: the close a protocol error sends goes out
// behind the data queued before it and is written before the read returns,
// so a handler that closes the transport when its read ends does not cut
// either off.
func TestQueueFailureCloseWritten(t *testing.T) {
	server, peer := raw(t, ws.Config{})
	got := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(peer)
		got <- b
	}()
	q := server.NewQueue(0)
	if err := q.Send(codec.Binary, []byte("queued")); err != nil {
		t.Fatal(err)
	}
	go peer.Write([]byte{0x81, 1, 'a'}) // Unmasked: a protocol error.
	if _, _, err := server.ReadMessage(); err == nil {
		t.Fatal("unmasked frame accepted")
	}
	server.Transport().Close()

	want := []byte{0x82, 6, 'q', 'u', 'e', 'u', 'e', 'd', 0x88, 2, 0x03, 0xea}
	select {
	case b := <-got:
		if !bytes.Equal(b, want) {
			t.Fatalf("peer read % x, want % x", b, want)
		}
	case <-time.After(time.Second):
		t.Fatal("peer read did not end")
	}
}
