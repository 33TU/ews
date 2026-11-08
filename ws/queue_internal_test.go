package ws

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/33TU/ews/codec"
)

type sink struct{ nopConn }

func (sink) Read([]byte) (int, error)    { return 0, io.EOF }
func (sink) Write(p []byte) (int, error) { return len(p), nil }

// TestQueueHoldsNoIdleArena checks that a queue keeps no frame storage once
// its bursts are written: arenas come from the shared pool and go back.
func TestQueueHoldsNoIdleArena(t *testing.T) {
	c, err := NewConn(sink{}, Config{Role: Server})
	if err != nil {
		t.Fatal(err)
	}
	q := c.NewQueue(0)
	big := bytes.Repeat([]byte("x"), 8<<10)
	for round := range 3 {
		if err := q.Send(codec.Binary, []byte("small")); err != nil {
			t.Fatal(err)
		}
		for range 16 {
			if err := q.Send(codec.Binary, big); err != nil {
				t.Fatal(err)
			}
		}
		if err := q.Wait(); err != nil {
			t.Fatal(err)
		}
		q.mu.Lock()
		cur, flushing := q.cur, q.flushing
		q.mu.Unlock()
		if cur != nil || flushing != nil {
			t.Fatalf("round %d: queue still holds an arena while idle", round)
		}
	}
}

// nopConn supplies the net.Conn methods a transport double never uses.
type nopConn struct{}

func (nopConn) Close() error                     { return nil }
func (nopConn) LocalAddr() net.Addr              { return nil }
func (nopConn) RemoteAddr() net.Addr             { return nil }
func (nopConn) SetDeadline(time.Time) error      { return nil }
func (nopConn) SetReadDeadline(time.Time) error  { return nil }
func (nopConn) SetWriteDeadline(time.Time) error { return nil }

// held is a transport whose writes block until release is closed.
type held struct {
	nopConn
	release chan struct{}
}

func (held) Read([]byte) (int, error) { return 0, io.EOF }
func (h held) Write(p []byte) (int, error) {
	<-h.release
	return len(p), nil
}

// TestQueuePongSwapReusesArena: past the limit each pong replaces the last
// one in place, so a flood does not grow the arena, even when the pong it
// replaces has a frame queued after it.
func TestQueuePongSwapReusesArena(t *testing.T) {
	h := held{release: make(chan struct{})}
	c, err := NewConn(h, Config{Role: Server})
	if err != nil {
		t.Fatal(err)
	}
	q := c.NewQueue(100)
	if err := q.Send(codec.Binary, make([]byte, 60)); err != nil {
		t.Fatal(err)
	}
	for q.Pending() != 0 {
		time.Sleep(time.Millisecond) // The writer has taken the frame and blocks on it.
	}
	if err := q.Send(codec.Binary, make([]byte, 160)); err != nil {
		t.Fatal(err)
	}
	if err := c.Pong([]byte("first")); err != nil {
		t.Fatal(err)
	}
	// A synchronous write would wait, so queue a frame after the pong directly.
	c.wmu.Lock()
	_, err = q.enqueue([]byte{0x82, 1}, []byte{'x'}, nil)
	c.wmu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	for i := range 100 {
		if err := c.Pong(fmt.Appendf(nil, "pong %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	q.mu.Lock()
	segments, arena, pong := len(q.segments), len(q.cur.b), q.segments[1]
	frame := string(q.cur.b[pong.start:pong.end])
	q.mu.Unlock()
	// 160 bytes with a 4-byte header, the 3-byte frame, and two pongs: the
	// first one's bytes, left behind by the swap, and the latest.
	if want := 164 + 3 + 7 + 9; segments != 3 || arena != want || frame != "\x8a\x07pong 99" {
		t.Fatalf("%d segments, arena %d bytes (want %d), pong %q", segments, arena, want, frame)
	}
	close(h.release)
	if err := q.Wait(); err != nil {
		t.Fatal(err)
	}
}
