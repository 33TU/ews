package bufpool

import "testing"

func TestGetCapacity(t *testing.T) {
	for _, n := range []int{1, 1 << 10, 1<<10 + 1, 3 << 20, 1 << MaxBits, 1<<MaxBits + 1} {
		b := Get(n)
		if len(b) != 0 || cap(b) < n {
			t.Fatalf("Get(%d): len %d cap %d", n, len(b), cap(b))
		}
		if n <= 1<<MaxBits && cap(b)&(cap(b)-1) != 0 {
			t.Fatalf("Get(%d): cap %d is not a power of two", n, cap(b))
		}
	}
}

func TestReuse(t *testing.T) {
	// Pools may drop entries at any time, so this is a best effort check
	// that a returned buffer comes back for its class.
	b := Get(3 << 20)
	b = append(b, 1, 2, 3)
	Put(b)
	for range 100 {
		if c := Get(3 << 20); &c[:1][0] == &b[:1][0] {
			return
		}
	}
	t.Skip("pool did not return the buffer")
}

func TestPutIgnoresForeign(t *testing.T) {
	Put(make([]byte, 0, 3000)) // Not a power of two.
	Put(make([]byte, 0, 512))  // Below the smallest class.
	Put(make([]byte, 0, 1<<MaxBits<<1))
	if got := Get(3000); cap(got) != 4096 {
		t.Fatalf("Get(3000): cap %d, want 4096", cap(got))
	}
}
