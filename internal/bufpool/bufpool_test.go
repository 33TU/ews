package bufpool

import "testing"

func TestGetCapacity(t *testing.T) {
	for _, n := range []int{1, 1 << 10, 1<<10 + 1, 3 << 20, 1 << MaxBits, 1<<MaxBits + 1} {
		b := Get(n)
		if len(b.B) != 0 || cap(b.B) < n {
			t.Fatalf("Get(%d): len %d cap %d", n, len(b.B), cap(b.B))
		}
		if n <= 1<<MaxBits && cap(b.B)&(cap(b.B)-1) != 0 {
			t.Fatalf("Get(%d): cap %d is not a power of two", n, cap(b.B))
		}
	}
}

func TestReuse(t *testing.T) {
	// Pools may drop entries at any time, so this is a best effort check
	// that a returned buffer comes back for its class, emptied.
	b := Get(3 << 20)
	b.B = append(b.B, 1, 2, 3)
	Put(b)
	for range 100 {
		if c := Get(3 << 20); c == b {
			if len(c.B) != 0 {
				t.Fatalf("reused buffer has len %d", len(c.B))
			}
			return
		}
	}
	t.Skip("pool did not return the buffer")
}

func TestPutIgnoresForeign(t *testing.T) {
	Put(nil)
	Put(&Buffer{B: make([]byte, 0, 3000)}) // Not a power of two.
	Put(&Buffer{B: make([]byte, 0, 512)})  // Below the smallest class.
	Put(&Buffer{B: make([]byte, 0, 1<<MaxBits<<1)})
	if got := Get(3000); cap(got.B) != 4096 {
		t.Fatalf("Get(3000): cap %d, want 4096", cap(got.B))
	}
}

func TestPutDoesNotAllocate(t *testing.T) {
	b := Get(1 << 20)
	if n := testing.AllocsPerRun(100, func() { Put(b); b = Get(1 << 20) }); n != 0 {
		t.Fatalf("Put and Get of a pooled buffer allocate %.1f times per pair", n)
	}
}
