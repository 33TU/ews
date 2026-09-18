package codec_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
)

func BenchmarkMask(b *testing.B) {
	for _, size := range []int{8, 125, 256, 384, 511, 512, 513, 4096, 65536} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			payload := make([]byte, size)
			key := [4]byte{0x37, 0xfa, 0x21, 0x3d}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				codec.Mask(payload, key, 0)
			}
		})
	}
}

func TestMaskPublic(t *testing.T) {
	key := [4]byte{0x37, 0xfa, 0x21, 0x3d}
	for _, n := range []int{0, 1, 3, 4, 7, 16, 63, 64, 125, 511, 512, 513, 1025} {
		for offset := range 256 {
			original := bytes.Repeat([]byte{0xab}, n)
			payload := bytes.Clone(original)
			next := codec.Mask(payload, key, uint8(offset))
			if want := uint8((offset + n) % 4); next != want {
				t.Fatalf("size=%d offset=%d: next=%d, want %d", n, offset, next, want)
			}
			for i, v := range payload {
				if want := original[i] ^ key[(offset+i)%4]; v != want {
					t.Fatalf("size=%d offset=%d: incorrect byte %d", n, offset, i)
				}
			}
			codec.Mask(payload, key, uint8(offset))
			if !bytes.Equal(payload, original) {
				t.Fatal("unmasking failed")
			}
		}
	}
}
