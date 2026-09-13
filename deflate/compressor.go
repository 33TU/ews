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

	window   int     // Largest dictionary worth priming with: the encoder's window.
	attached *Window // Window whose stream the encoder currently continues.
	gen      uint64  // The window's generation when we last touched it.
	mid      bool    // A chunked message is in progress; the next chunk continues it.
}

// NewCompressor creates a compressor using a flate compression level and the
// full 32 KB window.
func NewCompressor(level int) (*Compressor, error) {
	c := &Compressor{window: windowSize}
	var err error
	c.writer, err = flate.NewWriter(&c.output, level)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// NewCompressorWindow creates a compressor whose matches never reach back
// more than 1<<windowBits bytes, for peers that negotiated a smaller window.
// windowBits is 8 to 15; the compression level is fixed by the encoder.
func NewCompressorWindow(windowBits int) (*Compressor, error) {
	if windowBits < 8 || windowBits > 15 {
		return nil, ErrInvalidWindow
	}
	c := &Compressor{window: 1 << windowBits}
	var err error
	c.writer, err = flate.NewWriterWindow(&c.output, 1<<windowBits)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Compress borrows its output until the next call. Input remains unchanged.
// With a window, the message continues that direction's history and the
// window is updated; with nil, it is compressed on its own.
func (c *Compressor) Compress(payload []byte, w *Window) ([]byte, error) {
	return c.CompressChunk(payload, w, true)
}

// CompressChunk compresses one fragment of a message sent as it is produced.
// The first chunk primes or continues the stream as Compress does, and later
// chunks continue it, so the fragments decode as one message. Only the final
// chunk drops the sync-flush tail; middle chunks keep it, since the receiver
// only restores it once at the end. Output is borrowed until the next call.
func (c *Compressor) CompressChunk(payload []byte, w *Window, final bool) ([]byte, error) {
	c.output.Reset()
	// Mid-message the encoder must continue whatever the window says. Between
	// messages, when nothing but this compressor has touched the window since
	// the last one, the encoder's history already is the window: keep
	// writing. Otherwise prime from the window. Always ResetDict, never
	// Reset: the writer's plain Reset re-primes with the previous dictionary.
	if !c.mid && (w == nil || w != c.attached || w.gen != c.gen) {
		// Priming costs about as much as compressing the dictionary, and the
		// encoder cannot reach past its window, so prime with that much.
		dict := w.dict()
		if len(dict) > c.window {
			dict = dict[len(dict)-c.window:]
		}
		c.writer.ResetDict(&c.output, dict)
	}
	if _, err := c.writer.Write(payload); err != nil {
		c.attached, c.mid = nil, false
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		c.attached, c.mid = nil, false
		return nil, err
	}
	c.attached, c.mid = w, !final
	if w != nil {
		w.remember(payload)
		c.gen = w.gen
	}
	output := c.output.Bytes()
	if final {
		output = output[:len(output)-4] // Strip the permessage-deflate sync-flush tail.
	}
	return output, nil
}

// Reset clears output and detaches from any window, retaining storage.
func (c *Compressor) Reset() {
	c.output.Reset()
	c.writer.ResetDict(&c.output, nil)
	c.attached, c.mid = nil, false
}
