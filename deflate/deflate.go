// Package deflate implements permessage-deflate without context takeover.
// Helpers operate on complete, unmasked message payloads.
// Use them with no_context_takeover negotiated for the corresponding direction
// and the default 32 KB compression window.
package deflate

import (
	"bytes"
	"errors"
	"io"
	"slices"

	"github.com/klauspost/compress/flate"
)

var (
	ErrMessageTooLarge = errors.New("ews/deflate: decompressed message exceeds limit")
	ErrInvalidLimit    = errors.New("ews/deflate: negative output limit")
)

// Compressor reuses storage but starts each message with fresh history.
type Compressor struct {
	writer *flate.Writer
	output bytes.Buffer
}

// NewCompressor creates a compressor using a flate compression level.
func NewCompressor(level int) (*Compressor, error) {
	c := new(Compressor)
	var err error
	c.writer, err = flate.NewWriter(&c.output, level)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Compress borrows its output until the next call. Input remains unchanged.
func (c *Compressor) Compress(payload []byte) ([]byte, error) {
	c.output.Reset()
	c.writer.Reset(&c.output)
	if _, err := c.writer.Write(payload); err != nil {
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		return nil, err
	}
	output := c.output.Bytes()
	return output[:len(output)-4], nil // Strip the permessage-deflate sync-flush tail.
}

type resetReader interface {
	io.ReadCloser
	flate.Resetter
}

// Decompressor reuses storage without retaining history between messages.
// The zero value is ready to use.
type Decompressor struct {
	reader resetReader
	input  messageReader
	output []byte
}

// Decompress borrows its output until the next call. maxSize must be nonnegative.
// It returns ErrMessageTooLarge if the decompressed size exceeds maxSize.
func (d *Decompressor) Decompress(payload []byte, maxSize int) ([]byte, error) {
	if maxSize < 0 {
		return nil, ErrInvalidLimit
	}
	d.output = d.output[:0]
	d.input.payload = payload
	d.input.tail = inflateTail[:]
	defer func() { d.input = messageReader{} }()
	if d.reader == nil {
		d.reader = flate.NewReader(&d.input).(resetReader)
	} else if err := d.reader.Reset(&d.input, nil); err != nil {
		return nil, err
	}
	defer d.reader.Close()

	for {
		size := 32 << 10
		if remaining := maxSize - len(d.output); remaining < size {
			size = remaining + 1
		}
		d.output = slices.Grow(d.output, size)
		n, err := d.reader.Read(d.output[len(d.output) : len(d.output)+size])
		d.output = d.output[:len(d.output)+n]
		if len(d.output) > maxSize {
			return nil, ErrMessageTooLarge
		}
		if err == io.EOF {
			if len(d.input.payload) == 0 && len(d.input.tail) == 0 {
				return d.output, nil
			}
			// RFC 7692 permits final DEFLATE blocks within a message.
			dict := d.output[max(0, len(d.output)-(32<<10)):]
			if err := d.reader.Reset(&d.input, dict); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

// Restore the stripped sync-flush tail, then terminate the DEFLATE stream.
var inflateTail = [...]byte{0, 0, 0xff, 0xff, 1, 0, 0, 0xff, 0xff}

type messageReader struct {
	payload []byte
	tail    []byte
}

func (r *messageReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := copy(p, r.payload)
	r.payload = r.payload[n:]
	m := copy(p[n:], r.tail)
	r.tail = r.tail[m:]
	if n+m == 0 {
		return 0, io.EOF
	}
	return n + m, nil
}

func (r *messageReader) ReadByte() (byte, error) {
	if len(r.payload) != 0 {
		b := r.payload[0]
		r.payload = r.payload[1:]
		return b, nil
	}
	if len(r.tail) != 0 {
		b := r.tail[0]
		r.tail = r.tail[1:]
		return b, nil
	}
	return 0, io.EOF
}
