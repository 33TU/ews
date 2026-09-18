package deflate_test

import (
	"bytes"
	"math/rand/v2"
	"testing"

	"github.com/33TU/ews/deflate"
	"github.com/klauspost/compress/flate"
)

func TestCompressorResetAndWindowAdd(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	message := bytes.Repeat([]byte("reset keeps the compressor usable "), 100)
	var cw, dw deflate.Window
	compress(t, c, bytes.Repeat([]byte("earlier "), 100)) // Leaves output and state behind.
	c.Reset()
	// A history both sides already share, added out of band, is used: random
	// bytes compress only when the encoder can reference them.
	shared := make([]byte, 4000)
	rand.NewChaCha8([32]byte{3}).Read(shared)
	cw.Add(shared)
	dw.Add(shared)
	withHistory := len(bytes.Clone(compressWith(t, c, shared, &cw)))
	c.Reset()
	cw.Reset()
	alone := len(bytes.Clone(compressWith(t, c, shared, &cw)))
	if withHistory >= alone {
		t.Fatalf("history not used: %d bytes with, %d without", withHistory, alone)
	}
	var d deflate.Decompressor
	cw.Reset()
	dw.Reset()
	compressed := bytes.Clone(compressWith(t, c, message, &cw))
	if got, err := d.Decompress(compressed, len(message), &dw); err != nil || !bytes.Equal(got, message) {
		t.Fatal(err)
	}
	d.Reset()
	if got, err := d.Decompress(compressed, len(message), nil); err != nil || !bytes.Equal(got, message) {
		t.Fatal("decompressor unusable after Reset:", err)
	}
}

func TestCompressorWindow(t *testing.T) {
	for _, bits := range []int{7, 16} {
		if _, err := deflate.NewCompressorWindow(bits); err != deflate.ErrInvalidWindow {
			t.Fatalf("bits %d: %v", bits, err)
		}
	}
	// A 256-byte encoder primed from a full 32 KB window uses only what it
	// can reach; the decoder with the full history still decodes it.
	c, err := deflate.NewCompressorWindow(8)
	if err != nil {
		t.Fatal(err)
	}
	var d deflate.Decompressor
	var cw, dw deflate.Window
	history := bytes.Repeat([]byte("history "), 8192) // 64 KB, beyond any window.
	cw.Add(history)
	dw.Add(history)
	message := bytes.Repeat([]byte("history "), 40)
	for i := range 3 {
		compressed := bytes.Clone(compressWith(t, c, message, &cw))
		if got, err := d.Decompress(compressed, len(message), &dw); err != nil || !bytes.Equal(got, message) {
			t.Fatalf("message %d: %v", i, err)
		}
	}
	// Chunked compression through the same reduced-window encoder.
	c.Reset()
	cw.Reset()
	dw.Reset()
	var wire []byte
	for i, final := range []bool{false, false, true} {
		part, err := c.CompressChunk(message, &cw, final)
		if err != nil {
			t.Fatalf("chunk %d: %v", i, err)
		}
		wire = append(wire, part...)
	}
	want := bytes.Repeat(message, 3)
	if got, err := d.Decompress(wire, len(want), &dw); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("chunked: %v", err)
	}
}
