package ws

import (
	"bytes"
	"io"
	"testing"

	"github.com/33TU/ews/codec"
)

type sink struct{}

func (sink) Read([]byte) (int, error)    { return 0, io.EOF }
func (sink) Write(p []byte) (int, error) { return len(p), nil }

// TestQueueReleasesLargeArenas checks that a burst larger than queueRetain
// does not leave its arena attached to the queue once written; it goes back
// to the shared pool instead.
func TestQueueReleasesLargeArenas(t *testing.T) {
	c, err := NewConn(sink{}, Config{Role: Server})
	if err != nil {
		t.Fatal(err)
	}
	q := c.NewQueue(0)
	if err := q.Send(codec.Binary, []byte("small")); err != nil { // Creates the private arena.
		t.Fatal(err)
	}
	if err := q.Wait(); err != nil {
		t.Fatal(err)
	}
	big := bytes.Repeat([]byte("x"), 8<<10)
	for i := 0; i < 16; i++ { // 128 KiB queued before the writer can drain it all.
		if err := q.Send(codec.Binary, big); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Wait(); err != nil {
		t.Fatal(err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if cap(q.arena) > queueRetain || cap(q.flushArena) > queueRetain {
		t.Fatalf("arenas retained %d and %d bytes after a burst", cap(q.arena), cap(q.flushArena))
	}
	if cap(q.arena)+cap(q.flushArena) == 0 {
		t.Fatal("private arena was not restored after the pooled one was released")
	}
}
