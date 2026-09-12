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
				if err := d.Begin(&chunks{wire: compressed, size: chunk}); err != nil {
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
				if got, err := d.Decompress(compressed, len(message)); err != nil || !bytes.Equal(got, message) {
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
	if err := d.Begin(&chunks{wire: compressed.Bytes(), size: 100}); err != nil {
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
	c.ContextTakeover = true
	d := deflate.Decompressor{ContextTakeover: true}
	message := bytes.Repeat([]byte("history carries between streamed messages "), 100)
	for i := range 5 {
		compressed := bytes.Clone(compress(t, c, message))
		// Alternate streaming and slice reads; history must be shared.
		if i%2 == 0 {
			if err := d.Begin(&chunks{wire: compressed, size: 50}); err != nil {
				t.Fatal(err)
			}
			if got, err := drain(t, &d, 1000); err != nil || !bytes.Equal(got, message) {
				t.Fatalf("message %d: %v", i, err)
			}
		} else if got, err := d.Decompress(compressed, len(message)); err != nil || !bytes.Equal(got, message) {
			t.Fatalf("message %d: %v", i, err)
		}
	}
}

func TestStreamingErrors(t *testing.T) {
	c, err := deflate.NewCompressor(flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	c.ContextTakeover = true
	message := bytes.Repeat([]byte("abc"), 1000)
	first := bytes.Clone(compress(t, c, message))
	second := bytes.Clone(compress(t, c, message))

	// A source error is returned as is and clears history.
	d := deflate.Decompressor{ContextTakeover: true}
	if _, err := d.Decompress(first, len(message)); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if err := d.Begin(&chunks{wire: second, size: 10, err: boom, failAfter: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := drain(t, &d, 100); err != boom {
		t.Fatalf("source error not propagated: %v", err)
	}
	if _, err := d.Read(make([]byte, 1)); err != deflate.ErrNoMessage {
		t.Fatal("message still active after error")
	}
	if _, err := d.Decompress(second, len(message)); err == nil {
		t.Fatal("history survived a failed message")
	}

	// Corrupt data fails the same way.
	d.Reset()
	corrupt := bytes.Clone(first)
	corrupt[len(corrupt)/2] ^= 0xff
	if err := d.Begin(&chunks{wire: corrupt, size: 1 << 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := drain(t, &d, 100); err == nil {
		t.Fatal("corrupt input accepted")
	}

	// Begin during a message abandons it.
	d.Reset()
	d.Begin(&chunks{wire: first, size: 10})
	d.Read(make([]byte, 10))
	if err := d.Begin(&chunks{wire: first, size: 10}); err != nil {
		t.Fatal(err)
	}
	if got, err := drain(t, &d, 100); err != nil || !bytes.Equal(got, message) {
		t.Fatal(err)
	}
}

func compress(t *testing.T, c *deflate.Compressor, message []byte) []byte {
	t.Helper()
	b, err := c.Compress(message)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
