//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package ews

import (
	"encoding/binary"
	"simd/archsimd"
)

func mask(dst, src []byte, key [4]byte) {
	dst = dst[:len(src)]

	if len(src) >= 512 {
		k := [16]byte{
			key[0], key[1], key[2], key[3], key[0], key[1], key[2], key[3],
			key[0], key[1], key[2], key[3], key[0], key[1], key[2], key[3],
		}
		vkey := archsimd.LoadUint8x16Array(&k)
		for len(src) >= 64 {
			archsimd.LoadUint8x16(src[0:16]).Xor(vkey).Store(dst[0:16])
			archsimd.LoadUint8x16(src[16:32]).Xor(vkey).Store(dst[16:32])
			archsimd.LoadUint8x16(src[32:48]).Xor(vkey).Store(dst[32:48])
			archsimd.LoadUint8x16(src[48:64]).Xor(vkey).Store(dst[48:64])
			src, dst = src[64:], dst[64:]
		}
	}

	k32 := binary.LittleEndian.Uint32(key[:])
	k64 := uint64(k32) | uint64(k32)<<32

	for len(src) >= 32 {
		binary.LittleEndian.PutUint64(dst[0:8], binary.LittleEndian.Uint64(src[0:8])^k64)
		binary.LittleEndian.PutUint64(dst[8:16], binary.LittleEndian.Uint64(src[8:16])^k64)
		binary.LittleEndian.PutUint64(dst[16:24], binary.LittleEndian.Uint64(src[16:24])^k64)
		binary.LittleEndian.PutUint64(dst[24:32], binary.LittleEndian.Uint64(src[24:32])^k64)
		src, dst = src[32:], dst[32:]
	}

	for len(src) >= 8 {
		binary.LittleEndian.PutUint64(dst[:8], binary.LittleEndian.Uint64(src[:8])^k64)
		src, dst = src[8:], dst[8:]
	}

	if len(src) >= 4 {
		binary.LittleEndian.PutUint32(dst[:4], binary.LittleEndian.Uint32(src[:4])^k32)
		src, dst = src[4:], dst[4:]
	}

	for i, v := range src {
		dst[i] = v ^ key[i]
	}
}
