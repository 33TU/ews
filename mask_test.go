package ews

import (
	"bytes"
	"fmt"
	"testing"
)

func maskScalar(dst, src []byte, key [4]byte) {
	for i, v := range src {
		dst[i] = v ^ key[i&3]
	}
}

func TestMask(t *testing.T) {
	for _, key := range [][4]byte{{}, {0xff, 0xff, 0xff, 0xff}, {0x37, 0xfa, 0x21, 0x3d}} {
		for n := 0; n <= 1025; n++ {
			for offset := 0; offset < 16; offset++ {
				input := make([]byte, n+32)
				for i := range input {
					input[i] = byte(i*37 + n)
				}
				src := input[offset : offset+n : offset+n]
				original := bytes.Clone(input)
				want := make([]byte, n)
				maskScalar(want, src, key)
				storage := bytes.Repeat([]byte{0xa5}, n+32)
				start := 15 - offset
				dst := storage[start : start+n : start+n]
				mask(dst, src, key, 0)
				if !bytes.Equal(dst, want) || !bytes.Equal(input, original) {
					t.Fatalf("size=%d offset=%d: incorrect output or modified source", n, offset)
				}
				for i, v := range storage {
					if (i < start || i >= start+n) && v != 0xa5 {
						t.Fatalf("size=%d offset=%d: wrote past destination", n, offset)
					}
				}
				mask(src, src, key, 0)
				if !bytes.Equal(src, want) {
					t.Fatal("in-place masking failed")
				}
				mask(src, src, key, 0)
				if !bytes.Equal(input, original) {
					t.Fatal("unmasking failed")
				}
			}
		}
	}
}

func BenchmarkMask(b *testing.B) {
	for _, size := range []int{8, 125, 256, 384, 511, 512, 513, 4096, 65536} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			payload := make([]byte, size)
			key := [4]byte{0x37, 0xfa, 0x21, 0x3d}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				Mask(payload, key, 0)
			}
		})
	}
}

func TestMaskPublic(t *testing.T) {
	key := [4]byte{0x37, 0xfa, 0x21, 0x3d}
	for _, n := range []int{0, 1, 3, 4, 7, 16, 63, 64, 125, 511, 512, 513, 1025} {
		for offset := 0; offset < 256; offset++ {
			original := bytes.Repeat([]byte{0xab}, n)
			payload := bytes.Clone(original)
			next := Mask(payload, key, uint8(offset))
			if want := uint8((offset + n) % 4); next != want {
				t.Fatalf("size=%d offset=%d: next=%d, want %d", n, offset, next, want)
			}
			for i, v := range payload {
				if want := original[i] ^ key[(offset+i)%4]; v != want {
					t.Fatalf("size=%d offset=%d: incorrect byte %d", n, offset, i)
				}
			}
			Mask(payload, key, uint8(offset))
			if !bytes.Equal(payload, original) {
				t.Fatal("unmasking failed")
			}
		}
	}
}

func TestMaskChunks(t *testing.T) {
	key := [4]byte{0x37, 0xfa, 0x21, 0x3d}
	original := make([]byte, 1031)
	for i := range original {
		original[i] = byte(i * 37)
	}
	want := make([]byte, len(original))
	maskScalar(want, original, key)
	for split := 0; split <= len(original); split++ {
		payload := bytes.Clone(original)
		offset := Mask(payload[:split], key, 0)
		offset = Mask(nil, key, offset)
		offset = Mask(payload[split:], key, offset)
		if !bytes.Equal(payload, want) || offset != uint8(len(payload)%4) {
			t.Fatalf("incorrect chunked masking at split %d", split)
		}
	}
}
