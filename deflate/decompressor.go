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
)

type resetReader interface {
	io.ReadCloser
	flate.Resetter
}

// Decompressor decompresses messages using reusable storage.
// The zero value is ready to use.
type Decompressor struct {
	// ContextTakeover retains history between messages. Set before use or after Reset.
	ContextTakeover bool

	reader  resetReader
	input   messageReader
	output  []byte
	history []byte
}

// Decompress borrows its output until the next call. maxSize must be nonnegative.
// It returns ErrMessageTooLarge if the decompressed size exceeds maxSize.
// Decode errors clear history; reset both peers before continuing with takeover.
func (d *Decompressor) Decompress(payload []byte, maxSize int) ([]byte, error) {
	if maxSize < 0 {
		return nil, ErrInvalidLimit
	}
	success := false
	defer func() {
		if !success {
			d.history = d.history[:0]
		}
	}()
	d.output = d.output[:0]
	d.input.payload = payload
	d.input.tail = inflateTail[:]
	defer func() { d.input = messageReader{} }()
	if d.reader == nil {
		d.reader = flate.NewReaderDict(&d.input, d.history).(resetReader)
	} else if err := d.reader.Reset(&d.input, d.history); err != nil {
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
		if d.ContextTakeover {
			d.remember(d.output[len(d.output)-n:])
		}
		if err == io.EOF {
			if len(d.input.payload) == 0 && len(d.input.tail) == 0 {
				success = true
				return d.output, nil
			}
			// RFC 7692 permits final DEFLATE blocks within a message.
			dict := d.output[max(0, len(d.output)-(32<<10)):]
			if d.ContextTakeover {
				dict = d.history
			}
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

// Reset clears message history and output, retaining storage and configuration.
func (d *Decompressor) Reset() {
	d.history = d.history[:0]
	d.output = d.output[:0]
	d.input = messageReader{}
}

func (d *Decompressor) remember(p []byte) {
	const window = 32 << 10
	if len(p) == 0 {
		return
	}
	if cap(d.history) == 0 {
		d.history = make([]byte, 0, window)
	}
	if len(p) >= window {
		d.history = append(d.history[:0], p[len(p)-window:]...)
		return
	}
	if n := len(d.history) + len(p) - window; n > 0 {
		d.history = d.history[:copy(d.history, d.history[n:])]
	}
	d.history = append(d.history, p...)
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
