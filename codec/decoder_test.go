package codec_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
)

func TestDecoderPayloadN(t *testing.T) {
	var d codec.Decoder
	if p, done := d.PayloadN(1); p != nil || !done {
		t.Fatal("expected completed payload before a header")
	}
	wire := []byte{0x82, 5, 'a', 'b', 'c'}
	d.Feed(wire)
	if _, ok, err := d.NextHeader(); !ok || err != nil {
		t.Fatalf("header: %v, %v", ok, err)
	}
	for _, n := range []int{-1, 0} {
		if p, done := d.PayloadN(n); p != nil || done {
			t.Fatalf("limit %d: got %q, %v", n, p, done)
		}
	}
	if p, done := d.PayloadN(2); string(p) != "ab" || done || cap(p) != 2 || &p[0] != &wire[2] {
		t.Fatal("expected two borrowed bytes")
	}
	if p, done := d.PayloadN(99); string(p) != "c" || done {
		t.Fatal("expected remaining available byte")
	}
	if p, done := d.PayloadN(1); p != nil || done {
		t.Fatal("expected to wait for input")
	}
	d.Feed([]byte{'d', 'e', 0x82, 0})
	if p, done := d.PayloadN(99); string(p) != "de" || !done {
		t.Fatal("expected end of payload without consuming next header")
	}
	if _, ok, err := d.NextHeader(); !ok || err != nil {
		t.Fatalf("next header: %v, %v", ok, err)
	}
	if p, done := d.PayloadN(0); p != nil || !done {
		t.Fatal("expected completed empty payload")
	}
}

func TestDecoderPreserve(t *testing.T) {
	var d codec.Decoder
	var b [1]byte
	wire := []byte{0x82, 126, 0, 126}
	for _, v := range wire {
		d.Preserve()
		b[0] = v
		d.Feed(b[:])
	}
	d.Preserve()
	d.Preserve()
	b[0] = 0
	h, ok, err := d.NextHeader()
	if !ok || err != nil || !bytes.Equal(h.Bytes(), wire) {
		t.Fatalf("header = %x, %v, %v", h.Bytes(), ok, err)
	}
	b[0] = 42
	d.Feed(b[:])
	d.Preserve()
	b[0] = 0
	p, done := d.Payload()
	if !bytes.Equal(p, []byte{42}) || done {
		t.Fatalf("payload = %x, done = %v", p, done)
	}
}

func TestDecoderStreamingPayload(t *testing.T) {
	var d codec.Decoder
	first := []byte{0x82, 0x85, 1, 2, 3, 4, 0xaa, 0xbb}
	d.Feed(first)
	if p, done := d.Payload(); p != nil || !done {
		t.Fatal("payload before first header must be nil, true")
	}
	h, ok, err := d.NextHeader()
	if !ok || err != nil || h.PayloadLen() != 5 {
		t.Fatalf("NextHeader: %v, %v", ok, err)
	}
	if _, ok, err := d.NextHeader(); ok || err != codec.ErrPayloadPending {
		t.Fatal("NextHeader must reject an unread payload")
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
	if h, ok, err := d.NextHeader(); !ok || err != nil || h.Opcode() != codec.Ping {
		t.Fatal("failed to read empty ping")
	}
	if _, ok, err := d.NextHeader(); !ok || err != nil {
		t.Fatal("empty payload must allow immediate NextHeader")
	}
	if p, done := d.Payload(); !done || !bytes.Equal(p, []byte{0xff}) {
		t.Fatal("incorrect last frame payload")
	}
	if !bytes.Equal(h.MaskKey(), []byte{1, 2, 3, 4}) {
		t.Fatal("returned header changed across decoder calls")
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
				var d codec.Decoder
				b.ReportAllocs()
				for b.Loop() {
					d.Feed(frame)
					h, ok, err := d.NextHeader()
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
	var d codec.Decoder
	d.Feed(frame[:1])
	d.Feed(frame[1:])
	d.Reset()
	b.ReportAllocs()
	b.SetBytes(int64(len(frame)))
	for b.Loop() {
		d.Feed(frame[:1])
		if _, ok, err := d.NextHeader(); ok || err != nil {
			b.Fatal("partial header:", ok, err)
		}
		d.Feed(frame[1:])
		if _, ok, err := d.NextHeader(); !ok || err != nil {
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
			var d codec.Decoder
			b.ReportAllocs()
			for b.Loop() {
				d.Feed(frame[:codec.MaxHeaderSize])
				if _, ok, err := d.NextHeader(); !ok || err != nil {
					b.Fatal("header:", ok, err)
				}
				for offset := codec.MaxHeaderSize; offset < len(frame); offset += chunkSize {
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
	var d codec.Decoder
	b.ReportAllocs()
	for b.Loop() {
		d.Feed(input)
		for range count {
			if _, ok, err := d.NextHeader(); !ok || err != nil {
				b.Fatal("header:", ok, err)
			}
			if p, done := d.Payload(); !done || len(p) != 125 {
				b.Fatal("incorrect frame boundary")
			}
		}
		if _, ok, err := d.NextHeader(); ok || err != nil {
			b.Fatal("unexpected trailing frame:", ok, err)
		}
	}
}

// FuzzDecoder feeds arbitrary bytes in arbitrary splits: the decoder must
// never panic, must never hand out more payload than a header announced, and
// must reject nonminimal lengths.
func FuzzDecoder(f *testing.F) {
	f.Add([]byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'}, uint8(3))
	f.Add([]byte{0x82, 0xfe, 0x00, 0x7d, 1, 2, 3}, uint8(1))
	f.Add([]byte{0x82, 0xff, 0, 0, 0, 0, 0, 0, 0, 5, 1, 2, 3, 4, 5}, uint8(2))
	f.Fuzz(func(t *testing.T, wire []byte, split uint8) {
		var d codec.Decoder
		step := int(split)%7 + 1
		var remaining uint64
		for len(wire) > 0 {
			n := min(step, len(wire))
			d.Feed(wire[:n])
			wire = wire[n:]
			for {
				if remaining == 0 {
					h, ok, err := d.NextHeader()
					if err != nil {
						if err != codec.ErrInvalidPayloadLength {
							t.Fatal(err)
						}
						return
					}
					if !ok {
						break
					}
					remaining = h.PayloadLen()
					if h.Len() < 2 || h.Len() > codec.MaxHeaderSize {
						t.Fatal("header length", h.Len())
					}
				}
				chunk, done := d.Payload()
				if uint64(len(chunk)) > remaining {
					t.Fatalf("payload %d exceeds remaining %d", len(chunk), remaining)
				}
				remaining -= uint64(len(chunk))
				if done && remaining != 0 {
					t.Fatal("done with payload remaining")
				}
				if !done {
					break
				}
			}
			d.Preserve()
		}
	})
}
