package codec

import (
	"bytes"
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
				if _, ok, err := d.NextHeader(); ok || err != nil {
					t.Fatalf("header %x split %d: premature result %v, %v", wire, split, ok, err)
				}
				if !bytes.Equal(d.pending, wire[:split]) {
					t.Fatal("incomplete header consumed input")
				}
				d.Feed(wire[split:])
				h, ok, err := d.NextHeader()
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

func TestDecoderInvalidLengths(t *testing.T) {
	for _, wire := range [][]byte{
		{0x82, 126, 0, 125},
		{0x82, 127, 0, 0, 0, 0, 0, 0, 0xff, 0xff},
		{0x82, 127, 0x80, 0, 0, 0, 0, 0, 0, 0},
	} {
		var d Decoder
		d.Feed(wire)
		if _, ok, err := d.NextHeader(); ok || err != ErrInvalidPayloadLength {
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
	if _, ok, err := d.NextHeader(); !ok || err != nil {
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
	if _, ok, err := d.NextHeader(); !ok || err != nil {
		t.Fatal("decoder unusable after Reset")
	}
}
