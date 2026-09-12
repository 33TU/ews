package proto_test

import (
	"bytes"
	"errors"
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

func TestFragmentsAndControls(t *testing.T) {
	for _, masked := range []bool{false, true} {
		t.Run(fmt.Sprintf("masked=%t", masked), func(t *testing.T) {
			var r proto.Receiver
			r.Init(proto.Client)
			if masked {
				r.Init(proto.Server)
			}
			payload := []byte("A😀B€C")
			var wire []byte
			for i := range payload {
				op := byte(codec.Continuation)
				if i == 0 {
					op = byte(codec.Text)
				}
				if i == len(payload)-1 {
					op |= 0x80
				}
				wire = append(wire, frame(t, op, payload[i:i+1], masked)...)
				if i == 1 {
					wire = append(wire, frame(t, 0x89, []byte("ping"), masked)...)
				}
			}
			wire = append(wire, frame(t, 0x8a, []byte("unsolicited"), masked)...)
			wire = append(wire, frame(t, 0x81, nil, masked)...)
			events, err := drive(t, &r, wire, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 4 ||
				!events[0].control || events[0].opcode != codec.Ping || string(events[0].payload) != "ping" ||
				events[1].control || events[1].opcode != codec.Text || !bytes.Equal(events[1].payload, payload) ||
				!events[2].control || events[2].opcode != codec.Pong ||
				events[3].control || events[3].opcode != codec.Text || len(events[3].payload) != 0 {
				t.Fatalf("events: %+v", events)
			}
		})
	}
}

func TestInvalidFrames(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
		code uint16
	}{
		{"masked server", frame(t, 0x81, nil, true), 1002},
		{"rsv1", frame(t, 0xc1, nil, false), 1002},
		{"rsv2", frame(t, 0xa1, nil, false), 1002},
		{"rsv3", frame(t, 0x91, nil, false), 1002},
		{"opcode", frame(t, 0x83, nil, false), 1002},
		{"control opcode", frame(t, 0x8b, nil, false), 1002},
		{"continuation", frame(t, 0x80, nil, false), 1002},
		{"nested message", append(frame(t, 0x01, nil, false), frame(t, 0x82, nil, false)...), 1002},
		{"fragmented ping", frame(t, 0x09, nil, false), 1002},
		{"large ping", frame(t, 0x89, make([]byte, 126), false), 1002},
		{"close length", frame(t, 0x88, []byte{1}, false), 1002},
		{"close code", frame(t, 0x88, []byte{3, 237}, false), 1002},
		{"close utf8", frame(t, 0x88, []byte{3, 232, 255}, false), 1007},
		{"text utf8", frame(t, 0x81, []byte{255}, false), 1007},
		{"fragmented utf8", append(frame(t, 0x01, []byte{0xf0, 0x9f}, false), frame(t, 0x80, nil, false)...), 1007},
		{"split rune broken", append(frame(t, 0x01, []byte{0xe2}, false), frame(t, 0x80, []byte{0x41}, false)...), 1007},
		{"nonminimal length", []byte{0x82, 126, 0, 1}, 1002},
		{"invalid length", []byte{0x82, 127, 128, 0, 0, 0, 0, 0, 0, 0}, 1002},
	}
	for _, tt := range tests {
		for _, chunk := range []int{1, 1 << 16} {
			t.Run(fmt.Sprintf("%s/chunk=%d", tt.name, chunk), func(t *testing.T) {
				var r proto.Receiver
				r.Init(proto.Client)
				_, err := drive(t, &r, tt.wire, chunk)
				var pe *proto.Error
				if !errors.As(err, &pe) || pe.Code != tt.code {
					t.Fatalf("expected code %d, got %v", tt.code, err)
				}
				r.Feed(frame(t, 0x81, nil, false))
				if _, again := r.Next(); again != err {
					t.Fatal("failure was not terminal")
				}
				r.Init(proto.Client)
				if _, err := drive(t, &r, frame(t, 0x81, []byte("ok"), false), chunk); err != nil {
					t.Fatalf("Init did not recover: %v", err)
				}
			})
		}
	}
}

func TestCloseReceived(t *testing.T) {
	var r proto.Receiver
	r.Init(proto.Client)
	wire := append(frame(t, 0x88, []byte{3, 232, 'b', 'y', 'e'}, false), frame(t, 0x81, []byte("late"), false)...)
	events, err := drive(t, &r, wire, len(wire))
	if err != nil || len(events) != 1 || !events[0].control || events[0].opcode != codec.Close {
		t.Fatalf("events %+v, err %v", events, err)
	}
	code, reason := proto.ParseClose(events[0].payload)
	if code != 1000 || string(reason) != "bye" || !r.CloseReceived() {
		t.Fatalf("code %d reason %q", code, reason)
	}
	if kind, err := r.Next(); kind != proto.NeedInput || err != nil {
		t.Fatal("input after close must be ignored")
	}
	if code, reason := proto.ParseClose(nil); code != proto.NoStatus || reason != nil {
		t.Fatal("empty close must report NoStatus")
	}
}

func TestSender(t *testing.T) {
	var s proto.Sender
	s.Init(proto.Client)
	wire := encode(t, &s, codec.Text, []byte("hi"))
	if wire[1]&0x80 == 0 || len(wire) != 2+4+2 {
		t.Fatalf("client frame not masked: %x", wire)
	}
	if _, _, err := s.Encode(codec.Ping, make([]byte, 126)); err != proto.ErrProtocol {
		t.Fatal("oversized ping accepted")
	}
	if _, _, err := s.Encode(codec.Continuation, nil); err != proto.ErrProtocol {
		t.Fatal("continuation accepted")
	}
	for _, tt := range []struct {
		code   uint16
		reason string
	}{{0, "reason"}, {1000, string(make([]byte, 124))}, {1005, ""}, {999, ""}} {
		if _, _, err := s.EncodeClose(tt.code, tt.reason); err == nil {
			t.Fatalf("close %d with %d-byte reason accepted", tt.code, len(tt.reason))
		}
	}
	if s.CloseSent() {
		t.Fatal("rejected close marked as sent")
	}
	h, b, err := s.EncodeClose(1001, "bye")
	if err != nil || !s.CloseSent() {
		t.Fatal(err)
	}
	var r proto.Receiver
	r.Init(proto.Server)
	events, err := drive(t, &r, append(bytes.Clone(h), b...), 1)
	if err != nil || len(events) != 1 {
		t.Fatalf("events %+v, err %v", events, err)
	}
	if code, reason := proto.ParseClose(events[0].payload); code != 1001 || string(reason) != "bye" {
		t.Fatalf("code %d reason %q", code, reason)
	}
	if _, _, err := s.Encode(codec.Text, nil); err != proto.ErrClosing {
		t.Fatal("data after close accepted")
	}
	if _, _, err := s.Encode(codec.Pong, nil); err != nil {
		t.Fatal("pong after close rejected")
	}
	s.Init(proto.Server)
	if _, _, err := s.Encode(codec.Text, nil); err != nil || s.CloseSent() {
		t.Fatal("Init did not clear close state")
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
