package codec

// Mask masks or unmasks payload in place, returning the next offset modulo four.
// Start each frame at offset zero and carry the returned offset between chunks.
func Mask(payload []byte, key [4]byte, offset uint8) uint8 {
	mask(payload, payload, key, offset)
	return (offset + uint8(len(payload)&3)) & 3
}

func rotateMaskKey(key [4]byte, offset uint8) [4]byte {
	offset &= 3
	if offset != 0 {
		key = [4]byte{key[offset], key[(offset+1)&3], key[(offset+2)&3], key[(offset+3)&3]}
	}
	return key
}
