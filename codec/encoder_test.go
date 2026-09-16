package codec_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
)

func BenchmarkEncoderEncode(b *testing.B) {
	for _, size := range []int{0, 125, 126, 4096, 65536} {
		for _, masked := range []bool{false, true} {
			b.Run(fmt.Sprintf("size=%d/masked=%t", size, masked), func(b *testing.B) {
				payload := make([]byte, size)
				var key *[4]byte
				if masked {
					key = &[4]byte{1, 2, 3, 4}
				}
				var e codec.Encoder
				if err := e.Encode(true, codec.Binary, payload, key); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				if masked {
					b.SetBytes(int64(size))
				}
				for b.Loop() {
					if err := e.Encode(true, codec.Binary, payload, key); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// FuzzRoundTrip encodes arbitrary frames and decodes them back.
func FuzzRoundTrip(f *testing.F) {
	f.Add(uint8(codec.Text), true, true, []byte("hello"))
	f.Add(uint8(codec.Binary), false, false, make([]byte, 70000))
	f.Add(uint8(codec.Ping), true, true, []byte{})
	f.Fuzz(func(t *testing.T, op uint8, final, masked bool, payload []byte) {
		var enc codec.Encoder
		var key *[4]byte
		if masked {
			key = &[4]byte{1, 2, 3, 4}
		}
		err := enc.Encode(final, codec.Opcode(op), payload, key)
		valid := codec.Opcode(op) <= codec.Binary || codec.Opcode(op) >= codec.Close && codec.Opcode(op) <= codec.Pong
		control := codec.Opcode(op) >= codec.Close
		if control && (!final || len(payload) > 125 || codec.Opcode(op) == codec.Close && len(payload) == 1) {
			valid = false
		}
		if (err == nil) != valid {
			t.Fatalf("op %d final %t len %d: %v", op, final, len(payload), err)
		}
		if err != nil {
			return
		}
		var d codec.Decoder
		d.Feed(enc.HeaderBytes())
		d.Feed(enc.PayloadBytes())
		h, ok, err := d.NextHeader()
		if err != nil || !ok || h.Opcode() != codec.Opcode(op) || h.Final() != final || h.Masked() != masked || h.PayloadLen() != uint64(len(payload)) {
			t.Fatalf("header mismatch: %v %t %x", err, ok, h.Bytes())
		}
		got, done := d.Payload()
		if !done && len(payload) != 0 {
			t.Fatal("payload incomplete")
		}
		got = bytes.Clone(got)
		if masked {
			codec.Mask(got, *key, 0)
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch")
		}
	})
}

func TestScratchKeep(t *testing.T) {
	key := &[4]byte{1, 2, 3, 4}
	large, small := make([]byte, 2<<20), make([]byte, 100)
	var e codec.Encoder
	e.ScratchKeep = 1 << 20
	if err := e.Encode(true, codec.Binary, large, key); err != nil {
		t.Fatal(err)
	}
	if cap(e.PayloadBytes()) < len(large) {
		t.Fatal("large payload not masked into scratch")
	}
	if err := e.Encode(true, codec.Binary, small, key); err != nil {
		t.Fatal(err)
	}
	if got := cap(e.PayloadBytes()); got > e.ScratchKeep {
		t.Fatalf("scratch kept %d bytes after a small payload, want at most %d", got, e.ScratchKeep)
	}
	// Payloads within the bound keep reusing the scratch.
	before := cap(e.PayloadBytes())
	if err := e.Encode(true, codec.Binary, small, key); err != nil {
		t.Fatal(err)
	}
	if cap(e.PayloadBytes()) != before {
		t.Fatal("scratch reallocated for a payload that fit")
	}
	// Zero keeps whatever was needed.
	var unbounded codec.Encoder
	for _, p := range [][]byte{large, small} {
		if err := unbounded.Encode(true, codec.Binary, p, key); err != nil {
			t.Fatal(err)
		}
	}
	if cap(unbounded.PayloadBytes()) < len(large) {
		t.Fatal("zero ScratchKeep released the scratch")
	}
}
