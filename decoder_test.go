package ews

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

func TestDecoderSplitHeaders(t *testing.T) {
	headers := [][]byte{
		{0x82, 5}, {0x82, 126, 0, 126},
		{0x82, 127, 0, 0, 0, 0, 0, 1, 0, 0},
	}
	for _, base := range headers {
		for _, masked := range []bool{false, true} {
			wire := append([]byte(nil), base...)
			if masked {
				wire[1] |= 0x80
				wire = append(wire, 1, 2, 3, 4)
			}
			for split := 0; split < len(wire); split++ {
				var d Decoder
				d.Feed(wire[:split])
				if _, ok, err := d.Next(); ok || err != nil {
					t.Fatalf("header %x split %d: premature result %v, %v", wire, split, ok, err)
				}
				if !bytes.Equal(d.pending, wire[:split]) {
					t.Fatal("incomplete header consumed input")
				}
				d.Feed(wire[split:])
				h, ok, err := d.Next()
				if !ok || err != nil || !bytes.Equal(h.Bytes(), wire) {
					t.Fatalf("header %x split %d: got %x, %v, %v", wire, split, h.Bytes(), ok, err)
				}
				if p, done := d.Payload(); p != nil || done {
					t.Fatal("expected to wait for payload")
				}
			}
		}
	}
}

func TestDecoderStreamingPayload(t *testing.T) {
	var d Decoder
	first := []byte{0x82, 0x85, 1, 2, 3, 4, 0xaa, 0xbb}
	d.Feed(first)
	if p, done := d.Payload(); p != nil || !done {
		t.Fatal("payload before first header must be nil, true")
	}
	h, ok, err := d.Next()
	if !ok || err != nil || h.PayloadLen() != 5 {
		t.Fatalf("Next: %v, %v", ok, err)
	}
	if _, ok, err := d.Next(); ok || err != ErrPayloadPending {
		t.Fatal("Next must reject an unread payload")
	}
	p, done := d.Payload()
	if done || !bytes.Equal(p, first[6:]) || &p[0] != &first[6] {
		t.Fatal("payload must be borrowed and remain masked")
	}
	if p, done := d.Payload(); p != nil || done {
		t.Fatal("expected to wait for more payload")
	}
	d.Feed([]byte{0xcc, 0xdd, 0xee, 0x89, 0, 0x82, 1, 0xff})
	p, done = d.Payload()
	if !done || !bytes.Equal(p, []byte{0xcc, 0xdd, 0xee}) || cap(p) != len(p) {
		t.Fatal("payload crossed frame boundary or exposed append capacity")
	}
	if p, done := d.Payload(); p != nil || !done {
		t.Fatal("completed payload must return nil, true")
	}
	if h, ok, err := d.Next(); !ok || err != nil || h.Opcode() != Ping {
		t.Fatal("failed to read empty ping")
	}
	if _, ok, err := d.Next(); !ok || err != nil {
		t.Fatal("empty payload must allow immediate Next")
	}
	if p, done := d.Payload(); !done || !bytes.Equal(p, []byte{0xff}) {
		t.Fatal("incorrect last frame payload")
	}
	if !bytes.Equal(h.MaskKey(), []byte{1, 2, 3, 4}) {
		t.Fatal("returned header changed across decoder calls")
	}
}

func TestDecoderInvalidLengths(t *testing.T) {
	for _, wire := range [][]byte{
		{0x82, 126, 0, 125},
		{0x82, 127, 0, 0, 0, 0, 0, 0, 0xff, 0xff},
		{0x82, 127, 0x80, 0, 0, 0, 0, 0, 0, 0},
	} {
		var d Decoder
		d.Feed(wire)
		if _, ok, err := d.Next(); ok || err != ErrInvalidPayloadLength {
			t.Fatalf("invalid header %x: %v, %v", wire, ok, err)
		}
		if !bytes.Equal(d.pending, wire) || d.remaining != 0 {
			t.Fatal("invalid header changed decoder state")
		}
	}
}

func TestDecoderScratchAndReset(t *testing.T) {
	var d Decoder
	d.Feed([]byte{0x82})
	d.Feed([]byte{3, 'a'})
	if _, ok, err := d.Next(); !ok || err != nil {
		t.Fatal("failed to parse buffered header")
	}
	d.Feed([]byte{'b'}) // Compact pending bytes already backed by scratch.
	if p, done := d.Payload(); done || string(p) != "ab" {
		t.Fatal("scratch compaction corrupted payload")
	}
	scratchCap := cap(d.scratch)
	d.Reset()
	if d.pending != nil || d.remaining != 0 || len(d.scratch) != 0 || cap(d.scratch) != scratchCap {
		t.Fatal("Reset failed to clear state and retain storage")
	}
	d.Feed([]byte{0x82, 0})
	if _, ok, err := d.Next(); !ok || err != nil {
		t.Fatal("decoder unusable after Reset")
	}
}

func benchmarkFrame(size int, masked bool) []byte {
	header := []byte{0x82, 0}
	switch {
	case size <= 125:
		header[1] = byte(size)
	case size <= 65535:
		header[1] = 126
		header = binary.BigEndian.AppendUint16(header, uint16(size))
	default:
		header[1] = 127
		header = binary.BigEndian.AppendUint64(header, uint64(size))
	}
	if masked {
		header[1] |= 0x80
		header = append(header, 1, 2, 3, 4)
	}
	return append(header, make([]byte, size)...)
}

func BenchmarkDecoderComplete(b *testing.B) {
	for _, size := range []int{0, 125, 126, 4096, 65536} {
		for _, masked := range []bool{false, true} {
			b.Run(fmt.Sprintf("size=%d/masked=%t", size, masked), func(b *testing.B) {
				frame := benchmarkFrame(size, masked)
				var d Decoder
				b.ReportAllocs()
				for b.Loop() {
					d.Feed(frame)
					h, ok, err := d.Next()
					if !ok || err != nil || h.PayloadLen() != uint64(size) {
						b.Fatal("header:", ok, err)
					}
					if p, done := d.Payload(); !done || len(p) != size {
						b.Fatal("incomplete payload")
					}
				}
			})
		}
	}
}

func BenchmarkDecoderSplitHeader(b *testing.B) {
	frame := benchmarkFrame(4096, true)
	var d Decoder
	d.Feed(frame[:1])
	d.Feed(frame[1:])
	d.Reset()
	b.ReportAllocs()
	b.SetBytes(int64(len(frame)))
	for b.Loop() {
		d.Feed(frame[:1])
		if _, ok, err := d.Next(); ok || err != nil {
			b.Fatal("partial header:", ok, err)
		}
		d.Feed(frame[1:])
		if _, ok, err := d.Next(); !ok || err != nil {
			b.Fatal("header:", ok, err)
		}
		if p, done := d.Payload(); !done || len(p) != 4096 {
			b.Fatal("incomplete payload")
		}
	}
}

func BenchmarkDecoderStreamPayload(b *testing.B) {
	const size = 65536
	for _, chunkSize := range []int{64, 1024, 16384} {
		b.Run(fmt.Sprintf("chunk=%d", chunkSize), func(b *testing.B) {
			frame := benchmarkFrame(size, true)
			var d Decoder
			b.ReportAllocs()
			for b.Loop() {
				d.Feed(frame[:MaxHeaderSize])
				if _, ok, err := d.Next(); !ok || err != nil {
					b.Fatal("header:", ok, err)
				}
				for offset := MaxHeaderSize; offset < len(frame); offset += chunkSize {
					end := min(offset+chunkSize, len(frame))
					d.Feed(frame[offset:end])
					p, done := d.Payload()
					if len(p) != end-offset || done != (end == len(frame)) {
						b.Fatal("incorrect payload chunk")
					}
				}
			}
		})
	}
}

func BenchmarkDecoderBatch(b *testing.B) {
	const count = 64
	input := bytes.Repeat(benchmarkFrame(125, false), count)
	var d Decoder
	b.ReportAllocs()
	for b.Loop() {
		d.Feed(input)
		for range count {
			if _, ok, err := d.Next(); !ok || err != nil {
				b.Fatal("header:", ok, err)
			}
			if p, done := d.Payload(); !done || len(p) != 125 {
				b.Fatal("incorrect frame boundary")
			}
		}
		if _, ok, err := d.Next(); ok || err != nil {
			b.Fatal("unexpected trailing frame:", ok, err)
		}
	}
}
