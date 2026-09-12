package codec

import "io"

// Reader reads frame headers and payload chunks using caller-provided storage.
// Masking is explicit. Use NewReader or Reset before reading.
type Reader struct {
	r   io.Reader
	d   Decoder
	err error
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

// Reset switches sources and clears frame and error state.
func (r *Reader) Reset(src io.Reader) {
	r.r = src
	r.d.Reset()
	r.err = nil
}

func (r *Reader) ReadHeader(b []byte) (Header, error) {
	for {
		h, ok, err := r.d.NextHeader()
		if ok || err != nil {
			return h, err
		}
		if r.err != nil {
			if r.err == io.EOF && len(r.d.pending) != 0 {
				return h, io.ErrUnexpectedEOF
			}
			return h, r.err
		}
		if len(b) == 0 {
			return h, io.ErrShortBuffer
		}

		r.d.Preserve()

		n, err := r.r.Read(b)
		r.d.Feed(b[:n])
		r.err = err
		if n == 0 && err == nil {
			return h, io.ErrNoProgress
		}
	}
}

// ReadPayload returns a raw payload chunk and whether the frame is complete.
// The chunk is borrowed until the next reader call or reuse of b. Masking is explicit.
func (r *Reader) ReadPayload(b []byte) (chunk []byte, done bool, err error) {
	for {
		chunk, done = r.d.Payload()
		if len(chunk) != 0 || done {
			return chunk, done, nil
		}
		if r.err != nil {
			if r.err == io.EOF {
				return nil, false, io.ErrUnexpectedEOF
			}
			return nil, false, r.err
		}
		if len(b) == 0 {
			return nil, false, io.ErrShortBuffer
		}

		n, err := r.r.Read(b)
		r.d.Feed(b[:n])
		r.err = err
		if n == 0 && err == nil {
			return nil, false, io.ErrNoProgress
		}
	}
}
