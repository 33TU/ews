//go:build amd64 || arm64 || wasm || 386

package codec

import "unsafe"

// maskSWAR XORs eight bytes at a time with the key repeated into a word,
// then narrows through 32, 16, 8, 4 and single bytes for the tail. It is
// its own function so the compiler lays out the loop the same whether or
// not a SIMD path exists beside it.
func maskSWAR(dst, src []byte, key [4]byte, offset uint8) {
	key = rotateMaskKey(key, offset)
	dst = dst[:len(src)]
	n := len(src)

	k32 := *(*uint32)(unsafe.Pointer(&key))
	k64 := uint64(k32) | uint64(k32)<<32

	s := unsafe.Pointer(unsafe.SliceData(src))
	d := unsafe.Pointer(unsafe.SliceData(dst))

	i := 0
	for n-i >= 64 {
		*(*uint64)(unsafe.Add(d, i+0)) = *(*uint64)(unsafe.Add(s, i+0)) ^ k64
		*(*uint64)(unsafe.Add(d, i+8)) = *(*uint64)(unsafe.Add(s, i+8)) ^ k64
		*(*uint64)(unsafe.Add(d, i+16)) = *(*uint64)(unsafe.Add(s, i+16)) ^ k64
		*(*uint64)(unsafe.Add(d, i+24)) = *(*uint64)(unsafe.Add(s, i+24)) ^ k64
		*(*uint64)(unsafe.Add(d, i+32)) = *(*uint64)(unsafe.Add(s, i+32)) ^ k64
		*(*uint64)(unsafe.Add(d, i+40)) = *(*uint64)(unsafe.Add(s, i+40)) ^ k64
		*(*uint64)(unsafe.Add(d, i+48)) = *(*uint64)(unsafe.Add(s, i+48)) ^ k64
		*(*uint64)(unsafe.Add(d, i+56)) = *(*uint64)(unsafe.Add(s, i+56)) ^ k64
		i += 64
	}
	if n-i >= 32 {
		*(*uint64)(unsafe.Add(d, i+0)) = *(*uint64)(unsafe.Add(s, i+0)) ^ k64
		*(*uint64)(unsafe.Add(d, i+8)) = *(*uint64)(unsafe.Add(s, i+8)) ^ k64
		*(*uint64)(unsafe.Add(d, i+16)) = *(*uint64)(unsafe.Add(s, i+16)) ^ k64
		*(*uint64)(unsafe.Add(d, i+24)) = *(*uint64)(unsafe.Add(s, i+24)) ^ k64
		i += 32
	}
	if n-i >= 16 {
		*(*uint64)(unsafe.Add(d, i+0)) = *(*uint64)(unsafe.Add(s, i+0)) ^ k64
		*(*uint64)(unsafe.Add(d, i+8)) = *(*uint64)(unsafe.Add(s, i+8)) ^ k64
		i += 16
	}
	if n-i >= 8 {
		*(*uint64)(unsafe.Add(d, i+0)) = *(*uint64)(unsafe.Add(s, i+0)) ^ k64
		i += 8
	}
	if n-i >= 4 {
		*(*uint32)(unsafe.Add(d, i)) = *(*uint32)(unsafe.Add(s, i)) ^ k32
		i += 4
	}
	for ; i < n; i++ {
		*(*byte)(unsafe.Add(d, i)) = *(*byte)(unsafe.Add(s, i)) ^ key[i&3]
	}
}
