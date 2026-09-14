package proto_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/internal/proto"
)

func TestFragmentsAndControls(t *testing.T) {
	for _, masked := range []bool{false, true} {
		t.Run(fmt.Sprintf("masked=%t", masked), func(t *testing.T) {
			var r proto.Receiver
			r.Init(proto.Client, false)
			if masked {
				r.Init(proto.Server, false)
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
		{"rsv1 control", frame(t, 0xc9, nil, false), 1002},
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
		{"nonminimal length", []byte{0x82, 126, 0, 1}, 1002},
		{"invalid length", []byte{0x82, 127, 128, 0, 0, 0, 0, 0, 0, 0}, 1002},
	}
	for _, tt := range tests {
		for _, chunk := range []int{1, 1 << 16} {
			t.Run(fmt.Sprintf("%s/chunk=%d", tt.name, chunk), func(t *testing.T) {
				var r proto.Receiver
				r.Init(proto.Client, false)
				_, err := drive(t, &r, tt.wire, chunk)
				var pe *proto.Error
				if !errors.As(err, &pe) || pe.Code != tt.code {
					t.Fatalf("expected code %d, got %v", tt.code, err)
				}
				r.Feed(frame(t, 0x81, nil, false))
				if _, again := r.Next(); again != err {
					t.Fatal("failure was not terminal")
				}
				r.Init(proto.Client, false)
				if _, err := drive(t, &r, frame(t, 0x81, []byte("ok"), false), chunk); err != nil {
					t.Fatalf("Init did not recover: %v", err)
				}
			})
		}
	}
}

func TestCompressedFlag(t *testing.T) {
	var r proto.Receiver
	r.Init(proto.Client, true)
	wire := append(frame(t, 0x41, []byte{1}, false), frame(t, 0x80, []byte{2}, false)...) // RSV1 text, continuation.
	wire = append(wire, frame(t, 0x82, []byte{3}, false)...)                              // Plain binary.
	events, err := drive(t, &r, wire, len(wire))
	if err != nil || len(events) != 2 {
		t.Fatalf("%+v %v", events, err)
	}
	r.Init(proto.Client, true)
	r.Feed(frame(t, 0x41, []byte{1}, false))
	if kind, err := r.Next(); kind != proto.DataFrame || err != nil || !r.MessageCompressed() {
		t.Fatal("compressed flag not set")
	}
	r.Init(proto.Client, true)
	for _, bad := range [][]byte{
		frame(t, 0xc9, nil, false), // RSV1 ping.
		append(frame(t, 0x01, nil, false), frame(t, 0xc0, nil, false)...), // RSV1 continuation.
		append(frame(t, 0x41, nil, false), frame(t, 0xc0, nil, false)...), // RSV1 continuation of compressed.
	} {
		r.Init(proto.Client, true)
		if _, err := drive(t, &r, bad, len(bad)); err == nil {
			t.Fatalf("accepted %x", bad)
		}
	}
}

func TestCloseReceived(t *testing.T) {
	var r proto.Receiver
	r.Init(proto.Client, false)
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

// FuzzReceiver drives the receiver with arbitrary input in arbitrary chunks.
// It must never panic, and after a failure every call must keep failing.
func FuzzReceiver(f *testing.F) {
	f.Add([]byte{0x81, 0x85, 1, 2, 3, 4, 'h' ^ 1, 'e' ^ 2, 'l' ^ 3, 'l' ^ 4, 'o' ^ 1}, uint8(2), true)
	f.Add([]byte{0x88, 0x82, 0, 0, 0, 0, 0x03, 0xe8}, uint8(1), true)
	f.Add([]byte{0xc1, 0x02, 0xff, 0xff}, uint8(1), false)
	f.Fuzz(func(t *testing.T, wire []byte, split uint8, compression bool) {
		var r proto.Receiver
		r.Init(proto.Server, compression)
		step := int(split)%9 + 1
		var failed error
		for len(wire) > 0 {
			n := min(step, len(wire))
			r.Feed(wire[:n])
			wire = wire[n:]
			for {
				kind, err := r.Next()
				if err != nil {
					if failed != nil && err != failed {
						t.Fatalf("error changed after failure: %v then %v", failed, err)
					}
					failed = err
					if err != codec.ErrPayloadPending {
						break
					}
				}
				if kind == proto.NeedInput {
					break
				}
				if kind == proto.DataFrame {
					for {
						chunk, done, err := r.Payload()
						if err != nil {
							t.Fatal(err)
						}
						if uint64(len(chunk)) > r.Header().PayloadLen() {
							t.Fatal("chunk exceeds frame")
						}
						if done || len(chunk) == 0 {
							break
						}
					}
				}
				if r.CloseReceived() {
					return
				}
			}
			if failed != nil {
				// Feeding after a failure is ignored and Next keeps failing.
				r.Feed([]byte{0x81, 0x80, 0, 0, 0, 0})
				if _, err := r.Next(); err != failed {
					t.Fatalf("failure not sticky: %v", err)
				}
				return
			}
			r.Preserve()
		}
	})
}
