package deflate

import (
	"errors"
	"io"
	"slices"

	"github.com/klauspost/compress/flate"
)

var (
	ErrMessageTooLarge = errors.New("ews/deflate: decompressed message exceeds limit")
	ErrInvalidLimit    = errors.New("ews/deflate: negative output limit")
	ErrNoMessage       = errors.New("ews/deflate: no message in progress")
	ErrInvalidWindow   = errors.New("ews/deflate: window bits must be 8 to 15")
)

// ChunkSource supplies the compressed bytes of one message in order. Chunks
// are borrowed until the next call and may be empty. It returns io.EOF after
// the last chunk. Any other error ends the message and is returned by Read.
type ChunkSource interface {
	NextChunk() ([]byte, error)
}

type resetReader interface {
	io.ReadCloser
	flate.Resetter
}

// Decompressor decompresses messages using reusable storage. It carries no
// state between messages, so one decompressor can serve many connections in
// turn; context takeover lives in the Window passed to each call.
// The zero value is ready to use.
type Decompressor struct {
	reader    resetReader
	input     messageReader
	output    []byte  // Decompress result; also the reset dictionary in slice mode.
	window    *Window // The current message's direction history, or nil.
	scratch   Window  // Within-message history for streaming without a window.
	kept      int     // Output already added to the window in slice mode.
	streaming bool
	active    bool
	done      bool // Last byte delivered; the next Read returns io.EOF.
}

// Decompress borrows its output until the next call. maxSize must be nonnegative.
// It returns ErrMessageTooLarge if the decompressed size exceeds maxSize.
// With a window, the message continues that direction's history and the
// window is updated; with nil, it is decompressed on its own. Any error
// clears the window.
func (d *Decompressor) Decompress(payload []byte, maxSize int, w *Window) ([]byte, error) {
	if maxSize < 0 {
		return nil, ErrInvalidLimit
	}
	if err := d.begin(nil, payload, w); err != nil {
		return nil, err
	}
	d.output = d.output[:0]
	for {
		size := 32 << 10
		if remaining := maxSize - len(d.output); remaining < size {
			size = remaining + 1
		}
		d.output = slices.Grow(d.output, size)
		n, err := d.Read(d.output[len(d.output) : len(d.output)+size])
		d.output = d.output[:len(d.output)+n]
		if len(d.output) > maxSize {
			d.finish(false)
			return nil, ErrMessageTooLarge
		}
		if err == io.EOF {
			return d.output, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// Begin starts decompressing a message streamed from src; drain it with Read.
// The window works as for Decompress. A message still in progress is
// abandoned, which clears its window.
func (d *Decompressor) Begin(src ChunkSource, w *Window) error {
	return d.begin(src, nil, w)
}

// Read decompresses into p and returns io.EOF, with no data, after the last
// byte of the message. Output is never returned together with io.EOF.
// Any other error ends the message and clears its window.
func (d *Decompressor) Read(p []byte) (int, error) {
	if !d.active {
		return 0, ErrNoMessage
	}
	if d.done {
		d.finish(true)
		return 0, io.EOF
	}
	for {
		n, err := d.reader.Read(p)
		// Streamed output leaves with the caller, so its history is kept as
		// it is produced. In slice mode the whole output is at hand and the
		// window is updated once, when the message ends or a reset needs it.
		if n != 0 && d.streaming {
			if d.window != nil {
				d.window.remember(p[:n])
			} else {
				d.scratch.remember(p[:n])
			}
		}
		if err == io.EOF {
			if d.input.exhausted() {
				if n == 0 {
					d.finish(true)
					return 0, io.EOF
				}
				d.done = true
				return n, nil
			}
			// RFC 7692 permits final DEFLATE blocks within a message; continue
			// with the message so far as the dictionary.
			if err := d.reader.Reset(&d.input, d.dict(n)); err != nil {
				return 0, d.fail(err)
			}
			if n != 0 {
				return n, nil
			}
			continue
		}
		if err != nil {
			return 0, d.fail(err)
		}
		if n == 0 {
			return 0, d.fail(io.ErrNoProgress)
		}
		return n, nil
	}
}

// Reset abandons any message in progress and clears output, retaining storage.
func (d *Decompressor) Reset() {
	d.finish(false)
	d.output = d.output[:0]
}

func (d *Decompressor) begin(src ChunkSource, payload []byte, w *Window) error {
	if d.active {
		d.finish(false)
	}
	d.window = w
	d.scratch.Reset()
	d.input = messageReader{src: src, payload: payload, tail: inflateTail[:], eof: src == nil}
	d.streaming = src != nil
	d.kept = 0
	d.active, d.done = true, false
	if d.reader == nil {
		d.reader = flate.NewReaderDict(&d.input, w.dict()).(resetReader)
		return nil
	}
	if err := d.reader.Reset(&d.input, w.dict()); err != nil {
		return d.fail(err)
	}
	return nil
}

// dict is the dictionary for a mid-message reset: the direction's window when
// there is one, brought up to date with the output so far in slice mode; the
// within-message history when streaming; otherwise the tail of the output so
// far. In slice mode Read is only called by Decompress with the spare
// capacity of d.output, so the n bytes just produced sit directly after it.
func (d *Decompressor) dict(n int) []byte {
	if d.streaming {
		if d.window != nil {
			return d.window.dict()
		}
		return d.scratch.dict()
	}
	out := d.output[:len(d.output)+n]
	if d.window != nil {
		d.window.remember(out[d.kept:])
		d.kept = len(out)
		return d.window.dict()
	}
	return out[max(0, len(out)-windowSize):]
}

func (d *Decompressor) fail(err error) error {
	d.finish(false)
	return err
}

// finish ends the current message. Success in slice mode adds the output to
// the window, all at once; failure clears the window, since the takeover
// stream cannot continue past a corrupt message.
func (d *Decompressor) finish(ok bool) {
	if d.window != nil {
		switch {
		case !ok:
			d.window.Reset()
		case !d.streaming:
			d.window.remember(d.output[d.kept:])
		}
	}
	d.window = nil
	d.input = messageReader{}
	d.active, d.done, d.streaming = false, false, false
}

// Restore the stripped sync-flush tail, then terminate the DEFLATE stream.
var inflateTail = [...]byte{0, 0, 0xff, 0xff, 1, 0, 0, 0xff, 0xff}

// messageReader feeds the inflater from a slice or a ChunkSource, then the tail.
type messageReader struct {
	src     ChunkSource
	payload []byte
	tail    []byte
	eof     bool // No more chunks; in slice mode from the start.
}

func (r *messageReader) exhausted() bool {
	return r.eof && len(r.payload) == 0 && len(r.tail) == 0
}

// fill pulls the next nonempty chunk. It returns false at end of input or on error.
func (r *messageReader) fill() (bool, error) {
	for len(r.payload) == 0 && !r.eof {
		chunk, err := r.src.NextChunk()
		if err == io.EOF {
			r.eof = true
			return false, nil
		}
		if err != nil {
			return false, err
		}
		r.payload = chunk
	}
	return len(r.payload) != 0, nil
}

func (r *messageReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	ok, err := r.fill()
	if err != nil {
		return 0, err
	}
	if ok {
		n := copy(p, r.payload)
		r.payload = r.payload[n:]
		return n, nil
	}
	n := copy(p, r.tail)
	r.tail = r.tail[n:]
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (r *messageReader) ReadByte() (byte, error) {
	ok, err := r.fill()
	if err != nil {
		return 0, err
	}
	if ok {
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
