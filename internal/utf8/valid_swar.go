//go:build !(goexperiment.simd && amd64)

package utf8

import "unicode/utf8"

// Valid reports whether src is entirely valid UTF-8. The ASCII prefix is
// skipped with word loads; the rest runs through a shift-based DFA, one table
// load and one shift per byte with no branches on the byte value.
func Valid(src []byte) bool {
	const high = 0x8080808080808080
	i := asciiPrefix(src)
	if i == len(src) {
		return true
	}
	if len(src)-i < 16 {
		return utf8.Valid(src[i:])
	}

	var state uint64
	for i+8 <= len(src) {
		// Back in the accepting state, skip ASCII in words again.
		if state&63 == accept && word(src[i:i+8:i+8])&high == 0 {
			i += 8
			continue
		}
		s := src[i : i+8 : i+8]
		state = dfa[s[0]] >> (state & 63)
		state = dfa[s[1]] >> (state & 63)
		state = dfa[s[2]] >> (state & 63)
		state = dfa[s[3]] >> (state & 63)
		state = dfa[s[4]] >> (state & 63)
		state = dfa[s[5]] >> (state & 63)
		state = dfa[s[6]] >> (state & 63)
		state = dfa[s[7]] >> (state & 63)
		if state&63 == reject {
			return false
		}
		i += 8
	}
	for ; i < len(src); i++ {
		state = dfa[src[i]] >> (state & 63)
	}
	return state&63 == accept
}

// DFA states, as shift offsets into a table entry. Each entry holds the next
// state for every current state, six bits per state, so the transition is
// dfa[b] >> state.
const (
	accept  = 0  // Start of a sequence.
	cont1   = 6  // One continuation byte (80-BF) left.
	cont2   = 12 // Two continuation bytes left.
	afterE0 = 18 // A0-BF, then one continuation. Rejects overlong forms.
	afterED = 24 // 80-9F, then one continuation. Rejects surrogates.
	afterF0 = 30 // 90-BF, then two continuations. Rejects overlong forms.
	afterF1 = 36 // 80-BF, then two continuations.
	afterF4 = 42 // 80-8F, then two continuations. Rejects above U+10FFFF.
	reject  = 48 // Absorbing.
)

var dfa = func() (table [256]uint64) {
	states := []uint64{accept, cont1, cont2, afterE0, afterED, afterF0, afterF1, afterF4, reject}
	for b := range 256 {
		for _, s := range states {
			table[b] |= transition(s, byte(b)) << s
		}
	}
	return table
}()

func transition(state uint64, b byte) uint64 {
	in := func(lo, hi byte) bool { return b >= lo && b <= hi }
	switch state {
	case accept:
		switch {
		case b < 0x80:
			return accept
		case in(0xc2, 0xdf):
			return cont1
		case b == 0xe0:
			return afterE0
		case b == 0xed:
			return afterED
		case in(0xe1, 0xef):
			return cont2
		case b == 0xf0:
			return afterF0
		case in(0xf1, 0xf3):
			return afterF1
		case b == 0xf4:
			return afterF4
		}
	case cont1:
		if in(0x80, 0xbf) {
			return accept
		}
	case cont2:
		if in(0x80, 0xbf) {
			return cont1
		}
	case afterE0:
		if in(0xa0, 0xbf) {
			return cont1
		}
	case afterED:
		if in(0x80, 0x9f) {
			return cont1
		}
	case afterF0:
		if in(0x90, 0xbf) {
			return cont2
		}
	case afterF1:
		if in(0x80, 0xbf) {
			return cont2
		}
	case afterF4:
		if in(0x80, 0x8f) {
			return cont2
		}
	}
	return reject
}
