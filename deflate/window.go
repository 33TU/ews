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

func (w *Window) size() int {
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

func (w *Window) remember(p []byte) {
	if len(p) == 0 {
		return
	}
	w.gen++
	size := w.size()
	if cap(w.buf) == 0 {
		w.buf = make([]byte, 0, size)
	}
	if len(p) >= size {
		w.buf = append(w.buf[:0], p[len(p)-size:]...)
		return
	}
	if n := len(w.buf) + len(p) - size; n > 0 {
		w.buf = w.buf[:copy(w.buf, w.buf[n:])]
	}
	w.buf = append(w.buf, p...)
}

// dict returns the window content, or nil for a nil window.
func (w *Window) dict() []byte {
	if w == nil {
		return nil
	}
	return w.buf
}
