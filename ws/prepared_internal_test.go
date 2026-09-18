package ws

import (
	"io"
	"net"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
)

// TestPendingHistory checks the bookkeeping of deferred prepared payloads:
// small ones accumulate until they cover the window, a large one supersedes
// them all, and the connection's own compressed write copies them in.
func TestPendingHistory(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go io.Copy(io.Discard, b)
	c, err := NewConn(a, Config{Role: Server, Compression: &handshake.Compression{Level: 1, SendContextTakeover: true, ReceiveContextTakeover: true}})
	if err != nil {
		t.Fatal(err)
	}
	small := make([]*Prepared, 10)
	for i := range small {
		small[i], _ = Prepare(codec.Binary, make([]byte, 4<<10))
	}
	for i := range 8 {
		if err := c.WritePrepared(small[i]); err != nil {
			t.Fatal(err)
		}
	}
	if c.comp.npending != 8 || c.comp.pendingBytes != 32<<10 {
		t.Fatalf("after 8 x 4 KiB: %d pending, %d bytes", c.comp.npending, c.comp.pendingBytes)
	}
	// The ninth makes the last eight cover the window: the oldest is dropped.
	if err := c.WritePrepared(small[8]); err != nil {
		t.Fatal(err)
	}
	if c.comp.npending != 8 || c.comp.pending[0] != small[1] {
		t.Fatalf("after 9: %d pending, oldest %p (want %p)", c.comp.npending, c.comp.pending[0], small[1])
	}
	// A payload of the window size supersedes everything.
	large, _ := Prepare(codec.Binary, make([]byte, 32<<10))
	if err := c.WritePrepared(large); err != nil {
		t.Fatal(err)
	}
	if c.comp.npending != 1 || c.comp.pending[0] != large {
		t.Fatalf("after large: %d pending", c.comp.npending)
	}
	// The connection's own compressed message applies the rest.
	if err := c.Write(codec.Binary, make([]byte, 1<<10)); err != nil {
		t.Fatal(err)
	}
	if c.comp.npending != 0 || c.comp.pendingBytes != 0 || c.comp.pending[0] != nil || c.comp.window.Size() != 32<<10 {
		t.Fatalf("after own write: %d pending", c.comp.npending)
	}
}
