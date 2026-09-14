package deflate_test

import (
	"bytes"
	"fmt"
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
