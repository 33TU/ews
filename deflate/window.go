package deflate

// Window is one direction's compression history for context takeover: the
// most recent 32 KB of payload sent or received on a connection. Give each
// direction of each connection its own Window and pass it to every call for
// that direction; nil means no context takeover. The zero value is ready to
// use. A failed message clears the window, since the peers' histories have
// diverged.
type Window struct {
	// Bits is the window size as a power of two, 8 to 15; zero means 15. Set
	// it to the negotiated size before use: for a send window the size this
	// endpoint compresses with, for a receive window the size the peer
	// declared. Only that much history is then kept and copied.
	Bits int

	buf []byte
	gen uint64 // Bumped on every change so a compressor can tell whether it may continue.
}

const windowSize = 32 << 10

// Size returns the window size in bytes.
func (w *Window) Size() int {
	if w.Bits >= 8 && w.Bits <= 15 {
		return 1 << w.Bits
	}
	return windowSize
}

// Add records payload as compressed or decompressed data sent through some
// other path, such as a message compressed once for many recipients, so the
// history stays in step with the peer's. A compressor that was continuing
// this window primes again on its next message.
func (w *Window) Add(payload []byte) { w.remember(payload) }

// Reset forgets the history, retaining storage.
func (w *Window) Reset() {
	w.buf = w.buf[:0]
	w.gen++
}

// remember appends p, keeping at least the last size bytes. The buffer holds
// up to twice the window and is compacted only when full, so the shift that
// keeps history contiguous costs one byte per byte appended, amortized,
// rather than a whole-window move per message.
func (w *Window) remember(p []byte) {
	if len(p) == 0 {
		return
	}
	w.gen++
	size := w.Size()
	if len(p) >= size {
		w.buf = append(w.buf[:0], p[len(p)-size:]...)
		return
	}

	if cap(w.buf) < 2*size {
		w.buf = append(make([]byte, 0, 2*size), w.dict()...)
	}
	if len(w.buf)+len(p) > cap(w.buf) {
		// Compact to the most recent bytes that, with p, still fill the window.
		keep := size - len(p)
		w.buf = w.buf[:copy(w.buf, w.buf[len(w.buf)-keep:])]
	}
	w.buf = append(w.buf, p...)
}

// dict returns the window's history, at most size bytes, or nil for a nil window.
func (w *Window) dict() []byte {
	if w == nil {
		return nil
	}
	if n := len(w.buf) - w.Size(); n > 0 {
		return w.buf[n:]
	}
	return w.buf
}
