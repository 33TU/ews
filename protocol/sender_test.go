package protocol_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/protocol"
	"github.com/klauspost/compress/flate"
)

func TestAppendOwnershipAndBatching(t *testing.T) {
	for _, role := range []protocol.Role{protocol.Server, protocol.Client} {
		s := sender(t, protocol.SenderConfig{Role: role})
		payload := []byte("caller payload")
		dst := make([]byte, 3, 256)
		copy(dst, "pre")
		dst, err := s.Append(dst, codec.Text, payload, false)
		if err != nil {
			t.Fatal(err)
		}
		first := bytes.Clone(dst)
		clear(payload)
		if !bytes.Equal(first, dst) {
			t.Fatal("output aliases payload")
		}
		dst, err = s.Append(dst, codec.Binary, []byte{255}, false)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, dst[:len(first)]) {
			t.Fatal("append changed existing output")
		}
		r := receiver(t, protocol.ReceiverConfig{Role: 1 - role})
		r.Feed(dst[3:])
		for _, want := range []protocol.Event{{Opcode: codec.Text, Payload: []byte("caller payload")}, {Opcode: codec.Binary, Payload: []byte{255}}} {
			e, ok, err := r.NextEvent()
			if err != nil || !ok || e.Opcode != want.Opcode || !bytes.Equal(e.Payload, want.Payload) {
				t.Fatalf("batch: %v, %v", e, err)
			}
		}
		saved := bytes.Clone(dst)
		if _, err := s.Append(nil, codec.Text, []byte("another buffer"), false); err != nil {
			t.Fatal(err)
		}
		s.Reset()
		if !bytes.Equal(dst, saved) {
			t.Fatal("sender retained or changed old output")
		}
	}
}

func TestAppendOverlappingPayload(t *testing.T) {
	for _, role := range []protocol.Role{protocol.Server, protocol.Client} {
		for _, offset := range []int{0, 8, 9, 15, 128} {
			s := sender(t, protocol.SenderConfig{Role: role})
			backing := make([]byte, 512)
			for i := range backing {
				backing[i] = byte(i)
			}
			prefix := bytes.Clone(backing[:8])
			payload := backing[offset : offset+80]
			want := bytes.Clone(payload)
			dst, err := s.Append(backing[:8], codec.Binary, payload, false)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(dst[:8], prefix) {
				t.Fatal("prefix changed")
			}
			_, p := readFrame(t, dst[8:])
			if !bytes.Equal(p, want) {
				t.Fatalf("overlap offset %d corrupted payload", offset)
			}
		}
	}
}

func TestAppendErrorsPreserveBufferAndHistory(t *testing.T) {
	s := sender(t, protocol.SenderConfig{Compression: &protocol.Compression{Level: flate.DefaultCompression, ContextTakeover: true}})
	r := receiver(t, protocol.ReceiverConfig{Role: protocol.Client, Compression: true, ContextTakeover: true})
	payload := bytes.Repeat([]byte("history"), 1000)
	for i := 0; i < 2; i++ {
		wire, err := s.Append(nil, codec.Text, payload, true)
		if err != nil {
			t.Fatal(err)
		}
		if e := receive(t, r, wire, 7); !bytes.Equal(e.Payload, payload) {
			t.Fatal("compression history changed")
		}
		backing := bytes.Repeat([]byte{0x55}, 256)
		original := bytes.Clone(backing)
		for _, tt := range []struct {
			op       codec.Opcode
			p        []byte
			compress bool
		}{
			{codec.Continuation, nil, false}, {codec.Text, []byte{255}, true},
			{codec.Ping, nil, true}, {codec.Pong, make([]byte, 126), false}, {codec.Close, []byte{1}, false},
		} {
			got, err := s.Append(backing[:10], tt.op, tt.p, tt.compress)
			if err == nil {
				t.Fatal("invalid append accepted")
			}
			if len(got) != 10 || cap(got) != cap(backing) || &got[0] != &backing[0] || !bytes.Equal(backing, original) {
				t.Fatal("error modified destination")
			}
		}
	}
	plain := sender(t, protocol.SenderConfig{})
	if _, err := plain.Append(nil, codec.Text, nil, true); !errors.Is(err, protocol.ErrProtocol) {
		t.Fatal("unnegotiated compression accepted")
	}
}

func TestCloseValidation(t *testing.T) {
	s := sender(t, protocol.SenderConfig{})
	prefix := []byte("prefix")
	for _, code := range []uint16{999, 1004, 1005, 1006, 1015, 1016, 2000, 2999, 5000, 65535} {
		if dst, err := s.AppendClose(prefix, code, ""); err == nil || !bytes.Equal(dst, prefix) {
			t.Fatalf("invalid code %d accepted", code)
		}
	}
	for _, tt := range []struct {
		code   uint16
		reason string
	}{{0, "reason"}, {1000, string([]byte{255})}, {1000, string(make([]byte, 124))}} {
		if dst, err := s.AppendClose(prefix, tt.code, tt.reason); err == nil || !bytes.Equal(dst, prefix) {
			t.Fatal("invalid close accepted")
		}
	}
	for _, code := range []uint16{0, 1000, 1001, 1002, 1003, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014, 3000, 4999} {
		s.Reset()
		if s.CloseSent() {
			t.Fatal("reset retained close state")
		}
		wire, err := s.AppendClose(nil, code, "")
		if err != nil {
			t.Fatalf("valid code %d: %v", code, err)
		}
		h, p := readFrame(t, wire)
		if h.Opcode() != codec.Close || !s.CloseSent() {
			t.Fatal("missing close")
		}
		if code == 0 {
			if len(p) != 0 {
				t.Fatal("nonempty close")
			}
		} else if binary.BigEndian.Uint16(p) != code {
			t.Fatal("wrong close code")
		}
		if _, err := s.Append(nil, codec.Text, nil, false); !errors.Is(err, protocol.ErrClosing) {
			t.Fatal("sent data after close")
		}
		if _, err := s.AppendClose(nil, 1000, ""); !errors.Is(err, protocol.ErrClosing) {
			t.Fatal("sent second close")
		}
		if _, err := s.Append(nil, codec.Pong, nil, false); err != nil {
			t.Fatal("cannot respond to ping while waiting for peer close")
		}
	}
}

func TestExplicitControlReplies(t *testing.T) {
	s := sender(t, protocol.SenderConfig{})
	r := receiver(t, protocol.ReceiverConfig{})
	dst, err := s.Append(nil, codec.Text, []byte("already pending"), false)
	if err != nil {
		t.Fatal(err)
	}
	pending := bytes.Clone(dst)
	wire := append(frame(t, 0x89, []byte("one"), true), frame(t, 0x89, []byte("two"), true)...)
	wire = append(wire, frame(t, 0x81, []byte("message"), true)...)
	r.Feed(wire)
	for i := 0; i < 3; i++ {
		e, ok, err := r.NextEvent()
		if err != nil || !ok {
			t.Fatalf("event: %v", err)
		}
		if i < 2 {
			if e.Opcode != codec.Ping {
				t.Fatal("missing ping")
			}
			dst, err = s.Append(dst, codec.Pong, e.Payload, false)
			if err != nil {
				t.Fatal(err)
			}
		} else if e.Opcode != codec.Text || string(e.Payload) != "message" {
			t.Fatal("receive blocked by pending output")
		}
	}
	if !bytes.Equal(dst[:len(pending)], pending) {
		t.Fatal("reply overwrote pending output")
	}
	peer := receiver(t, protocol.ReceiverConfig{Role: protocol.Client})
	peer.Feed(dst)
	for _, want := range []protocol.Event{{Opcode: codec.Text, Payload: []byte("already pending")}, {Opcode: codec.Pong, Payload: []byte("one")}, {Opcode: codec.Pong, Payload: []byte("two")}} {
		got, ok, err := peer.NextEvent()
		if err != nil || !ok || got.Opcode != want.Opcode || !bytes.Equal(got.Payload, want.Payload) {
			t.Fatalf("reply: %v, %v", got, err)
		}
	}
	// Receiving a close never changes the independent sender.
	closePayload := []byte{3, 232, 'b', 'y', 'e'}
	r.Feed(frame(t, 0x88, closePayload, true))
	e, ok, err := r.NextEvent()
	if err != nil || !ok || e.Opcode != codec.Close || !r.CloseReceived() || s.CloseSent() {
		t.Fatal("invalid directional close state")
	}
	reply, err := s.Append(nil, codec.Close, e.Payload, false)
	if err != nil {
		t.Fatal(err)
	}
	peer.Feed(reply)
	if e, ok, err := peer.NextEvent(); err != nil || !ok || e.Opcode != codec.Close || !bytes.Equal(e.Payload, closePayload) {
		t.Fatal("invalid explicit close reply")
	}
	r.Feed(frame(t, 0x81, nil, true))
	if _, ok, err := r.NextEvent(); ok || err != nil {
		t.Fatal("received data after close")
	}
	r.Reset()
	s.Reset()
	if r.CloseReceived() || s.CloseSent() {
		t.Fatal("reset retained close state")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, c := range []protocol.SenderConfig{{Role: 2}, {Compression: &protocol.Compression{Level: 100}}} {
		if _, err := protocol.NewSender(c); err == nil {
			t.Fatal("invalid sender configuration accepted")
		}
	}
	for _, c := range []protocol.ReceiverConfig{{Role: 2}, {MaxMessageSize: -1}, {ContextTakeover: true}} {
		if _, err := protocol.NewReceiver(c); err == nil {
			t.Fatal("invalid receiver configuration accepted")
		}
	}
}
