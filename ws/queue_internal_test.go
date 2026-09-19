package ws

import (
	"bytes"
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
