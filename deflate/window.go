package deflate

// Window is one direction's compression history for context takeover: the
// most recent 32 KB of payload sent or received on a connection. Give each
// direction of each connection its own Window and pass it to every call for
// that direction; nil means no context takeover. The zero value is ready to
// use. A failed message clears the window, since the peers' histories have
// diverged.
type Window struct {
	buf []byte
	gen uint64 // Bumped on every change so a compressor can tell whether it may continue.
}

const windowSize = 32 << 10

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
	if cap(w.buf) == 0 {
		w.buf = make([]byte, 0, windowSize)
	}
	if len(p) >= windowSize {
		w.buf = append(w.buf[:0], p[len(p)-windowSize:]...)
		return
	}
	if n := len(w.buf) + len(p) - windowSize; n > 0 {
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
