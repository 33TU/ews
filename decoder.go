package ews

import "errors"

var (
	// ErrPayloadPending means Next was called before draining the payload.
	ErrPayloadPending = errors.New("ews: previous payload has not been consumed")
	// ErrInvalidPayloadLength indicates a nonminimal or out-of-range length.
	ErrInvalidPayloadLength = errors.New("ews: invalid payload length encoding")
)

type Decoder struct {
	pending   []byte
	scratch   []byte
	remaining uint64
}

// Reset clears decoder state, retaining scratch capacity.
func (d *Decoder) Reset() {
	d.pending = nil
	d.scratch = d.scratch[:0]
	d.remaining = 0
}

// Feed appends input, borrowing b when no input is pending.
func (d *Decoder) Feed(b []byte) {
	if len(b) == 0 {
		return
	}
	if len(d.pending) == 0 {
		d.pending = b
		return
	}

	d.scratch = append(d.scratch[:0], d.pending...)
	d.scratch = append(d.scratch, b...)
	d.pending = d.scratch
}

// Next reads a header; ok is false if incomplete. Errors consume nothing.
// Unread payloads and invalid lengths are errors; other validation is up to the caller.
func (d *Decoder) Next() (Header, bool, error) {
	var h Header
	if d.remaining != 0 {
		return h, false, ErrPayloadPending
	}
	if len(d.pending) < 2 {
		return h, false, nil
	}

	n := 2
	lengthCode := d.pending[1] & 0x7f
	switch lengthCode {
	case 126:
		n += 2
	case 127:
		n += 8
	}
	if d.pending[1]&0x80 != 0 {
		n += 4
	}
	if len(d.pending) < n {
		return h, false, nil
	}

	copy(h.raw[:], d.pending[:n])
	h.len = uint8(n)

	remaining := h.PayloadLen()
	if (lengthCode == 126 && remaining < 126) || (lengthCode == 127 && (remaining < 65536 || remaining>>63 != 0)) {
		return Header{}, false, ErrInvalidPayloadLength
	}
	d.pending = d.pending[n:]
	d.remaining = remaining

	return h, true, nil
}

// Payload consumes available bytes from this frame without unmasking.
// Bytes are borrowed until the next decoder call. Returns nil, false if waiting
// for input, or nil, true if no payload remains (including before the first header).
func (d *Decoder) Payload() (chunk []byte, done bool) {
	if d.remaining == 0 {
		return nil, true
	}
	if len(d.pending) == 0 {
		return nil, false
	}

	n := min(uint64(len(d.pending)), d.remaining)
	chunk = d.pending[:int(n):int(n)]
	d.pending = d.pending[int(n):]
	d.remaining -= n

	return chunk, d.remaining == 0
}
