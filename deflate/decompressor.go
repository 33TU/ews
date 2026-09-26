package deflate

import (
	"bytes"
	"errors"
	"io"

	"github.com/33TU/ews/internal/bufpool"
	"github.com/klauspost/compress/flate"
)

var (
	// ErrMessageTooLarge is returned by Decompress when the decompressed size
	// exceeds maxSize.
	ErrMessageTooLarge = errors.New("ews/deflate: decompressed message exceeds limit")
	// ErrInvalidLimit is returned by Decompress for a negative maxSize.
	ErrInvalidLimit = errors.New("ews/deflate: negative output limit")
	// ErrNoMessage is returned by Read when no message was started with Begin,
	// or the previous one has ended.
	ErrNoMessage = errors.New("ews/deflate: no message in progress")
	// ErrInvalidWindow is returned by NewCompressorWindow for window bits
	// outside 8 to 15.
	ErrInvalidWindow = errors.New("ews/deflate: window bits must be 8 to 15")
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
	reader resetReader
	// Slice mode feeds the inflater a bytes.Reader over the payload and tail
	// copied into in, which selects its fast path; the generic reader path
	// costs a call per byte. Streaming feeds it input, chunk by chunk.
	slice     bytes.Reader
	in        []byte
	input     messageReader
	output    []byte  // Decompress result; also the reset dictionary in slice mode.
	window    *Window // The current message's direction history, or nil.
	scratch   Window  // Within-message history for streaming without a window.
	kept      int     // Output already added to the window in slice mode.
	streaming bool
	active    bool
	done      bool // Last byte delivered; the next Read returns io.EOF.
}

// Decompress borrows its output until the next call. maxSize must be
// nonnegative, and a message that decompresses past it fails with
// ErrMessageTooLarge.
//
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
	// Reserve for the likely result up front: text and protobuf deflate to
	// a quarter or less, so a message that size skips every intermediate
	// doubling. One extra byte lets a result at the limit be told apart
	// from one past it.
	if hint := min(4*len(payload), maxSize+1); cap(d.output) < hint {
		bufpool.Put(d.output)
		d.output = bufpool.Get(hint)
	}
	for {
		// Each read fills the spare capacity, and the buffer grows once that
		// is used up, not before: a message ending a few KB short of the
		// capacity must not pay for room it never fills. It grows to the
		// output the input consumed so far projects, with a sixteenth of
		// slack, and by at least half again so a message that compresses
		// unevenly still gets there in a few steps. A fixed step past a few
		// hundred KB would leave the append rule adding a quarter per
		// reallocation, and a large message then copies itself several
		// times over on the way out.
		if cap(d.output) == len(d.output) {
			projected := d.projected()
			d.output = grow(d.output, max(32<<10, len(d.output)/2, projected+projected/16))
		}
		size := cap(d.output) - len(d.output)
		if remaining := maxSize - len(d.output); remaining < size {
			size = remaining + 1
		}

		n, err := d.Read(d.output[len(d.output) : len(d.output)+size])
		d.output = d.output[:len(d.output)+n]
		if len(d.output) > maxSize {
			d.finish(false)
			return nil, ErrMessageTooLarge
		}
		if err == io.EOF || d.done {
			// The inflater reports the end together with the last bytes.
			// Finishing here, rather than on the next read, spares a full
			// buffer a doubling that would only ever hold io.EOF.
			d.finish(true)
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
			if d.exhausted() {
				if n == 0 {
					d.finish(true)
					return 0, io.EOF
				}
				d.done = true
				return n, nil
			}

			// RFC 7692 permits final DEFLATE blocks within a message; continue
			// with the message so far as the dictionary.
			if err := d.reader.Reset(d.source(), d.dict(n)); err != nil {
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

// ReleaseOutput is Reset that also gives the output and input storage back
// to the pool, for a caller that keeps the decompressor across messages but
// not a large result. The inflater and its window are kept.
func (d *Decompressor) ReleaseOutput() {
	d.Reset()
	bufpool.Put(d.output)
	bufpool.Put(d.in)
	d.output, d.in = nil, nil
}

// source is the reader the inflater draws from in the current mode.
func (d *Decompressor) source() io.Reader {
	if d.streaming {
		return &d.input
	}
	return &d.slice
}

func (d *Decompressor) exhausted() bool {
	if d.streaming {
		return d.input.exhausted()
	}
	return d.slice.Len() == 0
}

func (d *Decompressor) begin(src ChunkSource, payload []byte, w *Window) error {
	if d.active {
		d.finish(false)
	}
	d.window = w
	d.scratch.Reset()
	d.streaming = src != nil
	if d.streaming {
		d.input = messageReader{src: src, tail: inflateTail[:]}
	} else {
		if n := len(payload) + len(inflateTail); cap(d.in) < n {
			bufpool.Put(d.in)
			d.in = bufpool.Get(n)
		}
		d.in = append(append(d.in[:0], payload...), inflateTail[:]...)
		d.slice.Reset(d.in)
	}
	d.kept = 0
	d.active, d.done = true, false

	if d.reader == nil {
		d.reader = flate.NewReaderDict(d.source(), w.dict()).(resetReader)
		return nil
	}
	if err := d.reader.Reset(d.source(), w.dict()); err != nil {
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
	d.slice.Reset(nil)
	d.active, d.done, d.streaming = false, false, false
}

// projected estimates the output still to come in slice mode from the
// ratio of output to input so far. Before the first byte, or streaming, it
// is zero.
func (d *Decompressor) projected() int {
	if d.streaming {
		return 0
	}
	remaining := d.slice.Len()
	consumed := len(d.in) - remaining
	if consumed <= 0 {
		return 0
	}
	return int(int64(len(d.output)) * int64(remaining) / int64(consumed))
}

// grow returns b with room for n more bytes, from the pool, and returns the
// old storage to it.
func grow(b []byte, n int) []byte {
	if cap(b)-len(b) >= n {
		return b
	}
	nb := append(bufpool.Get(len(b)+n), b...)
	bufpool.Put(b)
	return nb
}

// Restore the stripped sync-flush tail, then terminate the DEFLATE stream.
var inflateTail = [...]byte{0, 0, 0xff, 0xff, 1, 0, 0, 0xff, 0xff}

// messageReader feeds the inflater from a ChunkSource, then the tail.
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
