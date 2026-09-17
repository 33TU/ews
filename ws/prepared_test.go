package ws_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

func TestPrepared(t *testing.T) {
	payload := bytes.Repeat([]byte("prepared once, sent many times "), 40)
	p, err := ws.Prepare(codec.Text, payload)
	if err != nil || p.Opcode() != codec.Text || !bytes.Equal(p.Payload(), payload) {
		t.Fatal(err)
	}
	if _, err := ws.Prepare(codec.Ping, nil); err != ws.ErrProtocol {
		t.Fatal("control frame prepared")
	}

	// Plain server: the shared bytes go out as one frame without RSV1.
	// Compressing server: RSV1 and a payload that inflates to the message.
	// Tiny payloads stay plain under MinSize.
	tiny, _ := ws.Prepare(codec.Binary, []byte("tiny"))
	for _, compress := range []bool{false, true} {
		var cfg ws.Config
		if compress {
			cfg.Compression = &handshake.Compression{Level: flate.BestSpeed}
		}
		server, peer := raw(t, cfg)
		wait := run(t, func() error {
			h, frame := readFrame(t, peer)
			if h.Opcode() != codec.Text || !h.Final() || h.RSV1() != compress {
				return fmt.Errorf("compress=%t: header %x", compress, h.Bytes())
			}
			if compress {
				var d deflate.Decompressor
				out, err := d.Decompress(frame, len(payload), nil)
				if err != nil {
					return err
				}
				frame = bytes.Clone(out)
			}
			if !bytes.Equal(frame, payload) {
				return fmt.Errorf("compress=%t: payload mismatch", compress)
			}
			if h, frame := readFrame(t, peer); h.RSV1() || string(frame) != "tiny" {
				return fmt.Errorf("tiny message header %x", h.Bytes())
			}
			return nil
		})
		if err := server.WritePrepared(p); err != nil {
			t.Fatal(err)
		}
		if err := server.WritePrepared(tiny); err != nil {
			t.Fatal(err)
		}
		wait()
	}

	// Through real peers: a client masks and falls back to Write, and a
	// takeover server interleaves prepared and ordinary compressed messages.
	server, client := compressionPair(t, true, true, 1)
	wait := run(t, func() error {
		for i := 0; i < 4; i++ {
			if _, got, err := client.ReadMessage(); err != nil || !bytes.Equal(got, payload) {
				return fmt.Errorf("client message %d: %v", i, err)
			}
		}
		if _, got, err := server.ReadMessage(); err != nil || !bytes.Equal(got, payload) {
			return fmt.Errorf("server message: %v", err)
		}
		return nil
	})
	for i := 0; i < 2; i++ {
		if err := server.WritePrepared(p); err != nil {
			t.Fatal(err)
		}
		if err := server.Write(codec.Text, payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.WritePrepared(p); err != nil {
		t.Fatal(err)
	}
	wait()

}

// TestPreparedTakeoverHistory interleaves prepared and ordinary compressed
// messages on a takeover connection across the window size, so the deferred
// history update is exercised in every branch: payloads that accumulate
// below the window, one that covers it, and the connection's own compressed
// messages that must see the right dictionary each time.
func TestPreparedTakeoverHistory(t *testing.T) {
	server, client := compressionPair(t, true, true, 1)
	var want [][]byte
	text := func(i, size int) []byte {
		return bytes.Repeat([]byte(fmt.Sprintf("msg %02d payload text ", i)), size/20+1)[:size]
	}
	var prepared []*ws.Prepared
	for i := 0; i < 24; i++ {
		var p []byte
		switch {
		case i == 11:
			p = text(i, 40<<10) // Larger than the 32 KB window.
		case i%4 == 3:
			p = text(i, 300)
		default:
			p = text(i, 4<<10)
		}
		pr, err := ws.Prepare(codec.Text, p)
		if err != nil {
			t.Fatal(err)
		}
		prepared = append(prepared, pr)
		want = append(want, p)
	}
	wait := run(t, func() error {
		for i, w := range want {
			_, got, err := client.ReadMessage()
			if err != nil {
				return fmt.Errorf("message %d: %v", i, err)
			}
			if !bytes.Equal(got, w) {
				return fmt.Errorf("message %d: got %d bytes, want %d", i, len(got), len(w))
			}
		}
		return nil
	})
	for i, pr := range prepared {
		if i%4 == 3 {
			// The connection's own compressed message, primed from the
			// history the prepared messages built.
			if err := server.Write(codec.Text, want[i]); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := server.WritePrepared(pr); err != nil {
			t.Fatal(err)
		}
	}
	wait()
}

// PrepareAppend hands the encoder the frame itself: the payload lands after
// the header with no copy, with or without a size hint, and goes out as the
// same frame Prepare would have built.
func TestPrepareAppend(t *testing.T) {
	payload := bytes.Repeat([]byte("appended in place "), 200)
	fill := func(dst []byte) ([]byte, error) { return append(dst, payload...), nil }
	for _, hint := range []int{0, len(payload)} {
		p, err := ws.PrepareAppend(codec.Text, hint, fill)
		if err != nil || p.Opcode() != codec.Text || !bytes.Equal(p.Payload(), payload) {
			t.Fatalf("hint %d: %v", hint, err)
		}
		server, peer := raw(t, ws.Config{})
		wait := run(t, func() error {
			h, frame := readFrame(t, peer)
			if h.Opcode() != codec.Text || !h.Final() || !bytes.Equal(frame, payload) {
				return fmt.Errorf("hint %d: header %x, %d bytes", hint, h.Bytes(), len(frame))
			}
			return nil
		})
		if err := server.WritePrepared(p); err != nil {
			t.Fatal(err)
		}
		wait()
	}
	if n := testing.AllocsPerRun(20, func() { ws.PrepareAppend(codec.Binary, len(payload), fill) }); n > 2 {
		t.Fatalf("PrepareAppend allocates %.0f times, want the frame and the Prepared", n)
	}
	empty, err := ws.PrepareAppend(codec.Binary, 0, func(dst []byte) ([]byte, error) { return dst, nil })
	if err != nil || len(empty.Payload()) != 0 {
		t.Fatal("empty payload", err)
	}
	if _, err := ws.PrepareAppend(codec.Ping, 0, fill); err != ws.ErrProtocol {
		t.Fatal("control frame prepared")
	}
	fail := errors.New("encoder failed")
	if _, err := ws.PrepareAppend(codec.Binary, 0, func([]byte) ([]byte, error) { return nil, fail }); err != fail {
		t.Fatalf("fill error not returned: %v", err)
	}
	if _, err := ws.PrepareAppend(codec.Binary, 0, func([]byte) ([]byte, error) { return []byte("x"), nil }); err != ws.ErrProtocol {
		t.Fatal("a fill that drops dst was accepted")
	}
}
