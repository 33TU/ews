package protocol_test

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/protocol"
	"github.com/klauspost/compress/flate"
	"testing"
)

func TestFragmentsAndControls(t *testing.T) {
	for _, masked := range []bool{false, true} {
		for _, compressed := range []bool{false, true} {
			t.Run(fmt.Sprintf("masked=%t/compressed=%t", masked, compressed), func(t *testing.T) {
				config := protocol.ReceiverConfig{Role: protocol.Client}
				if masked {
					config.Role = protocol.Server
				}
				payload := []byte("A😀B€C")
				wirePayload := payload
				first := byte(codec.Text)
				if compressed {
					config.Compression = true
					c, err := deflate.NewCompressor(flate.BestSpeed)
					if err != nil {
						t.Fatal(err)
					}
					wirePayload, err = c.Compress(payload)
					if err != nil {
						t.Fatal(err)
					}
					first |= 0x40
				}
				s := receiver(t, config)
				var wire []byte
				for i := range wirePayload {
					op := byte(codec.Continuation)
					if i == 0 {
						op = first
					}
					if i == len(wirePayload)-1 {
						op |= 0x80
					}
					wire = append(wire, frame(t, op, wirePayload[i:i+1], masked)...)
					if i == 1 {
						wire = append(wire, frame(t, 0x89, []byte("ping"), masked)...)
					}
				}
				wire = append(wire, frame(t, 0x8a, []byte("unsolicited"), masked)...)
				var events []protocol.Event
				for _, b := range wire {
					s.Feed([]byte{b})
					for {
						e, ok, err := s.NextEvent()
						if err != nil {
							t.Fatal(err)
						}
						if !ok {
							break
						}
						events = append(events, protocol.Event{Opcode: e.Opcode, Payload: bytes.Clone(e.Payload)})
					}
				}
				if len(events) != 3 || events[0].Opcode != codec.Ping || events[1].Opcode != codec.Text || !bytes.Equal(events[1].Payload, payload) || events[2].Opcode != codec.Pong {
					t.Fatalf("events: %v", events)
				}
			})
		}
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
		{"rsv1 continuation", append(frame(t, 0x01, nil, false), frame(t, 0xc0, nil, false)...), 1002},
		{"fragmented ping", frame(t, 0x09, nil, false), 1002},
		{"compressed ping", frame(t, 0xc9, nil, false), 1002},
		{"large ping", frame(t, 0x89, make([]byte, 126), false), 1002},
		{"close length", frame(t, 0x88, []byte{1}, false), 1002},
		{"close code", frame(t, 0x88, []byte{3, 237}, false), 1002},
		{"close utf8", frame(t, 0x88, []byte{3, 232, 255}, false), 1007},
		{"text utf8", frame(t, 0x81, []byte{255}, false), 1007},
		{"fragmented utf8", append(frame(t, 0x01, []byte{0xf0, 0x9f}, false), frame(t, 0x80, nil, false)...), 1007},
		{"nonminimal length", []byte{0x82, 126, 0, 1}, 1002},
		{"invalid length", []byte{0x82, 127, 128, 0, 0, 0, 0, 0, 0, 0}, 1002},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := receiver(t, protocol.ReceiverConfig{Role: protocol.Client})
			s.Feed(tt.wire)
			_, ok, err := s.NextEvent()
			var pe *protocol.Error
			if ok || !errors.As(err, &pe) || pe.Code != tt.code {
				t.Fatalf("expected %d: %v, %v", tt.code, ok, err)
			}
			s.Feed(frame(t, 0x81, nil, false))
			if _, _, again := s.NextEvent(); again != err {
				t.Fatal("failure was not terminal")
			}
			s.Reset()
			s.Feed(frame(t, 0x81, []byte("recovered"), false))
			if e, ok, err := s.NextEvent(); err != nil || !ok || string(e.Payload) != "recovered" {
				t.Fatal("reset failed")
			}
		})
	}
	s := receiver(t, protocol.ReceiverConfig{})
	s.Feed(frame(t, 0x81, nil, false))
	if _, _, err := s.NextEvent(); !errors.Is(err, protocol.ErrProtocol) {
		t.Fatal("unmasked client accepted")
	}
}

func TestLimitsAndCompressionErrors(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	large, err := c.Compress(bytes.Repeat([]byte("x"), 1000))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		wire []byte
		code uint16
	}{
		{"header limit", frame(t, 0x82, make([]byte, 65), false)[:2], 1009},
		{"fragment limit", append(frame(t, 0x02, make([]byte, 40), false), frame(t, 0x80, make([]byte, 25), false)...), 1009},
		{"inflate limit", frame(t, 0xc2, large, false), 1009},
		{"invalid deflate", frame(t, 0xc2, []byte{6}, false), 1007},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := receiver(t, protocol.ReceiverConfig{Role: protocol.Client, MaxMessageSize: 64, Compression: true})
			s.Feed(tt.wire)
			_, _, err := s.NextEvent()
			var pe *protocol.Error
			if !errors.As(err, &pe) || pe.Code != tt.code {
				t.Fatalf("expected %d: %v", tt.code, err)
			}
		})
	}
}

func TestEmptyFragmentsAndCoalescedMessages(t *testing.T) {
	s := receiver(t, protocol.ReceiverConfig{Role: protocol.Client})
	var wire []byte
	for _, f := range []struct {
		op byte
		p  []byte
	}{
		{0x01, nil}, {0x00, nil}, {0x00, []byte("hello")}, {0x80, nil},
		{0x81, nil}, {0x82, []byte{255}},
	} {
		wire = append(wire, frame(t, f.op, f.p, false)...)
	}
	s.Feed(wire)
	for _, want := range []protocol.Event{{Opcode: codec.Text, Payload: []byte("hello")}, {Opcode: codec.Text}, {Opcode: codec.Binary, Payload: []byte{255}}} {
		got, ok, err := s.NextEvent()
		if err != nil || !ok || got.Opcode != want.Opcode || !bytes.Equal(got.Payload, want.Payload) {
			t.Fatalf("coalesced: %v, %v", got, err)
		}
	}
	if _, ok, err := s.NextEvent(); ok || err != nil {
		t.Fatal("unexpected event")
	}
}

func FuzzReceiverChunking(f *testing.F) {
	f.Add([]byte{0x81, 2, 'h', 'i'}, false)
	f.Add([]byte{0x01, 1, 'h', 0x89, 0, 0x80, 1, 'i', 0x88, 0}, false)
	f.Add([]byte{0x81, 0x82, 1, 2, 3, 4, 'h' ^ 1, 'i' ^ 2}, true)
	f.Add([]byte{0x82, 127, 127, 255, 255, 255, 255, 255, 255, 255}, false)
	f.Fuzz(func(t *testing.T, wire []byte, server bool) {
		if len(wire) > 8192 {
			t.Skip()
		}
		role := protocol.Client
		if server {
			role = protocol.Server
		}
		run := func(chunk int) ([]protocol.Event, string) {
			s := receiver(t, protocol.ReceiverConfig{Role: role, MaxMessageSize: 1024})
			var events []protocol.Event
			for off := 0; off < len(wire); {
				n := min(chunk, len(wire)-off)
				s.Feed(wire[off : off+n])
				off += n
				for {
					e, ok, err := s.NextEvent()
					if err != nil {
						return events, err.Error()
					}
					if !ok {
						break
					}
					events = append(events, protocol.Event{Opcode: e.Opcode, Payload: bytes.Clone(e.Payload)})
				}
			}
			return events, ""
		}
		whole, wholeErr := run(max(1, len(wire)))
		split, splitErr := run(1)
		if wholeErr != splitErr || len(whole) != len(split) {
			t.Fatalf("chunking changed outcome: %v/%q vs %v/%q", whole, wholeErr, split, splitErr)
		}
		for i := range whole {
			if whole[i].Opcode != split[i].Opcode || !bytes.Equal(whole[i].Payload, split[i].Payload) {
				t.Fatal("chunking changed event")
			}
		}
	})
}
