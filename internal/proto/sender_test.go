package proto_test

import (
	"bytes"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/internal/proto"
)

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
	if h, _, err := s.EncodeCompressed(codec.Binary, []byte{0}); err != nil || h[0]&0x40 == 0 {
		t.Fatal("compressed frame without RSV1")
	}
	if _, _, err := s.EncodeCompressed(codec.Ping, nil); err != proto.ErrProtocol {
		t.Fatal("compressed control frame accepted")
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
	r.Init(proto.Server, false)
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
