package codec

import (
	"encoding/binary"
	"errors"
	"slices"
)

var (
	ErrInvalidOpcode          = errors.New("ews: invalid opcode")
	ErrInvalidControlFrame    = errors.New("ews: invalid control frame")
	ErrInvalidCompressedFrame = errors.New("ews: only data frames may be compressed")
)

// Encoder prepares WebSocket frames. The zero value is ready to use.
type Encoder struct {
	h       Header
	payload []byte
	scratch []byte // Reusable storage for masked payloads.
}

// Reset clears the frame, retaining scratch capacity.
func (e *Encoder) Reset() {
	e.h = Header{}
	e.payload = nil
	e.scratch = e.scratch[:0]
}

// HeaderBytes borrows the header until the next Encode or Reset.
func (e *Encoder) HeaderBytes() []byte {
	return e.h.Bytes()
}

// PayloadBytes borrows the payload until the next Encode or Reset.
func (e *Encoder) PayloadBytes() []byte {
	return e.payload
}

// EncodeCompressed encodes precompressed message data, setting RSV1 on its first frame.
// Continuation frames leave RSV1 clear. Control frames are rejected.
func (e *Encoder) EncodeCompressed(final bool, opcode Opcode, payload []byte, key *[4]byte) error {
	if opcode != Text && opcode != Binary && opcode != Continuation {
		return ErrInvalidCompressedFrame
	}
	if err := e.Encode(final, opcode, payload, key); err != nil {
		return err
	}
	if opcode != Continuation {
		e.h.raw[0] |= 0x40
	}
	return nil
}

// Encode borrows payload if key is nil; otherwise it masks a copy in scratch.
// Errors leave the frame unchanged.
func (e *Encoder) Encode(final bool, opcode Opcode, payload []byte, key *[4]byte) error {
	switch opcode {
	case Continuation, Text, Binary:
	case Close, Ping, Pong:
		if !final || len(payload) > 125 || (opcode == Close && len(payload) == 1) {
			return ErrInvalidControlFrame
		}
	default:
		return ErrInvalidOpcode
	}

	var h Header
	h.raw[0] = byte(opcode)
	if final {
		h.raw[0] |= 0x80
	}

	switch n := len(payload); {
	case n <= 125:
		h.raw[1] = byte(n)
		h.len = 2
	case n <= 65535:
		h.raw[1] = 126
		binary.BigEndian.PutUint16(h.raw[2:4], uint16(n))
		h.len = 4
	default:
		h.raw[1] = 127
		binary.BigEndian.PutUint64(h.raw[2:10], uint64(n))
		h.len = 10
	}

	var maskKey [4]byte
	if key != nil {
		maskKey = *key
		copy(h.raw[h.len:], maskKey[:])
		h.raw[1] |= 0x80
		h.len += 4
	}

	if key == nil {
		e.payload = payload
	} else {
		e.scratch = slices.Grow(e.scratch[:0], len(payload))[:len(payload)]
		mask(e.scratch, payload, maskKey, 0)
		e.payload = e.scratch
	}
	e.h = h

	return nil
}
