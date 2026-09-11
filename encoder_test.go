package ews

import (
	"bytes"
	"fmt"
	"testing"
)

func TestEncoderWireFormat(t *testing.T) {
	for _, tt := range []struct {
		name    string
		final   bool
		opcode  Opcode
		payload []byte
		key     []byte
		want    []byte
	}{
		{"empty", true, Text, nil, nil, []byte{0x81, 0}},
		{"text", true, Text, []byte("Hello"), nil, []byte{0x81, 5, 'H', 'e', 'l', 'l', 'o'}},
		{"masked", true, Text, []byte("Hello"), []byte{0x37, 0xfa, 0x21, 0x3d}, []byte{0x81, 0x85, 0x37, 0xfa, 0x21, 0x3d, 0x7f, 0x9f, 0x4d, 0x51, 0x58}},
		{"fragment", false, Binary, []byte{42}, nil, []byte{2, 1, 42}},
		{"continuation", true, Continuation, []byte{42}, nil, []byte{0x80, 1, 42}},
		{"ping", true, Ping, nil, nil, []byte{0x89, 0}},
		{"pong", true, Pong, nil, nil, []byte{0x8a, 0}},
		{"close", true, Close, nil, nil, []byte{0x88, 0}},
		{"zero_key", true, Binary, []byte{42}, []byte{0, 0, 0, 0}, []byte{0x82, 0x81, 0, 0, 0, 0, 42}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var e Encoder
			payload := bytes.Clone(tt.payload)
			key := bytes.Clone(tt.key)
			if err := e.Encode(tt.final, tt.opcode, payload, key); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoderWire(&e), tt.want) {
				t.Fatalf("got %x, want %x", encoderWire(&e), tt.want)
			}
			if !bytes.Equal(payload, tt.payload) || !bytes.Equal(key, tt.key) {
				t.Fatal("Encode modified input")
			}
		})
	}
}

func TestEncoderLengthBoundaries(t *testing.T) {
	for _, size := range []int{0, 125, 126, 65535, 65536} {
		for _, masked := range []bool{false, true} {
			t.Run(fmt.Sprintf("size=%d/masked=%t", size, masked), func(t *testing.T) {
				var e Encoder
				payload := bytes.Repeat([]byte{0xab}, size)
				var key []byte
				if masked {
					key = []byte{1, 2, 3, 4}
				}
				if err := e.Encode(true, Binary, payload, key); err != nil {
					t.Fatal(err)
				}
				var d Decoder
				d.Feed(encoderWire(&e))
				h, ok, err := d.NextHeader()
				if !ok || err != nil || h.PayloadLen() != uint64(size) || h.Masked() != masked {
					t.Fatalf("invalid header: %+v, %v, %v", h, ok, err)
				}
				p, done := d.Payload()
				if !done || len(p) != size || len(d.pending) != 0 {
					t.Fatal("incorrect encoded frame size")
				}
				for i, v := range p {
					want := payload[i]
					if masked {
						want ^= key[i&3]
					}
					if v != want {
						t.Fatalf("incorrect payload byte at %d", i)
					}
				}
			})
		}
	}
}

func TestEncoderReuseAndReset(t *testing.T) {
	var e Encoder
	if len(e.HeaderBytes()) != 0 || e.PayloadBytes() != nil {
		t.Fatal("unexpected zero-value output")
	}
	key := []byte{1, 2, 3, 4}
	payload := []byte("Hello")
	if err := e.Encode(true, Text, payload, key); err != nil {
		t.Fatal(err)
	}
	storage := e.PayloadBytes()
	if &storage[0] == &payload[0] {
		t.Fatal("masked output must use separate storage")
	}
	if err := e.Encode(false, Binary, payload, nil); err != nil {
		t.Fatal(err)
	}
	if &e.PayloadBytes()[0] != &payload[0] || !bytes.Equal(e.HeaderBytes(), []byte{2, 5}) {
		t.Fatal("unmasked frame must replace header and borrow payload")
	}
	if err := e.Encode(true, Text, payload, key); err != nil {
		t.Fatal(err)
	}
	if &e.PayloadBytes()[0] != &storage[0] {
		t.Fatal("masked encoding did not reuse scratch")
	}
	e.Reset()
	if len(e.HeaderBytes()) != 0 || e.PayloadBytes() != nil || len(e.scratch) != 0 || cap(e.scratch) != cap(storage) {
		t.Fatal("Reset must clear the frame and retain scratch capacity")
	}
	if err := e.Encode(true, Text, payload[:2], key); err != nil {
		t.Fatal(err)
	}
	if &e.PayloadBytes()[0] != &storage[0] || len(e.PayloadBytes()) != 2 {
		t.Fatal("encoding after Reset failed to reuse storage")
	}
	if err := e.Encode(true, Ping, nil, nil); err != nil {
		t.Fatal(err)
	}
	if e.PayloadBytes() != nil || !bytes.Equal(e.HeaderBytes(), []byte{0x89, 0}) {
		t.Fatal("empty frame retained previous output")
	}
}

func TestEncoderInvalidFrames(t *testing.T) {
	for _, tt := range []struct {
		final   bool
		opcode  Opcode
		payload []byte
		want    error
	}{
		{true, 3, nil, ErrInvalidOpcode},
		{true, 0x81, nil, ErrInvalidOpcode},
		{false, Ping, nil, ErrInvalidControlFrame},
		{false, Pong, nil, ErrInvalidControlFrame},
		{false, Close, nil, ErrInvalidControlFrame},
		{true, Ping, make([]byte, 126), ErrInvalidControlFrame},
		{true, Close, []byte{0}, ErrInvalidControlFrame},
	} {
		var e Encoder
		if err := e.Encode(true, Text, []byte("saved"), []byte{1, 2, 3, 4}); err != nil {
			t.Fatal(err)
		}
		before := encoderWire(&e)
		if err := e.Encode(tt.final, tt.opcode, tt.payload, nil); err != tt.want {
			t.Fatalf("got %v, want %v", err, tt.want)
		}
		if !bytes.Equal(encoderWire(&e), before) {
			t.Fatal("error changed output")
		}
	}
}

func TestEncoderInvalidMaskKey(t *testing.T) {
	for _, key := range [][]byte{make([]byte, 0), make([]byte, 3), make([]byte, 3, 4), make([]byte, 5)} {
		t.Run(fmt.Sprintf("len=%d/cap=%d", len(key), cap(key)), func(t *testing.T) {
			var e Encoder
			if err := e.Encode(true, Text, []byte("saved"), []byte{1, 2, 3, 4}); err != nil {
				t.Fatal(err)
			}
			before := encoderWire(&e)
			defer func() {
				if recover() == nil {
					t.Error("invalid key did not panic")
				}
				if !bytes.Equal(encoderWire(&e), before) {
					t.Error("panic changed output")
				}
			}()
			_ = e.Encode(true, Text, nil, key)
		})
	}
}

func BenchmarkEncoderEncode(b *testing.B) {
	for _, size := range []int{0, 125, 126, 4096, 65536} {
		for _, masked := range []bool{false, true} {
			b.Run(fmt.Sprintf("size=%d/masked=%t", size, masked), func(b *testing.B) {
				payload := make([]byte, size)
				var key []byte
				if masked {
					key = []byte{1, 2, 3, 4}
				}
				var e Encoder
				if err := e.Encode(true, Binary, payload, key); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				if masked {
					b.SetBytes(int64(size))
				}
				for b.Loop() {
					if err := e.Encode(true, Binary, payload, key); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func encoderWire(e *Encoder) []byte {
	return append(bytes.Clone(e.HeaderBytes()), e.PayloadBytes()...)
}
