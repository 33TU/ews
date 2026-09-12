package codec_test

import (
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
)

func BenchmarkEncoderEncode(b *testing.B) {
	for _, size := range []int{0, 125, 126, 4096, 65536} {
		for _, masked := range []bool{false, true} {
			b.Run(fmt.Sprintf("size=%d/masked=%t", size, masked), func(b *testing.B) {
				payload := make([]byte, size)
				var key *[4]byte
				if masked {
					key = &[4]byte{1, 2, 3, 4}
				}
				var e codec.Encoder
				if err := e.Encode(true, codec.Binary, payload, key); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				if masked {
					b.SetBytes(int64(size))
				}
				for b.Loop() {
					if err := e.Encode(true, codec.Binary, payload, key); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
