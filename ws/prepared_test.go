package ws_test

import (
	"bytes"
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

	// Through real peers: a client masks and falls back to Write; a takeover
	// server interleaves prepared and ordinary compressed messages, and a
	// Batch carries prepared messages too.
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

	server, client = pair(t, ws.Config{}, ws.Config{})
	wait = run(t, func() error {
		for _, want := range [][]byte{payload, []byte("plain"), payload} {
			if _, got, err := client.ReadMessage(); err != nil || !bytes.Equal(got, want) {
				return fmt.Errorf("batch: %v", err)
			}
		}
		return nil
	})
	b := server.NewBatch()
	b.WritePrepared(p)
	b.Write(codec.Binary, []byte("plain"))
	b.WritePrepared(p)
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	wait()

	// Reference counting: the owner may release right after handing the
	// message to a queue, which keeps its own reference until written, and
	// over-release panics.
	server, client = pair(t, ws.Config{}, ws.Config{})
	q := server.NewQueue(0)
	p2, _ := ws.Prepare(codec.Binary, payload)
	wait = run(t, func() error {
		if _, got, err := client.ReadMessage(); err != nil || !bytes.Equal(got, payload) {
			return fmt.Errorf("released prepared: %v", err)
		}
		return nil
	})
	if err := q.SendPrepared(p2); err != nil {
		t.Fatal(err)
	}
	p2.Release()
	if err := q.Wait(); err != nil {
		t.Fatal(err)
	}
	wait()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("double release did not panic")
			}
		}()
		p2.Release()
	}()
	p.Release()
}
