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
					compressed, err := c.Compress(input, nil)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(input, message) {
						t.Fatal("compression changed input")
					}
					decoded, err := d.Decompress(compressed, len(message), nil)
					if err != nil || !bytes.Equal(decoded, message) {
						t.Fatalf("round trip size %d: %v", len(message), err)
					}
					r := stdflate.NewReader(bytes.NewReader(append(bytes.Clone(compressed), tail...)))
					decoded, err = io.ReadAll(r)
					_ = r.Close()
					if err != nil || !bytes.Equal(decoded, message) {
						t.Fatalf("standard reader size %d: %v", len(message), err)
					}
					decoded, err = d.Decompress(standardCompress(t, message, nil), len(message), nil)
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
		got, err := d.Decompress(payload, 5, nil)
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
	first, err := c.Compress(message, nil)
	if err != nil {
		t.Fatal(err)
	}
	first = bytes.Clone(first)
	if _, err := c.Compress([]byte("different message"), nil); err != nil {
		t.Fatal(err)
	}
	second, err := c.Compress(message, nil)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("compression retained message history")
	}
	var d deflate.Decompressor
	if _, err := d.Decompress(first, len(message), nil); err != nil {
		t.Fatal(err)
	}
	withHistory := standardCompress(t, message, message)
	if _, err := d.Decompress(withHistory, len(message), nil); err == nil {
		t.Fatal("decompression accepted a reference to the previous message")
	}
	if got, err := d.Decompress(first, len(message), nil); err != nil || !bytes.Equal(got, message) {
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
	got, err := d.Decompress(compressed.Bytes(), len(want), nil)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("final blocks lost message history: %v", err)
	}
}

func TestLimitsAndInvalidInput(t *testing.T) {
	var d deflate.Decompressor
	compressed := standardCompress(t, []byte("Hello"), nil)
	for _, limit := range []int{0, 4} {
		if got, err := d.Decompress(compressed, limit, nil); got != nil || !errors.Is(err, deflate.ErrMessageTooLarge) {
			t.Fatalf("limit %d: got %x, %v", limit, got, err)
		}
	}
	if _, err := d.Decompress(compressed, -1, nil); !errors.Is(err, deflate.ErrInvalidLimit) {
		t.Fatal("negative limit accepted")
	}
	for _, input := range [][]byte{nil, {0x06}, {0x00, 0x01}} {
		if _, err := d.Decompress(input, 1024, nil); err == nil {
			t.Fatalf("invalid input %x accepted", input)
		}
	}
	if got, err := d.Decompress(compressed, 5, nil); err != nil || string(got) != "Hello" {
		t.Fatal("decompressor did not recover")
	}
	if got, err := d.Decompress([]byte{0}, 0, nil); err != nil || len(got) != 0 {
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
	compressed, err := c.Compress(message, nil)
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
	got, err := d.Decompress(assembled, len(message), nil)
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
			var cw, dw *deflate.Window
			if takeover {
				cw, dw = new(deflate.Window), new(deflate.Window)
			}
			var d deflate.Decompressor
			// Prime both histories with one message, then measure a second.
			compressed, err := c.Compress(payload, cw)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := d.Decompress(bytes.Clone(compressed), len(payload), dw); err != nil {
				b.Fatal(err)
			}
			compressed, err = c.Compress(payload, cw)
			if err != nil {
				b.Fatal(err)
			}
			compressed = bytes.Clone(compressed)
			b.Run("compress", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				for b.Loop() {
					if _, err := c.Compress(payload, cw); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("decompress", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				for b.Loop() {
					// The window keeps the same content as the payload repeats.
					if _, err := d.Decompress(compressed, len(payload), dw); err != nil {
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
			var d, peer deflate.Decompressor
			var cw, dw, pw deflate.Window
			var wire bytes.Buffer
			w, err := stdflate.NewWriter(&wire, stdflate.DefaultCompression)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			var history []byte
			for round := 0; round < 2; round++ {
				cw.Reset()
				dw.Reset()
				pw.Reset()
				w.Reset(&wire)
				history = history[:0]
				for i, message := range messages {
					compressed, err := c.Compress(message, &cw)
					if err != nil {
						t.Fatal(err)
					}
					got, err := d.Decompress(compressed, len(message), &dw)
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
					got, err = peer.Decompress(wire.Bytes()[:wire.Len()-4], len(message), &pw)
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
	var d deflate.Decompressor
	var cw, dw deflate.Window
	first, err := c.Compress(message, &cw)
	if err != nil {
		t.Fatal(err)
	}
	first = bytes.Clone(first)
	if _, err := d.Decompress(first, len(message), &dw); err != nil {
		t.Fatal(err)
	}
	second, err := c.Compress(message, &cw)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) >= len(first)/2 {
		t.Fatal("compressor did not reuse history")
	}
	second = bytes.Clone(second)
	if got, err := d.Decompress(second, len(message), &dw); err != nil || !bytes.Equal(got, message) {
		t.Fatalf("history: %v", err)
	}
	dw.Reset()
	if _, err := d.Decompress(second, len(message), &dw); err == nil {
		t.Fatal("reset retained history")
	}
	cw.Reset()
	fresh, err := c.Compress(message, &cw)
	if err != nil || !bytes.Equal(fresh, first) {
		t.Fatal("compressor reset retained history")
	}
	for _, bad := range []struct {
		payload []byte
		limit   int
	}{{[]byte{6}, len(message)}, {second, 10}} {
		dw.Reset()
		if _, err := d.Decompress(first, len(message), &dw); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Decompress(bad.payload, bad.limit, &dw); err == nil {
			t.Fatal("expected decode error")
		}
		if _, err := d.Decompress(second, len(message), &dw); err == nil {
			t.Fatal("decode error retained history")
		}
	}
	// Without a window every message stands alone, whatever came before.
	for i := 0; i < 2; i++ {
		compressed, err := c.Compress(message, nil)
		if err != nil || !bytes.Equal(compressed, first) {
			t.Fatal("nil window retained history")
		}
		if got, err := d.Decompress(compressed, len(message), nil); err != nil || !bytes.Equal(got, message) {
			t.Fatalf("nil window: %v", err)
		}
	}
}

// TestAttachedWindow checks stream continuation and re-priming: one
// compressor stays on a window, then gets interrupted by another window, a
// window reset, and a second compressor, and every message must still decode.
func TestAttachedWindow(t *testing.T) {
	a, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	b, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	var d deflate.Decompressor
	var w1, w2, r1, r2 deflate.Window
	noise := make([]byte, 8000)
	_, _ = rand.New(rand.NewSource(5)).Read(noise)
	step := func(c *deflate.Compressor, send, recv *deflate.Window, i int) {
		t.Helper()
		message := append(bytes.Clone(noise), byte(i))
		compressed, err := c.Compress(message, send)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && len(compressed) > len(message)/4 {
			t.Fatalf("step %d: history unused, %d bytes", i, len(compressed))
		}
		got, err := d.Decompress(bytes.Clone(compressed), len(message), recv)
		if err != nil || !bytes.Equal(got, message) {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	step(a, &w1, &r1, 0)
	step(a, &w1, &r1, 1) // Continues the stream.
	step(a, &w1, &r1, 2)
	step(a, &w2, &r2, 0) // Another connection: primes.
	step(a, &w1, &r1, 3) // Back: primes from w1.
	step(b, &w1, &r1, 4) // A different compressor on the same window: primes.
	step(a, &w1, &r1, 5) // a's cached state is stale; the generation catches it.
	w1.Reset()
	r1.Reset()
	step(a, &w1, &r1, 0) // Reset windows start over.
	step(a, &w1, &r1, 1)
	step(a, nil, nil, 0) // No window compresses alone.
	step(a, &w1, &r1, 2)
}

// TestWindowed checks compressors with reduced windows against a standard
// decoder, with and without history, and the accepted range.
func TestWindowed(t *testing.T) {
	message := bytes.Repeat([]byte("windowed compression keeps matches short "), 800)
	for _, bits := range []int{8, 9, 12, 15} {
		c, err := deflate.NewCompressorWindow(bits)
		if err != nil {
			t.Fatal(err)
		}
		var cw deflate.Window
		var history []byte
		for i := 0; i < 3; i++ {
			compressed, err := c.Compress(message, &cw)
			if err != nil {
				t.Fatal(err)
			}
			r := stdflate.NewReaderDict(bytes.NewReader(append(bytes.Clone(compressed), tail...)), history)
			got, err := io.ReadAll(r)
			r.Close()
			if err != nil || !bytes.Equal(got, message) {
				t.Fatalf("bits %d message %d: %v", bits, i, err)
			}
			history = append(history, message...)
			history = history[max(0, len(history)-(32<<10)):]
		}
	}
	for _, bits := range []int{7, 16} {
		if _, err := deflate.NewCompressorWindow(bits); err != deflate.ErrInvalidWindow {
			t.Fatalf("bits %d: %v", bits, err)
		}
	}
}

// TestCompressChunk sends messages in chunks and decodes the concatenated
// fragments as one stream, with and without a window, then checks that a
// following whole message still works.
func TestCompressChunk(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	message := bytes.Repeat([]byte("chunked message fragments share one deflate stream "), 600)
	for _, w := range []*deflate.Window{nil, new(deflate.Window)} {
		var d deflate.Decompressor
		var dw *deflate.Window
		if w != nil {
			dw = new(deflate.Window)
		}
		for round := 0; round < 2; round++ {
			var wire []byte
			for i := 0; i < len(message); i += 7000 {
				end := min(i+7000, len(message))
				out, err := c.CompressChunk(message[i:end], w, end == len(message))
				if err != nil {
					t.Fatal(err)
				}
				wire = append(wire, out...)
			}
			got, err := d.Decompress(wire, len(message), dw)
			if err != nil || !bytes.Equal(got, message) {
				t.Fatalf("window=%v round %d: %v", w != nil, round, err)
			}
			// An empty final chunk is how a sender ends a message of unknown length.
			out, err := c.CompressChunk(message[:100], w, false)
			if err != nil {
				t.Fatal(err)
			}
			wire = bytes.Clone(out)
			if out, err = c.CompressChunk(nil, w, true); err != nil {
				t.Fatal(err)
			}
			wire = append(wire, out...)
			if got, err := d.Decompress(wire, 100, dw); err != nil || !bytes.Equal(got, message[:100]) {
				t.Fatalf("empty final chunk: %v", err)
			}
		}
		whole, err := c.Compress(message, w)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := d.Decompress(bytes.Clone(whole), len(message), dw); err != nil || !bytes.Equal(got, message) {
			t.Fatalf("whole message after chunks: %v", err)
		}
	}
}

// TestWindowBits sizes both windows to a 9-bit negotiation: the compressor
// reaches back at most 512 bytes and the receiver keeps and primes with only
// that much, and every message still decodes.
func TestWindowBits(t *testing.T) {
	c, err := deflate.NewCompressorWindow(9)
	if err != nil {
		t.Fatal(err)
	}
	var d deflate.Decompressor
	send, recv := deflate.Window{Bits: 9}, deflate.Window{Bits: 9}
	message := bytes.Repeat([]byte("nine-bit windows on both sides "), 300)
	for i := 0; i < 4; i++ {
		compressed, err := c.Compress(message, &send)
		if err != nil {
			t.Fatal(err)
		}
		got, err := d.Decompress(bytes.Clone(compressed), len(message), &recv)
		if err != nil || !bytes.Equal(got, message) {
			t.Fatalf("message %d: %v", i, err)
		}
	}
	// An out-of-range Bits falls back to the full window.
	full := deflate.Window{Bits: 3}
	c15, _ := deflate.NewCompressor(flate.BestSpeed)
	compressed, _ := c15.Compress(message, &full)
	if got, err := d.Decompress(bytes.Clone(compressed), len(message), &deflate.Window{Bits: 99}); err != nil || !bytes.Equal(got, message) {
		t.Fatalf("fallback window: %v", err)
	}
}

// TestSharedHelpers interleaves two connections through one compressor and
// one decompressor; each connection's history must stay intact.
func TestSharedHelpers(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	var d deflate.Decompressor
	// Random messages are incompressible on their own and tiny against a
	// window holding the same bytes, so history use is unmistakable.
	conns := []struct {
		send, recv deflate.Window
		message    []byte
	}{{message: make([]byte, 16000)}, {message: make([]byte, 16000)}}
	for i := range conns {
		_, _ = rand.New(rand.NewSource(int64(10 + i))).Read(conns[i].message)
	}
	for round := 0; round < 3; round++ {
		for i := range conns {
			cn := &conns[i]
			compressed, err := c.Compress(cn.message, &cn.send)
			if err != nil {
				t.Fatal(err)
			}
			if round == 0 && len(compressed) < len(cn.message)/2 || round > 0 && len(compressed) > len(cn.message)/10 {
				t.Fatalf("round %d connection %d: %d bytes", round, i, len(compressed))
			}
			got, err := d.Decompress(bytes.Clone(compressed), len(cn.message), &cn.recv)
			if err != nil || !bytes.Equal(got, cn.message) {
				t.Fatalf("round %d connection %d: %v", round, i, err)
			}
		}
	}
}

func TestTakeoverFinalBlocks(t *testing.T) {
	history := make([]byte, 16000)
	_, _ = rand.New(rand.NewSource(4)).Read(history)
	var d deflate.Decompressor
	var dw deflate.Window
	if _, err := d.Decompress(standardCompress(t, history, nil), len(history), &dw); err != nil {
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
	got, err := d.Decompress(wire.Bytes(), 1000, &dw)
	if err != nil || !bytes.Equal(got, history[:1000]) {
		t.Fatalf("final block history: %v", err)
	}
}
