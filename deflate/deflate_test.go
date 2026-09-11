package deflate_test

import (
	"bytes"
	stdflate "compress/flate"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
	"github.com/klauspost/compress/flate"
)

var tail = []byte{0, 0, 0xff, 0xff, 1, 0, 0, 0xff, 0xff}

func standardCompress(t *testing.T, payload, dict []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	w, err := stdflate.NewWriterDict(&output, stdflate.DefaultCompression, dict)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	result := bytes.Clone(output.Bytes()[:output.Len()-4])
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRoundTrip(t *testing.T) {
	noise := make([]byte, 65536)
	_, _ = rand.New(rand.NewSource(1)).Read(noise)
	messages := [][]byte{nil, []byte("Hello"), bytes.Repeat([]byte("hello world"), 8192), noise}
	for _, level := range []int{flate.NoCompression, flate.BestSpeed, flate.DefaultCompression, flate.BestCompression, flate.HuffmanOnly} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			c, err := deflate.NewCompressor(level)
			if err != nil {
				t.Fatal(err)
			}
			var d deflate.Decompressor
			for round := 0; round < 2; round++ {
				for _, message := range messages {
					input := bytes.Clone(message)
					compressed, err := c.Compress(input)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(input, message) {
						t.Fatal("compression changed input")
					}
					decoded, err := d.Decompress(compressed, len(message))
					if err != nil || !bytes.Equal(decoded, message) {
						t.Fatalf("round trip size %d: %v", len(message), err)
					}
					r := stdflate.NewReader(bytes.NewReader(append(bytes.Clone(compressed), tail...)))
					decoded, err = io.ReadAll(r)
					_ = r.Close()
					if err != nil || !bytes.Equal(decoded, message) {
						t.Fatalf("standard reader size %d: %v", len(message), err)
					}
					decoded, err = d.Decompress(standardCompress(t, message, nil), len(message))
					if err != nil || !bytes.Equal(decoded, message) {
						t.Fatalf("standard writer size %d: %v", len(message), err)
					}
				}
			}
		})
	}
}

func TestRFCExamples(t *testing.T) {
	var d deflate.Decompressor
	for _, payload := range [][]byte{
		{0xf2, 0x48, 0xcd, 0xc9, 0xc9, 0x07, 0x00},
		{0xf3, 0x48, 0xcd, 0xc9, 0xc9, 0x07, 0x00, 0x00},
		{0xf2, 0x48, 0x05, 0x00, 0x00, 0x00, 0xff, 0xff, 0xca, 0xc9, 0xc9, 0x07, 0x00},
	} {
		got, err := d.Decompress(payload, 5)
		if err != nil || string(got) != "Hello" {
			t.Fatalf("payload %x: got %q, %v", payload, got, err)
		}
	}
}

func TestFreshContext(t *testing.T) {
	c, err := deflate.NewCompressor(flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	message := bytes.Repeat([]byte("history should not cross message boundaries"), 256)
	first, err := c.Compress(message)
	if err != nil {
		t.Fatal(err)
	}
	first = bytes.Clone(first)
	if _, err := c.Compress([]byte("different message")); err != nil {
		t.Fatal(err)
	}
	second, err := c.Compress(message)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("compression retained message history")
	}
	var d deflate.Decompressor
	if _, err := d.Decompress(first, len(message)); err != nil {
		t.Fatal(err)
	}
	withHistory := standardCompress(t, message, message)
	if _, err := d.Decompress(withHistory, len(message)); err == nil {
		t.Fatal("decompression accepted a reference to the previous message")
	}
	if got, err := d.Decompress(first, len(message)); err != nil || !bytes.Equal(got, message) {
		t.Fatal("decompressor did not recover after error")
	}
}

func TestFinalBlocksWithinMessage(t *testing.T) {
	message := bytes.Repeat([]byte("dictionary within one message"), 256)
	var compressed bytes.Buffer
	for _, dict := range [][]byte{nil, message} {
		w, err := stdflate.NewWriterDict(&compressed, stdflate.BestCompression, dict)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(message); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	compressed.WriteByte(0) // Header of the stripped sync-flush block.
	var d deflate.Decompressor
	want := bytes.Repeat(message, 2)
	got, err := d.Decompress(compressed.Bytes(), len(want))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("final blocks lost message history: %v", err)
	}
}

func TestLimitsAndInvalidInput(t *testing.T) {
	var d deflate.Decompressor
	compressed := standardCompress(t, []byte("Hello"), nil)
	for _, limit := range []int{0, 4} {
		if got, err := d.Decompress(compressed, limit); got != nil || !errors.Is(err, deflate.ErrMessageTooLarge) {
			t.Fatalf("limit %d: got %x, %v", limit, got, err)
		}
	}
	if _, err := d.Decompress(compressed, -1); !errors.Is(err, deflate.ErrInvalidLimit) {
		t.Fatal("negative limit accepted")
	}
	for _, input := range [][]byte{nil, {0x06}, {0x00, 0x01}} {
		if _, err := d.Decompress(input, 1024); err == nil {
			t.Fatalf("invalid input %x accepted", input)
		}
	}
	if got, err := d.Decompress(compressed, 5); err != nil || string(got) != "Hello" {
		t.Fatal("decompressor did not recover")
	}
	if got, err := d.Decompress([]byte{0}, 0); err != nil || len(got) != 0 {
		t.Fatalf("empty message: %x, %v", got, err)
	}
	if _, err := deflate.NewCompressor(100); err == nil {
		t.Fatal("invalid compression level accepted")
	}
}

func TestCompressedFragments(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	message := bytes.Repeat([]byte("message split across compressed frames"), 100)
	compressed, err := c.Compress(message)
	if err != nil {
		t.Fatal(err)
	}
	var enc codec.Encoder
	var wire []byte
	for i := 0; i < len(compressed); i++ {
		opcode := codec.Continuation
		if i == 0 {
			opcode = codec.Text
		}
		key := [4]byte{byte(i), 2, 3, 4}
		if err := enc.EncodeCompressed(i == len(compressed)-1, opcode, compressed[i:i+1], &key); err != nil {
			t.Fatal(err)
		}
		wire = append(wire, enc.HeaderBytes()...)
		wire = append(wire, enc.PayloadBytes()...)
		if i == 0 {
			if err := enc.Encode(true, codec.Ping, nil, &[4]byte{}); err != nil {
				t.Fatal(err)
			}
			wire = append(wire, enc.HeaderBytes()...)
		}
	}
	var dec codec.Decoder
	dec.Feed(wire)
	var assembled []byte
	first := true
	for {
		h, ok, err := dec.NextHeader()
		if err != nil || !ok {
			t.Fatalf("header: %v, %v", ok, err)
		}
		p, done := dec.Payload()
		if !done {
			t.Fatal("incomplete frame")
		}
		if h.Opcode() == codec.Ping {
			if h.RSV1() {
				t.Fatal("control frame has RSV1 set")
			}
			continue
		}
		if h.RSV1() != first {
			t.Fatal("RSV1 must only be set on the first fragment")
		}
		first = false
		codec.Mask(p, [4]byte(h.MaskKey()), 0)
		assembled = append(assembled, p...)
		if h.Final() {
			break
		}
	}
	var d deflate.Decompressor
	got, err := d.Decompress(assembled, len(message))
	if err != nil || !bytes.Equal(got, message) {
		t.Fatalf("fragmented round trip: %v", err)
	}
	before := bytes.Clone(enc.HeaderBytes())
	if err := enc.EncodeCompressed(true, codec.Ping, nil, nil); !errors.Is(err, codec.ErrInvalidCompressedFrame) {
		t.Fatal("compressed control frame accepted")
	}
	if !bytes.Equal(before, enc.HeaderBytes()) {
		t.Fatal("invalid compressed frame changed encoder state")
	}
}

func BenchmarkCompression(b *testing.B) {
	for _, takeover := range []bool{false, true} {
		b.Run(fmt.Sprintf("takeover=%t", takeover), func(b *testing.B) {
			payload := bytes.Repeat([]byte(`{"type":"update","value":12345}`), 128)
			c, err := deflate.NewCompressor(flate.BestSpeed)
			if err != nil {
				b.Fatal(err)
			}
			c.ContextTakeover = takeover
			compressed, err := c.Compress(payload)
			if err != nil {
				b.Fatal(err)
			}
			compressed = bytes.Clone(compressed)
			d := deflate.Decompressor{ContextTakeover: takeover}
			if _, err := d.Decompress(compressed, len(payload)); err != nil {
				b.Fatal(err)
			}
			compressed, err = c.Compress(payload)
			if err != nil {
				b.Fatal(err)
			}
			compressed = bytes.Clone(compressed)
			if _, err := d.Decompress(compressed, len(payload)); err != nil {
				b.Fatal(err)
			}
			b.Run("compress", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				for b.Loop() {
					if _, err := c.Compress(payload); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("decompress", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				for b.Loop() {
					if _, err := d.Decompress(compressed, len(payload)); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func TestContextTakeover(t *testing.T) {
	noise := make([]byte, 65536)
	_, _ = rand.New(rand.NewSource(2)).Read(noise)
	messages := [][]byte{noise, nil, nil, []byte("short"), noise[40000:50000], noise[50000:], bytes.Repeat([]byte("repeat"), 20000)}
	for _, level := range []int{flate.NoCompression, flate.BestSpeed, flate.DefaultCompression, flate.BestCompression, flate.HuffmanOnly} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			c, err := deflate.NewCompressor(level)
			if err != nil {
				t.Fatal(err)
			}
			c.ContextTakeover = true
			d := deflate.Decompressor{ContextTakeover: true}
			peer := deflate.Decompressor{ContextTakeover: true}
			var wire bytes.Buffer
			w, err := stdflate.NewWriter(&wire, stdflate.DefaultCompression)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			var history []byte
			for round := 0; round < 2; round++ {
				c.Reset()
				d.Reset()
				peer.Reset()
				w.Reset(&wire)
				history = history[:0]
				for i, message := range messages {
					compressed, err := c.Compress(message)
					if err != nil {
						t.Fatal(err)
					}
					got, err := d.Decompress(compressed, len(message))
					if err != nil || !bytes.Equal(got, message) {
						t.Fatalf("round %d message %d: %v", round, i, err)
					}
					// Returned output may be modified without corrupting retained history.
					clear(got)
					r := stdflate.NewReaderDict(bytes.NewReader(append(bytes.Clone(compressed), tail...)), history)
					got, err = io.ReadAll(r)
					r.Close()
					if err != nil || !bytes.Equal(got, message) {
						t.Fatalf("standard reader message %d: %v", i, err)
					}
					wire.Reset()
					if _, err := w.Write(message); err != nil {
						t.Fatal(err)
					}
					if err := w.Flush(); err != nil {
						t.Fatal(err)
					}
					got, err = peer.Decompress(wire.Bytes()[:wire.Len()-4], len(message))
					if err != nil || !bytes.Equal(got, message) {
						t.Fatalf("standard writer message %d: %v", i, err)
					}
					history = append(history, message...)
					history = history[max(0, len(history)-(32<<10)):]
				}
			}
		})
	}
}

func TestTakeoverHistoryAndReset(t *testing.T) {
	message := make([]byte, 16000)
	_, _ = rand.New(rand.NewSource(3)).Read(message)
	c, err := deflate.NewCompressor(flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	c.ContextTakeover = true
	d := deflate.Decompressor{ContextTakeover: true}
	first, err := c.Compress(message)
	if err != nil {
		t.Fatal(err)
	}
	first = bytes.Clone(first)
	if _, err := d.Decompress(first, len(message)); err != nil {
		t.Fatal(err)
	}
	second, err := c.Compress(message)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) >= len(first)/2 {
		t.Fatal("compressor did not reuse history")
	}
	second = bytes.Clone(second)
	if got, err := d.Decompress(second, len(message)); err != nil || !bytes.Equal(got, message) {
		t.Fatalf("history: %v", err)
	}
	d.Reset()
	if _, err := d.Decompress(second, len(message)); err == nil {
		t.Fatal("reset retained history")
	}
	c.Reset()
	fresh, err := c.Compress(message)
	if err != nil || !bytes.Equal(fresh, first) {
		t.Fatal("compressor reset retained history")
	}
	for _, bad := range []struct {
		payload []byte
		limit   int
	}{{[]byte{6}, len(message)}, {second, 10}} {
		if _, err := d.Decompress(first, len(message)); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Decompress(bad.payload, bad.limit); err == nil {
			t.Fatal("expected decode error")
		}
		if _, err := d.Decompress(second, len(message)); err == nil {
			t.Fatal("decode error retained history")
		}
	}
	// Switching modes after Reset must also discard history.
	c.Reset()
	d.Reset()
	c.ContextTakeover = false
	d.ContextTakeover = false
	for i := 0; i < 2; i++ {
		compressed, err := c.Compress(message)
		if err != nil || !bytes.Equal(compressed, first) {
			t.Fatal("no-context mode retained history")
		}
		if got, err := d.Decompress(compressed, len(message)); err != nil || !bytes.Equal(got, message) {
			t.Fatalf("no-context mode: %v", err)
		}
	}
}

func TestTakeoverFinalBlocks(t *testing.T) {
	history := make([]byte, 16000)
	_, _ = rand.New(rand.NewSource(4)).Read(history)
	d := deflate.Decompressor{ContextTakeover: true}
	if _, err := d.Decompress(standardCompress(t, history, nil), len(history)); err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	// An empty final block must not discard history from the preceding message.
	for _, message := range [][]byte{nil, history[:1000]} {
		w, err := stdflate.NewWriterDict(&wire, stdflate.DefaultCompression, history)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(message); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	wire.WriteByte(0)
	got, err := d.Decompress(wire.Bytes(), 1000)
	if err != nil || !bytes.Equal(got, history[:1000]) {
		t.Fatalf("final block history: %v", err)
	}
}
