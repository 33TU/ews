package proto

import (
	"math/rand/v2"
	"testing"
	"unicode/utf8"
)

func TestUTF8Incremental(t *testing.T) {
	samples := [][]byte{
		[]byte("plain ascii"),
		[]byte("A😀B€C"),
		[]byte("κόσμε"),
		{0xff},
		{0xc0, 0xaf},                   // Overlong.
		{0xe0, 0x80, 0x80},             // Overlong.
		{0xed, 0xa0, 0x80},             // Surrogate.
		{0xf4, 0x90, 0x80, 0x80},       // Above U+10FFFF.
		{0xf5, 0x80, 0x80, 0x80},       // Invalid start byte.
		{0x80, 0x80, 0x80, 0x80},       // Continuations only.
		{0xe2, 0x82},                   // Incomplete at end.
		{0xf0, 0x9f, 0x98},             // Incomplete at end.
		{0xe2, 0x82, 0xac, 0xe2, 0x82}, // Valid then incomplete.
		{0x41, 0xe2, 0x41},             // Truncated in the middle.
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for _, s := range samples {
		want := utf8.Valid(s)
		for round := 0; round < 200; round++ {
			var v utf8Validator
			ok := true
			for off := 0; off < len(s) && ok; {
				n := 1 + rng.IntN(len(s)-off)
				if rng.IntN(4) == 0 {
					n = len(s) - off
				}
				ok = v.feed(s[off : off+n])
				off += n
			}
			ok = ok && v.complete()
			if ok != want {
				t.Fatalf("%x: incremental %v, want %v", s, ok, want)
			}
		}
	}
}

func TestUTF8FailsEarly(t *testing.T) {
	var v utf8Validator
	if v.feed([]byte{0xff}) {
		t.Fatal("invalid start byte accepted")
	}
	v.reset()
	if !v.feed([]byte{0xe2}) || v.complete() {
		t.Fatal("incomplete prefix must be pending")
	}
	if v.feed([]byte{0x41}) {
		t.Fatal("broken sequence accepted")
	}
}
