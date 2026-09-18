//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package codec

import (
	"simd/archsimd"
	"unsafe"
)

// simdMin is the payload size from which the vector loop pays for itself.
// Measured against maskSWAR in the same build: it loses at 512 bytes, ties
// at 768, and wins from 1 KiB up, by 7 percent at 1 KiB, 15 at 2 KiB, 10
// at 16 KiB and 25 at 64 KiB.
const simdMin = 1024

func mask(dst, src []byte, key [4]byte, offset uint8) {
	if len(src) >= simdMin {
		maskVector(dst, src, key, offset)
		return
	}
	maskSWAR(dst, src, key, offset)
}

// maskVector runs the 16-byte vector loop over 64-byte blocks and hands the
// remainder to maskSWAR; blocks are multiples of four bytes, so the key's
// rotation is unchanged for the tail.
func maskVector(dst, src []byte, key [4]byte, offset uint8) {
	rk := rotateMaskKey(key, offset)
	dst = dst[:len(src)]
	n := len(src)
	k32 := *(*uint32)(unsafe.Pointer(&rk))
	k64 := uint64(k32) | uint64(k32)<<32
	s := unsafe.Pointer(unsafe.SliceData(src))
	d := unsafe.Pointer(unsafe.SliceData(dst))
	k := [2]uint64{k64, k64}
	vkey := archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Pointer(&k)))
	i := 0
	for n-i >= 64 {
		archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+0))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+0)))
		archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+16))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+16)))
		archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+32))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+32)))
		archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+48))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+48)))
		i += 64
	}
	if i < n {
		maskSWAR(dst[i:], src[i:], key, offset)
	}
}
