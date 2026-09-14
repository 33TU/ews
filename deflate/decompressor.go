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
	ErrInvalidWindow   = errors.New("ews/deflate: window bits must be 8 to 15")
)

type resetReader interface {
	io.ReadCloser
	flate.Resetter
}

// Decompressor decompresses whole messages using reusable storage. It carries
// no state between messages, so one decompressor can serve many connections
// in turn; context takeover lives in the Window passed to each call.
// The zero value is ready to use.
type Decompressor struct {
	reader resetReader
	input  messageReader
	output []byte
}

// Decompress borrows its output until the next call. maxSize must be
// nonnegative. It returns ErrMessageTooLarge if the decompressed size exceeds
// maxSize. With a window, the message continues that direction's history and
// the window is updated; with nil, it is decompressed on its own. Any error
// clears the window, since the peers' histories have diverged.
func (d *Decompressor) Decompress(payload []byte, maxSize int, w *Window) ([]byte, error) {
	if maxSize < 0 {
		return nil, ErrInvalidLimit
	}
	d.input = messageReader{payload: payload, tail: inflateTail[:]}
	d.output = d.output[:0]
	if d.reader == nil {
		d.reader = flate.NewReaderDict(&d.input, w.dict()).(resetReader)
	} else if err := d.reader.Reset(&d.input, w.dict()); err != nil {
		return nil, d.fail(w, err)
	}
	for {
		// Inflate in window-sized steps, reaching one byte past the limit so
		// an oversized message fails without being inflated in full.
		size := windowSize
		if remaining := maxSize - len(d.output); remaining < size {
			size = remaining + 1
		}
		d.output = slices.Grow(d.output, size)
		n, err := d.reader.Read(d.output[len(d.output) : len(d.output)+size])
		d.output = d.output[:len(d.output)+n]
		if len(d.output) > maxSize {
			return nil, d.fail(w, ErrMessageTooLarge)
		}
		if w != nil && n != 0 {
			w.remember(d.output[len(d.output)-n:])
		}
		switch {
		case err == nil:
			if n == 0 {
				return nil, d.fail(w, io.ErrNoProgress)
			}
		case err == io.EOF:
			if d.input.exhausted() {
				return d.output, nil
			}
			// RFC 7692 permits final DEFLATE blocks within a message; continue
			// with the message so far as the dictionary.
			if err := d.reader.Reset(&d.input, d.dict(w)); err != nil {
				return nil, d.fail(w, err)
			}
		default:
			return nil, d.fail(w, err)
		}
	}
}

// Reset clears the output, retaining storage.
func (d *Decompressor) Reset() {
	d.output = d.output[:0]
	d.input = messageReader{}
}

// dict is the dictionary for a mid-message reset: the direction's window,
// which already holds the output so far, or without one the tail of that
// output.
func (d *Decompressor) dict(w *Window) []byte {
	if w != nil {
		return w.dict()
	}
	return d.output[max(0, len(d.output)-windowSize):]
}

// fail ends the message. The window is cleared, since the takeover stream
// cannot continue past a corrupt message.
func (d *Decompressor) fail(w *Window, err error) error {
	if w != nil {
		w.Reset()
	}
	d.input = messageReader{}
	return err
}

// Restore the stripped sync-flush tail, then terminate the DEFLATE stream.
var inflateTail = [...]byte{0, 0, 0xff, 0xff, 1, 0, 0, 0xff, 0xff}

// messageReader feeds the inflater the message payload, then the tail. It
// implements io.ByteReader so the inflater does not wrap it in a bufio.Reader.
type messageReader struct {
	payload []byte
	tail    []byte
}

func (r *messageReader) exhausted() bool {
	return len(r.payload) == 0 && len(r.tail) == 0
}

func (r *messageReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.payload) != 0 {
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
