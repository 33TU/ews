package proto

import "unicode/utf8"

// utf8Validator checks UTF-8 across chunk boundaries by carrying an
// incomplete trailing sequence to the next call.
type utf8Validator struct {
	pending [utf8.UTFMax]byte
	n       uint8
}

func (v *utf8Validator) reset() { v.n = 0 }

// complete reports whether no partial sequence is pending.
func (v *utf8Validator) complete() bool { return v.n == 0 }

// feed validates p given what came before it.
func (v *utf8Validator) feed(p []byte) bool {
	for v.n != 0 && len(p) != 0 {
		v.pending[v.n] = p[0]
		v.n++
		p = p[1:]
		if utf8.FullRune(v.pending[:v.n]) {
			if r, size := utf8.DecodeRune(v.pending[:v.n]); r == utf8.RuneError && size == 1 {
				return false
			}
			v.n = 0
		}
	}
	if v.n != 0 {
		return true // Input ended inside the pending sequence.
	}
	tail := incompleteTail(p)
	if !utf8.Valid(p[:len(p)-tail]) {
		return false
	}
	v.n = uint8(copy(v.pending[:], p[len(p)-tail:]))
	return true
}

// incompleteTail returns the length of a trailing sequence that later input
// could still complete, or zero. Sequences that are already invalid return
// zero so utf8.Valid rejects them now.
func incompleteTail(p []byte) int {
	for i := 1; i < utf8.UTFMax && i <= len(p); i++ {
		b := p[len(p)-i]
		if b < 0x80 {
			return 0
		}
		if b < 0xC0 {
			continue // Continuation byte; keep looking for the start.
		}
		if b < 0xC2 || b > 0xF4 {
			return 0
		}
		need := 2
		if b >= 0xE0 {
			need = 3
		}
		if b >= 0xF0 {
			need = 4
		}
		if need > i {
			return i
		}
		return 0
	}
	return 0
}
