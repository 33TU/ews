// Package bufpool pools byte slices by power-of-two capacity, so a buffer
// that grew for one large message serves the next of its size instead of
// being dropped, and never sits in a pool meant for small ones.
package bufpool

import (
	"math/bits"
	"sync"
)

const (
	// MinBits is the smallest class, 1 KiB; smaller requests get it.
	MinBits = 10
	// MaxBits is the largest class, 64 MiB; larger buffers are not pooled.
	MaxBits = 26
)

// Buffer is a pooled slice. B is the working slice, extended within its
// capacity; the holder keeps the Buffer and gives it back with Put.
type Buffer struct{ B []byte }

var classes [MaxBits + 1]sync.Pool

// class is the smallest c with 1<<c >= n, at least MinBits.
func class(n int) int {
	return max(MinBits, bits.Len(uint(n-1)))
}

// Get returns a Buffer whose B has length 0 and capacity at least n, a
// power of two when pooled. Contents are whatever the previous holder left.
func Get(n int) *Buffer {
	if n > 1<<MaxBits {
		return &Buffer{B: make([]byte, 0, n)}
	}
	c := class(n)
	if b, ok := classes[c].Get().(*Buffer); ok {
		b.B = b.B[:0]
		return b
	}
	return &Buffer{B: make([]byte, 0, 1<<c)}
}

// Put returns b to the class of its capacity. nil is ignored, and a
// capacity that is not a power of two in range did not come from Get and
// is left to the GC.
func Put(b *Buffer) {
	if b == nil {
		return
	}
	c := cap(b.B)
	if c < 1<<MinBits || c > 1<<MaxBits || c&(c-1) != 0 {
		return
	}
	b.B = b.B[:0]
	classes[bits.Len(uint(c))-1].Put(b)
}
