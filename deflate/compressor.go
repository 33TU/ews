package deflate

import (
	"bytes"

	"github.com/klauspost/compress/flate"
)

// Compressor compresses messages using reusable storage. Context takeover
// state lives in the Window passed to each call, so one compressor can serve
// many connections in turn. Priming the encoder from a window costs about as
// much as compressing the window, so a compressor that keeps serving the same
// window continues its stream instead and pays nothing; a connection that
// wants speed keeps a compressor attached, one that wants memory shares it.
type Compressor struct {
	writer *flate.Writer
	output bytes.Buffer

	attached *Window // Window whose stream the encoder currently continues.
	gen      uint64  // The window's generation when we last touched it.
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
// With a window, the message continues that direction's history and the
// window is updated; with nil, it is compressed on its own.
func (c *Compressor) Compress(payload []byte, w *Window) ([]byte, error) {
	c.output.Reset()
	// When nothing but this compressor has touched the window since the last
	// message, the encoder's history already is the window: keep writing.
	// Otherwise prime from the window. Always ResetDict, never Reset: the
	// writer's plain Reset re-primes with the previous dictionary.
	if w == nil || w != c.attached || w.gen != c.gen {
		c.writer.ResetDict(&c.output, w.dict())
	}
	if _, err := c.writer.Write(payload); err != nil {
		c.attached = nil
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		c.attached = nil
		return nil, err
	}
	c.attached = w
	if w != nil {
		w.remember(payload)
		c.gen = w.gen
	}
	output := c.output.Bytes()
	return output[:len(output)-4], nil // Strip the permessage-deflate sync-flush tail.
}

// Reset clears output and detaches from any window, retaining storage.
func (c *Compressor) Reset() {
	c.output.Reset()
	c.writer.ResetDict(&c.output, nil)
	c.attached = nil
}
