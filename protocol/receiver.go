package protocol

import (
	"errors"
	"unicode/utf8"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
)

// ReceiverConfig describes the local endpoint and negotiated receive direction.
type ReceiverConfig struct {
	Role Role
	// MaxMessageSize bounds wire and decompressed message data. Zero uses 8 MiB.
	MaxMessageSize  int
	Compression     bool
	ContextTakeover bool
}

// Receiver assembles and validates incoming messages. Use NewReceiver.
// Calls on the same receiver must be serialized.
type Receiver struct {
	role          Role
	limit         int
	decoder       codec.Decoder
	decompressor  *deflate.Decompressor
	header        codec.Header
	reading       bool
	maskKey       [4]byte
	maskOffset    uint8
	messageOpcode codec.Opcode
	compressed    bool
	message       []byte
	control       []byte
	closeReceived bool
	failed        *Error
}

func NewReceiver(config ReceiverConfig) (*Receiver, error) {
	if config.Role != Server && config.Role != Client || config.MaxMessageSize < 0 || config.ContextTakeover && !config.Compression {
		return nil, ErrInvalidConfig
	}
	r := &Receiver{role: config.Role, limit: config.MaxMessageSize}
	if r.limit == 0 {
		r.limit = DefaultMaxMessageSize
	}
	if config.Compression {
		r.decompressor = &deflate.Decompressor{ContextTakeover: config.ContextTakeover}
	}
	return r, nil
}

// Reset clears connection state, retaining storage and configuration.
func (r *Receiver) Reset() {
	r.decoder.Reset()
	if r.decompressor != nil {
		r.decompressor.Reset()
	}
	r.header = codec.Header{}
	r.reading = false
	r.maskKey, r.maskOffset = [4]byte{}, 0
	r.messageOpcode, r.compressed = 0, false
	r.message, r.control = r.message[:0], r.control[:0]
	r.closeReceived, r.failed = false, nil
}

// Feed borrows input without modifying it. Keep it unchanged until consumed or
// the next Feed returns. Drain NextEvent before feeding more input.
func (r *Receiver) Feed(input []byte) {
	if r.failed == nil && !r.closeReceived {
		r.decoder.Feed(input)
	}
}

// CloseReceived reports whether a valid close frame has been received.
func (r *Receiver) CloseReceived() bool { return r.closeReceived }

// NextEvent returns the next message or control frame; ok is false if incomplete.
// The caller handles ping and close replies. Errors are terminal until Reset.
func (r *Receiver) NextEvent() (Event, bool, error) {
	if r.failed != nil {
		return Event{}, false, r.failed
	}
	if r.closeReceived {
		return Event{}, false, nil
	}
	for {
		if !r.reading {
			h, ok, err := r.decoder.NextHeader()
			if err != nil {
				return r.fail(1002, err)
			}
			if !ok {
				return Event{}, false, nil
			}
			if err := r.startFrame(h); err != nil {
				code := uint16(1002)
				if errors.Is(err, ErrMessageTooLarge) {
					code = 1009
				}
				return r.fail(code, err)
			}
		}
		chunk, done := r.decoder.Payload()
		dst := &r.message
		if r.header.Opcode() >= codec.Close {
			dst = &r.control
		}
		start := len(*dst)
		*dst = append(*dst, chunk...)
		if r.header.Masked() {
			r.maskOffset = codec.Mask((*dst)[start:], r.maskKey, r.maskOffset)
		}
		if !done {
			return Event{}, false, nil
		}
		opcode := r.header.Opcode()
		if opcode == codec.Close {
			if err := validateClose(r.control); err != nil {
				code := uint16(1002)
				if errors.Is(err, ErrInvalidUTF8) {
					code = 1007
				}
				return r.fail(code, err)
			}
			r.closeReceived = true
			r.decoder.Reset()
		}
		r.reading = false
		if opcode >= codec.Close {
			return Event{Opcode: opcode, Payload: r.control}, true, nil
		}
		if !r.header.Final() {
			continue
		}
		payload := r.message
		if r.compressed {
			var err error
			payload, err = r.decompressor.Decompress(payload, r.limit)
			if err != nil {
				if errors.Is(err, deflate.ErrMessageTooLarge) {
					return r.fail(1009, ErrMessageTooLarge)
				}
				return r.fail(1007, err)
			}
		}
		if r.messageOpcode == codec.Text && !utf8.Valid(payload) {
			return r.fail(1007, ErrInvalidUTF8)
		}
		event := Event{Opcode: r.messageOpcode, Payload: payload}
		r.messageOpcode = 0
		return event, true, nil
	}
}

func (r *Receiver) fail(code uint16, err error) (Event, bool, error) {
	r.failed = &Error{Code: code, Err: err}
	r.reading = false
	r.decoder.Reset()
	return Event{}, false, r.failed
}

func (r *Receiver) startFrame(h codec.Header) error {
	if h.Masked() != (r.role == Server) || h.RSV2() || h.RSV3() {
		return ErrProtocol
	}
	switch h.Opcode() {
	case codec.Text, codec.Binary:
		if r.messageOpcode != 0 || h.RSV1() && r.decompressor == nil {
			return ErrProtocol
		}
		r.messageOpcode, r.compressed = h.Opcode(), h.RSV1()
		r.message = r.message[:0]
	case codec.Continuation:
		if r.messageOpcode == 0 || h.RSV1() {
			return ErrProtocol
		}
	case codec.Close, codec.Ping, codec.Pong:
		if !h.Final() || h.RSV1() || h.PayloadLen() > 125 {
			return ErrProtocol
		}
		r.control = r.control[:0]
	default:
		return ErrProtocol
	}
	if h.Opcode() < codec.Close && h.PayloadLen() > uint64(r.limit-len(r.message)) {
		return ErrMessageTooLarge
	}
	r.header, r.reading = h, true
	r.maskKey, r.maskOffset = [4]byte{}, 0
	if h.Masked() {
		copy(r.maskKey[:], h.MaskKey())
	}
	return nil
}
