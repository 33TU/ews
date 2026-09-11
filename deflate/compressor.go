package deflate

import (
	"bytes"

	"github.com/klauspost/compress/flate"
)

// Compressor compresses messages using reusable storage.
type Compressor struct {
	// ContextTakeover retains history between messages. Set before use or after Reset.
	ContextTakeover bool

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
	if !c.ContextTakeover {
		c.writer.Reset(&c.output)
	}
	if _, err := c.writer.Write(payload); err != nil {
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		return nil, err
	}
	output := c.output.Bytes()
	return output[:len(output)-4], nil // Strip the permessage-deflate sync-flush tail.
}

// Reset clears message history and output, retaining storage and configuration.
func (c *Compressor) Reset() {
	c.output.Reset()
	c.writer.Reset(&c.output)
}
