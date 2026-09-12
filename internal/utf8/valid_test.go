package utf8_test

import (
	"bytes"
	"strconv"
	"testing"
	stdutf8 "unicode/utf8"

	"github.com/33TU/ews/internal/utf8"
)

func TestValidMatchesStandard(t *testing.T) {
	state := uint64(0x9e3779b97f4a7c15)
	for length := 0; length <= 512; length++ {
		for range 16 {
			src := make([]byte, length)
			for i := range src {
				state ^= state << 7
				state ^= state >> 9
				state ^= state << 8
				src[i] = byte(state)
			}
			if got, want := utf8.Valid(src), stdutf8.Valid(src); got != want {
				t.Fatalf("Valid(%x) = %v, want %v", src, got, want)
			}
		}
	}

	runes := []byte("a¢€𐍈")
	for prefix := range 32 {
		src := append(bytes.Repeat([]byte{'a'}, prefix), bytes.Repeat(runes, 40)...)
		if !utf8.Valid(src) {
			t.Fatalf("rejected valid input with prefix length %d", prefix)
		}
		for i := range src {
			corrupt := bytes.Clone(src)
			corrupt[i] = 0xff
			if utf8.Valid(corrupt) {
				t.Fatalf("accepted corruption at prefix %d, offset %d", prefix, i)
			}
		}
	}

	for _, bad := range [][]byte{
		{0xc0, 0xaf},             // Overlong.
		{0xe0, 0x80, 0x80},       // Overlong.
		{0xed, 0xa0, 0x80},       // Surrogate.
		{0xf4, 0x90, 0x80, 0x80}, // Above U+10FFFF.
		{0xf5, 0x80, 0x80, 0x80}, // Invalid start byte.
		{0xe2, 0x82},             // Truncated.
	} {
		src := append(bytes.Repeat([]byte("valid prefix "), 8), bad...)
		if utf8.Valid(src) || utf8.Valid(append(src, bytes.Repeat([]byte{'x'}, 64)...)) {
			t.Fatalf("accepted %x", bad)
		}
	}
}

var sink bool

func BenchmarkValid(b *testing.B) {
	for _, size := range []int{16, 64, 125, 384, 1024, 4096, 65536} {
		for _, kind := range []string{"ascii", "mixed", "multibyte"} {
			var src []byte
			switch kind {
			case "ascii":
				src = bytes.Repeat([]byte{'a'}, size)
			case "mixed":
				src = bytes.Repeat([]byte("hello 世界 "), size/12+1)[:size]
			case "multibyte":
				src = bytes.Repeat([]byte("世"), size/3)
			}
			for _, impl := range []struct {
				name string
				fn   func([]byte) bool
			}{{"std", stdutf8.Valid}, {"ews", utf8.Valid}} {
				b.Run(kind+"/"+strconv.Itoa(size)+"/"+impl.name, func(b *testing.B) {
					b.SetBytes(int64(len(src)))
					for b.Loop() {
						sink = impl.fn(src)
					}
				})
			}
		}
	}
}
