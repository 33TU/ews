//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package codec

import (
	"simd/archsimd"
	"unsafe"
)

// simdMin is the payload size from which the vector loop pays for itself.
const simdMin = 4096

func mask(dst, src []byte, key [4]byte, offset uint8) {
	key = rotateMaskKey(key, offset)
	dst = dst[:len(src)]
	n := len(src)

	k32 := *(*uint32)(unsafe.Pointer(&key))
	k64 := uint64(k32) | uint64(k32)<<32

	s := unsafe.Pointer(unsafe.SliceData(src))
	d := unsafe.Pointer(unsafe.SliceData(dst))

	i := 0
	// Below simdMin the vector loop's setup costs more than it saves: measured
	// against the SWAR loop it loses at 512 B and 2 KiB, ties at 1 KiB, and
	// wins by 4 to 8 percent from 4 KiB up, 25 percent at 64 KiB.
	if n >= simdMin {
		k := [2]uint64{k64, k64}
		vkey := archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Pointer(&k)))
		for n-i >= 64 {
			archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+0))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+0)))
			archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+16))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+16)))
			archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+32))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+32)))
			archsimd.LoadUint8x16Array((*[16]byte)(unsafe.Add(s, i+48))).Xor(vkey).StoreArray((*[16]byte)(unsafe.Add(d, i+48)))
			i += 64
		}
	}
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
