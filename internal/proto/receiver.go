package proto

import "github.com/33TU/ews/codec"

// Kind is the result of Receiver.Next.
type Kind uint8

const (
	// NeedInput means no complete event is available; feed more bytes.
	NeedInput Kind = iota
	// DataFrame means a validated text, binary, or continuation header was
	// accepted. Drain it with Payload before calling Next again.
	DataFrame
	// ControlFrame means a complete ping, pong, or close frame is available
	// through ControlOpcode and ControlPayload.
	ControlFrame
)

// Receiver validates incoming frames and tracks message state. Use Init.
// Calls must be serialized. Failures are terminal until Init.
type Receiver struct {
	role Role
	dec  codec.Decoder

	header     codec.Header // Most recent data frame header.
	remaining  uint64       // Unread payload of the open data frame.
	masked     bool
	maskKey    [4]byte
	maskOffset uint8

	messageOpcode codec.Opcode // Open message, or zero.
	utf8          utf8Validator

	control       [125]byte
	controlLen    uint8
	controlOpcode codec.Opcode
	controlOpen   bool

	closeReceived bool
	failed        *Error
}

// Init prepares the receiver for a connection, retaining decoder storage.
func (r *Receiver) Init(role Role) {
	dec := r.dec
	*r = Receiver{role: role, dec: dec}
	r.dec.Reset()
}

// Feed borrows input like codec.Decoder.Feed. Nothing is fed after a close or failure.
func (r *Receiver) Feed(b []byte) {
	if r.failed == nil && !r.closeReceived {
		r.dec.Feed(b)
	}
}

// Preserve copies pending input so the feed buffer can be reused.
func (r *Receiver) Preserve() { r.dec.Preserve() }

// Buffered returns fed bytes not yet consumed.
func (r *Receiver) Buffered() int { return r.dec.Buffered() }

// Idle reports whether no frame, message, or partial input is in progress.
func (r *Receiver) Idle() bool {
	return r.remaining == 0 && !r.controlOpen && r.messageOpcode == 0 && r.dec.Buffered() == 0
}

// Header borrows the most recent data frame header until the next Next call.
func (r *Receiver) Header() *codec.Header { return &r.header }

// Remaining returns unread payload bytes of the open data frame.
func (r *Receiver) Remaining() uint64 { return r.remaining }

// MessageOpen reports whether a data message has started but not finished.
func (r *Receiver) MessageOpen() bool { return r.messageOpcode != 0 }

// MessageOpcode returns the open message's opcode, or zero.
func (r *Receiver) MessageOpcode() codec.Opcode { return r.messageOpcode }

// ControlOpcode returns the opcode of the last complete control frame.
func (r *Receiver) ControlOpcode() codec.Opcode { return r.controlOpcode }

// ControlPayload borrows the last complete control frame's payload until the next call.
func (r *Receiver) ControlPayload() []byte { return r.control[:r.controlLen] }

// CloseReceived reports whether a valid close frame has been received.
func (r *Receiver) CloseReceived() bool { return r.closeReceived }

// Next advances to the next frame event. It returns codec.ErrPayloadPending
// if the open data frame has not been drained.
func (r *Receiver) Next() (Kind, error) {
	if r.failed != nil {
		return NeedInput, r.failed
	}
	if r.remaining != 0 {
		return NeedInput, codec.ErrPayloadPending
	}
	if r.closeReceived {
		return NeedInput, nil
	}
	for {
		if r.controlOpen {
			chunk, done := r.dec.Payload()
			n := copy(r.control[r.controlLen:], chunk)
			r.controlLen += uint8(n)
			if r.masked {
				r.maskOffset = codec.Mask(r.control[int(r.controlLen)-n:r.controlLen], r.maskKey, r.maskOffset)
			}
			if !done {
				return NeedInput, nil
			}
			r.controlOpen = false
			return r.finishControl()
		}

		h, ok, err := r.dec.NextHeader()
		if err != nil {
			return NeedInput, r.fail(1002, err)
		}
		if !ok {
			return NeedInput, nil
		}
		if err := r.accept(h); err != nil {
			return NeedInput, r.fail(1002, err)
		}
		r.masked, r.maskOffset = h.Masked(), 0
		if r.masked {
			copy(r.maskKey[:], h.MaskKey())
		}

		if h.Opcode() >= codec.Close {
			r.controlOpcode, r.controlLen = h.Opcode(), 0
			if h.PayloadLen() == 0 {
				return r.finishControl()
			}
			r.controlOpen = true
			continue
		}

		r.header, r.remaining = h, h.PayloadLen()
		if r.remaining == 0 {
			if err := r.finishFrame(); err != nil {
				return NeedInput, err
			}
		}
		return DataFrame, nil
	}
}

// Payload consumes available bytes of the open data frame, unmasked and
// UTF-8 checked. The chunk is borrowed until the next call. done reports
// frame completion; it is true when no frame is open.
func (r *Receiver) Payload() ([]byte, bool, error) {
	if r.failed != nil {
		return nil, false, r.failed
	}
	if r.remaining == 0 {
		return nil, true, nil
	}
	chunk, done := r.dec.Payload()
	return r.consumed(chunk, done)
}

// PayloadN is like Payload but consumes at most n bytes.
func (r *Receiver) PayloadN(n int) ([]byte, bool, error) {
	if r.failed != nil {
		return nil, false, r.failed
	}
	if r.remaining == 0 {
		return nil, true, nil
	}
	chunk, done := r.dec.PayloadN(n)
	return r.consumed(chunk, done)
}

func (r *Receiver) consumed(chunk []byte, done bool) ([]byte, bool, error) {
	if len(chunk) != 0 {
		if r.masked {
			r.maskOffset = codec.Mask(chunk, r.maskKey, r.maskOffset)
		}
		r.remaining -= uint64(len(chunk))
		if r.messageOpcode == codec.Text && !r.utf8.feed(chunk) {
			return nil, false, r.fail(1007, ErrInvalidUTF8)
		}
	}
	if done {
		if err := r.finishFrame(); err != nil {
			return nil, false, err
		}
	}
	return chunk, done, nil
}

func (r *Receiver) accept(h codec.Header) error {
	if h.Masked() != (r.role == Server) || h.RSV1() || h.RSV2() || h.RSV3() {
		return ErrProtocol
	}
	switch h.Opcode() {
	case codec.Text, codec.Binary:
		if r.messageOpcode != 0 {
			return ErrProtocol
		}
		r.messageOpcode = h.Opcode()
		r.utf8.reset()
	case codec.Continuation:
		if r.messageOpcode == 0 {
			return ErrProtocol
		}
	case codec.Close, codec.Ping, codec.Pong:
		if !h.Final() || h.PayloadLen() > 125 {
			return ErrProtocol
		}
	default:
		return ErrProtocol
	}
	return nil
}

// finishFrame closes the drained data frame and, if final, the message.
func (r *Receiver) finishFrame() error {
	if !r.header.Final() {
		return nil
	}
	if r.messageOpcode == codec.Text && !r.utf8.complete() {
		return r.fail(1007, ErrInvalidUTF8)
	}
	r.messageOpcode = 0
	return nil
}

func (r *Receiver) finishControl() (Kind, error) {
	if r.controlOpcode == codec.Close {
		if err := ValidateClose(r.control[:r.controlLen]); err != nil {
			code := uint16(1002)
			if err == ErrInvalidUTF8 {
				code = 1007
			}
			return NeedInput, r.fail(code, err)
		}
		r.closeReceived = true
		r.dec.Reset()
	}
	return ControlFrame, nil
}

func (r *Receiver) fail(code uint16, err error) error {
	r.failed = &Error{Code: code, Err: err}
	r.remaining, r.controlOpen, r.messageOpcode = 0, false, 0
	r.dec.Reset()
	return r.failed
}
