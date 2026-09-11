//go:build !386 && !amd64 && !arm64 && !wasm

package ews

import "encoding/binary"

func mask(dst, src []byte, key [4]byte, offset uint8) {
	key = rotateMaskKey(key, offset)
	dst = dst[:len(src)]

	k32 := binary.LittleEndian.Uint32(key[:])
	k64 := uint64(k32) | uint64(k32)<<32

	for len(src) >= 64 {
		binary.LittleEndian.PutUint64(dst[0:8], binary.LittleEndian.Uint64(src[0:8])^k64)
		binary.LittleEndian.PutUint64(dst[8:16], binary.LittleEndian.Uint64(src[8:16])^k64)
		binary.LittleEndian.PutUint64(dst[16:24], binary.LittleEndian.Uint64(src[16:24])^k64)
		binary.LittleEndian.PutUint64(dst[24:32], binary.LittleEndian.Uint64(src[24:32])^k64)
		binary.LittleEndian.PutUint64(dst[32:40], binary.LittleEndian.Uint64(src[32:40])^k64)
		binary.LittleEndian.PutUint64(dst[40:48], binary.LittleEndian.Uint64(src[40:48])^k64)
		binary.LittleEndian.PutUint64(dst[48:56], binary.LittleEndian.Uint64(src[48:56])^k64)
		binary.LittleEndian.PutUint64(dst[56:64], binary.LittleEndian.Uint64(src[56:64])^k64)
		src, dst = src[64:], dst[64:]
	}
	if len(src) >= 32 {
		binary.LittleEndian.PutUint64(dst[0:8], binary.LittleEndian.Uint64(src[0:8])^k64)
		binary.LittleEndian.PutUint64(dst[8:16], binary.LittleEndian.Uint64(src[8:16])^k64)
		binary.LittleEndian.PutUint64(dst[16:24], binary.LittleEndian.Uint64(src[16:24])^k64)
		binary.LittleEndian.PutUint64(dst[24:32], binary.LittleEndian.Uint64(src[24:32])^k64)
		src, dst = src[32:], dst[32:]
	}
	if len(src) >= 16 {
		binary.LittleEndian.PutUint64(dst[0:8], binary.LittleEndian.Uint64(src[0:8])^k64)
		binary.LittleEndian.PutUint64(dst[8:16], binary.LittleEndian.Uint64(src[8:16])^k64)
		src, dst = src[16:], dst[16:]
	}
	if len(src) >= 8 {
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
