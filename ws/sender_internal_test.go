package ws

import (
	"bytes"
	"testing"

	"github.com/33TU/ews/codec"
)

func TestSender(t *testing.T) {
	var s sender
	s.Init(Client)
	wire := encode(t, &s, codec.Text, []byte("hi"))
	if wire[1]&0x80 == 0 || len(wire) != 2+4+2 {
		t.Fatalf("client frame not masked: %x", wire)
	}
	if _, _, err := s.Encode(codec.Ping, make([]byte, 126)); err != ErrProtocol {
		t.Fatal("oversized ping accepted")
	}
	if _, _, err := s.Encode(codec.Continuation, nil); err != ErrProtocol {
		t.Fatal("continuation accepted")
	}
	if h, _, err := s.EncodeCompressed(codec.Binary, []byte{0}); err != nil || h[0]&0x40 == 0 {
		t.Fatal("compressed frame without RSV1")
	}
	if _, _, err := s.EncodeCompressed(codec.Ping, nil); err != ErrProtocol {
		t.Fatal("compressed control frame accepted")
	}
	if h, _, err := s.EncodeFragment(codec.Text, false, []byte("a"), true); err != nil || h[0] != 0x41 {
		t.Fatalf("first compressed fragment header %x, %v", h, err)
	}
	if h, _, err := s.EncodeFragment(codec.Continuation, true, nil, true); err != nil || h[0] != 0x80 {
		t.Fatalf("final continuation header %x, %v", h, err)
	}
	if _, _, err := s.EncodeFragment(codec.Ping, false, nil, false); err != ErrProtocol {
		t.Fatal("fragmented control frame accepted")
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
	var r receiver
	r.Init(Server, false)
	events, err := drive(t, &r, append(bytes.Clone(h), b...), 1)
	if err != nil || len(events) != 1 {
		t.Fatalf("events %+v, err %v", events, err)
	}
	if code, reason := parseClose(events[0].payload); code != 1001 || string(reason) != "bye" {
		t.Fatalf("code %d reason %q", code, reason)
	}
	if _, _, err := s.Encode(codec.Text, nil); err != ErrClosing {
		t.Fatal("data after close accepted")
	}
	if _, _, err := s.Encode(codec.Pong, nil); err != nil {
		t.Fatal("pong after close rejected")
	}
	s.Init(Server)
	if _, _, err := s.Encode(codec.Text, nil); err != nil || s.CloseSent() {
		t.Fatal("Init did not clear close state")
	}
}
