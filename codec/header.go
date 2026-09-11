package codec

import "encoding/binary"

// MaxHeaderSize is the maximum encoded header size.
const MaxHeaderSize = 14

// Header stores a WebSocket frame header. Accessors assume valid input.
type Header struct {
	raw [MaxHeaderSize]byte
	len uint8
}

// Bytes borrows the encoded header bytes.
func (h *Header) Bytes() []byte {
	return h.raw[:h.len]
}

// Len returns the encoded header length in bytes.
func (h *Header) Len() int {
	return int(h.len)
}

// PayloadLen returns the payload size in bytes.
func (h *Header) PayloadLen() uint64 {
	switch n := h.raw[1] & 0x7f; n {
	case 126:
		return uint64(binary.BigEndian.Uint16(h.raw[2:4]))
	case 127:
		return binary.BigEndian.Uint64(h.raw[2:10])
	default:
		return uint64(n)
	}
}

// Final reports whether this is the final fragment of a message.
func (h *Header) Final() bool {
	return h.raw[0]&0x80 != 0
}

// RSV1 reports whether the first reserved bit is set.
func (h *Header) RSV1() bool {
	return h.raw[0]&0x40 != 0
}

// RSV2 reports whether the second reserved bit is set.
func (h *Header) RSV2() bool {
	return h.raw[0]&0x20 != 0
}

// RSV3 reports whether the third reserved bit is set.
func (h *Header) RSV3() bool {
	return h.raw[0]&0x10 != 0
}

// Opcode returns the frame's opcode.
func (h *Header) Opcode() Opcode {
	return Opcode(h.raw[0] & 0x0f)
}

// Masked reports whether the payload is masked.
func (h *Header) Masked() bool {
	return h.raw[1]&0x80 != 0
}

// MaskKey borrows the four-byte masking key, or returns nil if unmasked.
func (h *Header) MaskKey() []byte {
	if !h.Masked() {
		return nil
	}
	return h.raw[h.len-4 : h.len]
}
