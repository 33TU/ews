package proto_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/internal/proto"
)

// frame builds one frame and overwrites its first byte for header variants.
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

type event struct {
	opcode  codec.Opcode
	payload []byte
	control bool
}

// drive feeds wire in chunks and collects complete messages and control frames
// the way a blocking or event-driven consumer would.
func drive(t testing.TB, r *proto.Receiver, wire []byte, chunk int) ([]event, error) {
	t.Helper()
	var events []event
	var message []byte
	var opcode codec.Opcode
	for off := 0; off < len(wire); {
		n := min(chunk, len(wire)-off)
		r.Feed(wire[off : off+n])
		off += n
		for {
			if r.Remaining() != 0 {
				p, done, err := r.Payload()
				if err != nil {
					return events, err
				}
				message = append(message, p...)
				if !done {
					break
				}
				if !r.MessageOpen() {
					events = append(events, event{opcode, bytes.Clone(message), false})
				}
				continue
			}
			kind, err := r.Next()
			if err != nil {
				return events, err
			}
			if kind == proto.NeedInput {
				break
			}
			if kind == proto.ControlFrame {
				events = append(events, event{r.ControlOpcode(), bytes.Clone(r.ControlPayload()), true})
				continue
			}
			if h := r.Header(); h.Opcode() != codec.Continuation {
				message, opcode = message[:0], h.Opcode()
			}
			if r.Remaining() == 0 && !r.MessageOpen() {
				events = append(events, event{opcode, bytes.Clone(message), false})
			}
		}
	}
	return events, nil
}

func encode(t testing.TB, s *proto.Sender, op codec.Opcode, payload []byte) []byte {
	t.Helper()
	h, b, err := s.Encode(op, payload)
	if err != nil {
		t.Fatal(err)
	}
	return append(bytes.Clone(h), b...)
}

func TestRoundTrip(t *testing.T) {
	messages := []struct {
		op      codec.Opcode
		payload []byte
	}{
		{codec.Text, nil},
		{codec.Text, []byte("hello 😀")},
		{codec.Binary, bytes.Repeat([]byte("message"), 1000)},
		{codec.Text, bytes.Repeat([]byte("κόσμε"), 3000)},
		{codec.Binary, []byte{255, 0, 254}},
	}
	for _, role := range []proto.Role{proto.Server, proto.Client} {
		for _, chunk := range []int{1, 7, 65536} {
			t.Run(fmt.Sprintf("role=%d/chunk=%d", role, chunk), func(t *testing.T) {
				var s proto.Sender
				var r proto.Receiver
				s.Init(role)
				r.Init(1 - role)
				var wire []byte
				for _, m := range messages {
					wire = append(wire, encode(t, &s, m.op, m.payload)...)
				}
				events, err := drive(t, &r, wire, chunk)
				if err != nil {
					t.Fatal(err)
				}
				if len(events) != len(messages) {
					t.Fatalf("got %d events, want %d", len(events), len(messages))
				}
				for i, m := range messages {
					if e := events[i]; e.control || e.opcode != m.op || !bytes.Equal(e.payload, m.payload) {
						t.Fatalf("message %d mismatch: %v", i, e.opcode)
					}
				}
				if !r.Idle() {
					t.Fatal("receiver not idle after complete input")
				}
			})
		}
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	for _, size := range []int{125, 4096} {
		for _, role := range []proto.Role{proto.Server, proto.Client} {
			b.Run(fmt.Sprintf("size=%d/role=%d", size, role), func(b *testing.B) {
				var s proto.Sender
				var r proto.Receiver
				s.Init(role)
				r.Init(1 - role)
				payload := bytes.Repeat([]byte("x"), size)
				wire := make([]byte, 0, size+14)
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					h, body, err := s.Encode(codec.Binary, payload)
					if err != nil {
						b.Fatal(err)
					}
					wire = append(append(wire[:0], h...), body...)
					r.Feed(wire)
					if kind, err := r.Next(); kind != proto.DataFrame || err != nil {
						b.Fatalf("next: %v %v", kind, err)
					}
					if _, done, err := r.Payload(); !done || err != nil {
						b.Fatalf("payload: %v %v", done, err)
					}
				}
			})
		}
	}
}
