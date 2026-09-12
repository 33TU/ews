package codec

import (
	"bytes"
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
