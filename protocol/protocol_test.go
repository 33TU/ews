package protocol_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/protocol"
	"github.com/klauspost/compress/flate"
)

func receiver(t testing.TB, config protocol.ReceiverConfig) *protocol.Receiver {
	t.Helper()
	r, err := protocol.NewReceiver(config)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func sender(t testing.TB, config protocol.SenderConfig) *protocol.Sender {
	t.Helper()
	s, err := protocol.NewSender(config)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func frame(t testing.TB, first byte, payload []byte, masked bool) []byte {
	t.Helper()
	var enc codec.Encoder
	var key *[4]byte
	if masked {
		key = &[4]byte{17, 28, 39, 40}
	}
	if err := enc.Encode(true, codec.Binary, payload, key); err != nil {
		t.Fatal(err)
	}
	out := append(bytes.Clone(enc.HeaderBytes()), enc.PayloadBytes()...)
	out[0] = first
	return out
}

func readFrame(t testing.TB, wire []byte) (codec.Header, []byte) {
	t.Helper()
	var dec codec.Decoder
	dec.Feed(bytes.Clone(wire))
	h, ok, err := dec.NextHeader()
	if err != nil || !ok {
		t.Fatalf("header: %v, %v", ok, err)
	}
	p, done := dec.Payload()
	if !done {
		t.Fatal("incomplete output")
	}
	if h.Masked() {
		codec.Mask(p, [4]byte(h.MaskKey()), 0)
	}
	if _, ok, err := dec.NextHeader(); ok || err != nil {
		t.Fatal("unexpected extra output")
	}
	return h, p
}

func receive(t testing.TB, r *protocol.Receiver, wire []byte, chunkSize int) protocol.Event {
	t.Helper()
	var event protocol.Event
	found := false
	original := bytes.Clone(wire)
	for off := 0; off < len(wire); {
		n := min(chunkSize, len(wire)-off)
		r.Feed(wire[off : off+n])
		off += n
		for {
			e, ok, err := r.NextEvent()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			if found {
				t.Fatal("unexpected second event")
			}
			found = true
			event = protocol.Event{Opcode: e.Opcode, Payload: bytes.Clone(e.Payload)}
		}
	}
	if !found {
		t.Fatal("missing event")
	}
	if !bytes.Equal(wire, original) {
		t.Fatal("Feed modified input")
	}
	return event
}

func TestMessages(t *testing.T) {
	for _, compression := range []string{"none", "fresh", "takeover"} {
		for _, role := range []protocol.Role{protocol.Server, protocol.Client} {
			for _, chunk := range []int{1, 7, 65536} {
				t.Run(fmt.Sprintf("%s/role=%d/chunk=%d", compression, role, chunk), func(t *testing.T) {
					sc := protocol.SenderConfig{Role: role}
					rc := protocol.ReceiverConfig{Role: 1 - role}
					if compression != "none" {
						sc.Compression = &protocol.Compression{Level: flate.DefaultCompression, ContextTakeover: compression == "takeover"}
						rc.Compression = true
						rc.ContextTakeover = sc.Compression.ContextTakeover
					}
					s, r := sender(t, sc), receiver(t, rc)
					var dst []byte
					for round := 0; round < 2; round++ {
						s.Reset()
						r.Reset()
						for i, p := range [][]byte{nil, []byte("hello 😀"), bytes.Repeat([]byte("message"), 1000), []byte("plain"), bytes.Repeat([]byte("message"), 1000), {255, 0, 254}} {
							op := codec.Text
							if i == 5 {
								op = codec.Binary
							}
							compress := compression != "none" && i != 3
							var err error
							dst, err = s.Append(dst[:0], op, p, compress)
							if err != nil {
								t.Fatal(err)
							}
							h, _ := readFrame(t, dst)
							if h.Masked() != (role == protocol.Client) || h.RSV1() != compress || !h.Final() {
								t.Fatal("invalid header")
							}
							e := receive(t, r, dst, chunk)
							if e.Opcode != op || !bytes.Equal(e.Payload, p) {
								t.Fatalf("message %d: %v", i, e)
							}
						}
					}
				})
			}
		}
	}
}

func BenchmarkProtocol(b *testing.B) {
	for _, size := range []int{125, 4096} {
		for _, role := range []protocol.Role{protocol.Server, protocol.Client} {
			b.Run(fmt.Sprintf("size=%d/role=%d", size, role), func(b *testing.B) {
				s := sender(b, protocol.SenderConfig{Role: role})
				r := receiver(b, protocol.ReceiverConfig{Role: 1 - role})
				p := bytes.Repeat([]byte("x"), size)
				var dst []byte
				run := func() {
					var err error
					dst, err = s.Append(dst[:0], codec.Text, p, false)
					if err != nil {
						b.Fatal(err)
					}
					r.Feed(dst)
					if _, ok, err := r.NextEvent(); !ok || err != nil {
						b.Fatalf("receive: %v", err)
					}
				}
				run()
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					run()
				}
			})
		}
	}
}
