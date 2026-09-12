package utf8

import (
	"encoding/binary"
	"unicode/utf8"
)

// asciiPrefix returns the length of the leading run of ASCII bytes, checking
// 32, 16, and 8 bytes per step before finishing byte by byte. ASCII is the
// common case, and word loads beat any per-byte validator on it.
func asciiPrefix(src []byte) int {
	const high = 0x8080808080808080
	i := 0
	for i+32 <= len(src) {
		s := src[i : i+32 : i+32]
		words := binary.LittleEndian.Uint64(s) | binary.LittleEndian.Uint64(s[8:]) |
			binary.LittleEndian.Uint64(s[16:]) | binary.LittleEndian.Uint64(s[24:])
		if words&high != 0 {
			break
		}
		i += 32
	}
	if i+16 <= len(src) {
		s := src[i : i+16 : i+16]
		if (binary.LittleEndian.Uint64(s)|binary.LittleEndian.Uint64(s[8:]))&high == 0 {
			i += 16
		}
	}
	if i+8 <= len(src) && binary.LittleEndian.Uint64(src[i:i+8:i+8])&high == 0 {
		i += 8
	}
	for i < len(src) && src[i] < utf8.RuneSelf {
		i++
	}
	return i
}

// word loads eight bytes; endianness is irrelevant for high-bit tests.
func word(b []byte) uint64 { return binary.LittleEndian.Uint64(b) }
