package deflate_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/33TU/ews/deflate"
	"github.com/klauspost/compress/flate"
)

func TestDecompress(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	var d deflate.Decompressor
	for _, size := range []int{0, 1, 100, 200_000} {
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			message := bytes.Repeat([]byte("message payload "), size/16+1)[:size]
			compressed := bytes.Clone(compress(t, c, message))
			got, err := d.Decompress(compressed, len(message), nil)
			if err != nil || !bytes.Equal(got, message) {
				t.Fatalf("%d bytes, %v", len(got), err)
			}
			// The limit is exact: one byte less fails without clearing state
			// the next message needs.
			if size > 0 {
				if _, err := d.Decompress(compressed, len(message)-1, nil); err != deflate.ErrMessageTooLarge {
					t.Fatalf("limit not enforced: %v", err)
				}
			}
			if got, err := d.Decompress(compressed, len(message), nil); err != nil || !bytes.Equal(got, message) {
				t.Fatal("decompressor unusable after a failed message")
			}
		})
	}
	if _, err := d.Decompress(nil, -1, nil); err != deflate.ErrInvalidLimit {
		t.Fatal("negative limit accepted")
	}
}

func TestFinalBlocks(t *testing.T) {
	message := bytes.Repeat([]byte("dictionary within one message"), 256)
	var compressed bytes.Buffer
	for _, dict := range [][]byte{nil, message} {
		w, err := flate.NewWriterDict(&compressed, flate.BestCompression, dict)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(message)
		w.Close()
	}
	compressed.WriteByte(0)
	var d deflate.Decompressor
	want := bytes.Repeat(message, 2)
	got, err := d.Decompress(compressed.Bytes(), len(want), nil)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("final blocks lost message history: %v", err)
	}
	// The same through a takeover window, which then holds the message.
	var w deflate.Window
	if got, err := d.Decompress(compressed.Bytes(), len(want), &w); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("final blocks with a window: %v", err)
	}
}

func TestTakeover(t *testing.T) {
	c, err := deflate.NewCompressor(flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	var d deflate.Decompressor
	var cw, dw deflate.Window
	message := bytes.Repeat([]byte("history carries between messages "), 100)
	for i := range 5 {
		compressed := bytes.Clone(compressWith(t, c, message, &cw))
		if i > 0 && len(compressed) > 64 {
			t.Fatalf("message %d did not use the history: %d bytes", i, len(compressed))
		}
		if got, err := d.Decompress(compressed, len(message), &dw); err != nil || !bytes.Equal(got, message) {
			t.Fatalf("message %d: %v", i, err)
		}
	}
}

func TestDecompressErrors(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	message := bytes.Repeat([]byte("abc"), 1000)
	var cw, dw deflate.Window
	first := bytes.Clone(compressWith(t, c, message, &cw))
	second := bytes.Clone(compressWith(t, c, message, &cw))

	// Corrupt data fails and clears the window, so the next message of the
	// stream cannot be decoded against it.
	var d deflate.Decompressor
	if _, err := d.Decompress(first, len(message), &dw); err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Clone(second)
	corrupt[len(corrupt)/2] ^= 0xff
	if _, err := d.Decompress(corrupt, len(message), &dw); err == nil {
		t.Fatal("corrupt input accepted")
	}
	if _, err := d.Decompress(second, len(message), &dw); err == nil {
		t.Fatal("window survived a failed message")
	}

	// An oversized message clears the window too.
	dw.Reset()
	if _, err := d.Decompress(first, len(message), &dw); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Decompress(second, 10, &dw); err != deflate.ErrMessageTooLarge {
		t.Fatalf("limit not enforced: %v", err)
	}
	if _, err := d.Decompress(second, len(message), &dw); err == nil {
		t.Fatal("window survived an oversized message")
	}
}

// chunks serves wire in fixed-size pieces, with an empty chunk sprinkled in,
// and fails with err after failAfter chunks when err is set.
type chunks struct {
	wire      []byte
	size      int
	served    int
	err       error
	failAfter int
}

func (c *chunks) NextChunk() ([]byte, error) {
	if c.err != nil && c.served == c.failAfter {
		return nil, c.err
	}
	c.served++
	if c.served == 2 {
		return nil, nil // Sources may hand out empty chunks.
	}
	if len(c.wire) == 0 {
		return nil, io.EOF
	}
	n := min(c.size, len(c.wire))
	chunk := c.wire[:n]
	c.wire = c.wire[n:]
	return chunk, nil
}

// drain reads a streamed message with buffers of readSize bytes.
func drain(t *testing.T, d *deflate.Decompressor, readSize int) ([]byte, error) {
	t.Helper()
	var out []byte
	buf := make([]byte, readSize)
	for {
		n, err := d.Read(buf)
		if err == io.EOF {
			if n != 0 {
				t.Fatal("data returned with io.EOF")
			}
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if n == 0 {
			t.Fatal("Read returned 0, nil")
		}
		out = append(out, buf[:n]...)
	}
}

func TestStreaming(t *testing.T) {
	message := bytes.Repeat([]byte("streamed message payload "), 5000)
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	compressed := bytes.Clone(compress(t, c, message))
	for _, chunk := range []int{1, 7, 4096, len(compressed) + 1} {
		for _, read := range []int{1, 100, 64 << 10} {
			t.Run(fmt.Sprintf("chunk=%d/read=%d", chunk, read), func(t *testing.T) {
				var d deflate.Decompressor
				if err := d.Begin(&chunks{wire: compressed, size: chunk}, nil); err != nil {
					t.Fatal(err)
				}
				got, err := drain(t, &d, read)
				if err != nil || !bytes.Equal(got, message) {
					t.Fatalf("%d bytes, %v", len(got), err)
				}
				if _, err := d.Read(make([]byte, 1)); err != deflate.ErrNoMessage {
					t.Fatal("Read outside a message must fail")
				}
				// The slice path still works on the same decompressor.
				if got, err := d.Decompress(compressed, len(message), nil); err != nil || !bytes.Equal(got, message) {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestStreamingFinalBlocks(t *testing.T) {
	message := bytes.Repeat([]byte("dictionary within one message"), 256)
	var compressed bytes.Buffer
	for _, dict := range [][]byte{nil, message} {
		w, err := flate.NewWriterDict(&compressed, flate.BestCompression, dict)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(message)
		w.Close()
	}
	compressed.WriteByte(0)
	var d deflate.Decompressor
	if err := d.Begin(&chunks{wire: compressed.Bytes(), size: 100}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := drain(t, &d, 777)
	if err != nil || !bytes.Equal(got, bytes.Repeat(message, 2)) {
		t.Fatalf("final blocks lost message history: %v", err)
	}
}

func TestStreamingTakeover(t *testing.T) {
	c, err := deflate.NewCompressor(flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	var d deflate.Decompressor
	var cw, dw deflate.Window
	message := bytes.Repeat([]byte("history carries between streamed messages "), 100)
	for i := range 5 {
		compressed := bytes.Clone(compressWith(t, c, message, &cw))
		// Alternate streaming and slice reads; history must be shared.
		if i%2 == 0 {
			if err := d.Begin(&chunks{wire: compressed, size: 50}, &dw); err != nil {
				t.Fatal(err)
			}
			if got, err := drain(t, &d, 1000); err != nil || !bytes.Equal(got, message) {
				t.Fatalf("message %d: %v", i, err)
			}
		} else if got, err := d.Decompress(compressed, len(message), &dw); err != nil || !bytes.Equal(got, message) {
			t.Fatalf("message %d: %v", i, err)
		}
	}
}

func TestStreamingErrors(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	message := bytes.Repeat([]byte("abc"), 1000)
	var cw, dw deflate.Window
	first := bytes.Clone(compressWith(t, c, message, &cw))
	second := bytes.Clone(compressWith(t, c, message, &cw))

	// A source error is returned as is and clears the window.
	var d deflate.Decompressor
	if _, err := d.Decompress(first, len(message), &dw); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if err := d.Begin(&chunks{wire: second, size: 10, err: boom, failAfter: 3}, &dw); err != nil {
		t.Fatal(err)
	}
	if _, err := drain(t, &d, 100); err != boom {
		t.Fatalf("source error not propagated: %v", err)
	}
	if _, err := d.Read(make([]byte, 1)); err != deflate.ErrNoMessage {
		t.Fatal("message still active after error")
	}
	if _, err := d.Decompress(second, len(message), &dw); err == nil {
		t.Fatal("window survived a failed message")
	}

	// Corrupt data fails the same way.
	d.Reset()
	corrupt := bytes.Clone(first)
	corrupt[len(corrupt)/2] ^= 0xff
	if err := d.Begin(&chunks{wire: corrupt, size: 1 << 20}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := drain(t, &d, 100); err == nil {
		t.Fatal("corrupt input accepted")
	}

	// Begin during a message abandons it.
	d.Reset()
	d.Begin(&chunks{wire: first, size: 10}, nil)
	d.Read(make([]byte, 10))
	if err := d.Begin(&chunks{wire: first, size: 10}, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := drain(t, &d, 100); err != nil || !bytes.Equal(got, message) {
		t.Fatal(err)
	}
}

func compress(t *testing.T, c *deflate.Compressor, message []byte) []byte {
	return compressWith(t, c, message, nil)
}

func compressWith(t *testing.T, c *deflate.Compressor, message []byte, w *deflate.Window) []byte {
	t.Helper()
	b, err := c.Compress(message, w)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// FuzzDecompress runs arbitrary bytes through the inflater, with and without
// a window, and checks that what the compressor produces round-trips.
func FuzzDecompress(f *testing.F) {
	f.Add([]byte("hello hello hello"), uint8(0))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff}, uint8(1))
	f.Add(bytes.Repeat([]byte("abc"), 20000), uint8(9))
	f.Fuzz(func(t *testing.T, data []byte, level uint8) {
		var d deflate.Decompressor
		var w deflate.Window
		// Arbitrary input: no panic, and an error clears the window.
		if _, err := d.Decompress(data, 1<<20, &w); err != nil && w.Size() != 32<<10 {
			t.Fatal("window size changed")
		}
		d.Decompress(data, 1<<20, nil)

		// Compressed input round-trips through a fresh window pair.
		c, err := deflate.NewCompressor(int(level)%10 - 1) // -1 (default) through 8.
		if err != nil {
			t.Fatal(err)
		}
		var cw, dw deflate.Window
		for i := 0; i < 2; i++ {
			compressed := bytes.Clone(compressWith(t, c, data, &cw))
			got, err := d.Decompress(compressed, len(data), &dw)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("round %d: %d bytes, %v", i, len(got), err)
			}
		}
	})
}
