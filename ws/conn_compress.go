package ws

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/internal/proto"
)

// Context takeover state is a 32 KB window per direction on the connection.
// Decompressors are always shared across connections. Compressors are shared
// too, except that a connection with send takeover keeps one attached so its
// messages continue one stream instead of re-priming from the window on every
// message. Attached is faster per message; shared is far smaller per
// connection and wins once connections outnumber what the cache can hold.
// Config.CompressionShared picks, and Config.CompressionIdle bounds how long
// an idle attached connection holds its compressor.
var (
	compressorPools  [12]sync.Pool // Full window, indexed by flate level + 2.
	windowPools      [7]sync.Pool  // Reduced windows, indexed by window bits - 8.
	decompressorPool sync.Pool
)

// compressorPool picks the pool for a negotiated configuration: a reduced
// window fixes the encoder and ignores the level.
func compressorPool(comp *handshake.Compression) *sync.Pool {
	if bits := comp.SendWindowBits; bits != 0 && bits != 15 {
		return &windowPools[bits-8]
	}
	return &compressorPools[comp.Level+2]
}

func (c *Conn) getCompressor() *deflate.Compressor {
	if comp, ok := compressorPool(c.compression).Get().(*deflate.Compressor); ok {
		return comp
	}
	// The configuration was validated by Reset.
	if bits := c.compression.SendWindowBits; bits != 0 && bits != 15 {
		comp, _ := deflate.NewCompressorWindow(bits)
		return comp
	}
	comp, _ := deflate.NewCompressor(c.compression.Level)
	return comp
}

func (c *Conn) putCompressor(comp *deflate.Compressor) {
	comp.Reset()
	compressorPool(c.compression).Put(comp)
}

// compressorLease is a compressor in use for one write or batch; release
// returns a pooled one to its pool once the frame bodies borrowing its output
// have been written. Callers hold wmu.
type compressorLease struct {
	*deflate.Compressor
	c      *Conn
	shared bool
}

func (c *Conn) leaseCompressor() *compressorLease {
	comp, shared := c.compressorFor()
	return &compressorLease{comp, c, shared}
}

func (l *compressorLease) release() {
	if l.shared {
		l.c.putCompressor(l.Compressor)
	}
}

// compressorFor returns the compressor for the next message, attaching a
// pooled one when the send direction has takeover. When shared is true the
// caller returns it to the pool after the write, since the frame body borrows
// its output. Callers hold wmu.
func (c *Conn) compressorFor() (comp *deflate.Compressor, shared bool) {
	if c.compressor != nil {
		return c.compressor, false
	}
	comp = c.getCompressor()
	if c.sendWindow == nil || c.shared {
		return comp, true
	}
	c.compressor = comp
	return comp, false
}

// noteCompressed arms the idle release after a compressed write. Callers hold wmu.
func (c *Conn) noteCompressed() {
	if c.compressor == nil || c.idle == 0 {
		return
	}
	c.lastCompress = time.Now()
	if c.idleTimer == nil {
		c.idleTimer = time.AfterFunc(c.idle, c.releaseIdleCompressor)
		return
	}
	c.idleTimer.Reset(c.idle)
}

func (c *Conn) releaseIdleCompressor() {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.compressor == nil {
		return
	}
	if remaining := c.idle - time.Since(c.lastCompress); remaining > 0 {
		c.idleTimer.Reset(remaining)
		return
	}
	c.releaseCompressor()
}

// releaseCompressor returns an attached compressor to the pool. Callers hold wmu.
func (c *Conn) releaseCompressor() {
	if c.compressor != nil {
		c.putCompressor(c.compressor)
		c.compressor = nil
	}
}

// acquireDecompressor attaches a pooled decompressor to the connection until
// the next read, so borrowed output remains valid.
func (c *Conn) acquireDecompressor() *deflate.Decompressor {
	if c.decompressor == nil {
		d, ok := decompressorPool.Get().(*deflate.Decompressor)
		if !ok {
			d = new(deflate.Decompressor)
		}
		c.decompressor = d
	}
	return c.decompressor
}

func (c *Conn) releaseDecompressor() {
	if c.decompressor != nil {
		c.decompressor.Reset()
		decompressorPool.Put(c.decompressor)
		c.decompressor = nil
	}
}

// decompress inflates a complete compressed message within the size limit.
func (c *Conn) decompress(payload []byte) ([]byte, error) {
	out, err := c.acquireDecompressor().Decompress(payload, c.limit, c.recvWindow)
	if err != nil {
		if errors.Is(err, deflate.ErrMessageTooLarge) {
			return nil, c.fail(&proto.Error{Code: 1009, Err: ErrMessageTooLarge})
		}
		return nil, c.fail(&proto.Error{Code: 1007, Err: ErrInvalidData})
	}
	return out, nil
}

// inflate serves Read for a compressed message by streaming frame payload
// through the decompressor.
func (c *Conn) inflate(b []byte) (int, error) {
	d := c.acquireDecompressor()
	if !c.inflating {
		if err := d.Begin(chunkSource{c}, c.recvWindow); err != nil {
			return 0, c.fail(&proto.Error{Code: 1007, Err: ErrInvalidData})
		}
		c.inflating = true
	}
	n, err := d.Read(b)
	switch {
	case err == nil:
		return n, nil
	case err == io.EOF:
		c.inflating, c.inMessage = false, false
		return 0, io.EOF
	case c.readErr != nil:
		// The source hit a protocol failure or the peer's close; both already recorded.
		return 0, c.readErr
	case err == c.srcErr:
		// A transport error poisoned the inflater, so this message cannot resume.
		c.readErr, c.inMessage, c.inflating = err, false, false
		return 0, err
	default:
		return 0, c.fail(&proto.Error{Code: 1007, Err: ErrInvalidData})
	}
}

// chunkSource feeds the inflater borrowed frame payload straight from the core.
type chunkSource struct{ c *Conn }

func (s chunkSource) NextChunk() ([]byte, error) {
	chunk, err := s.c.nextChunk()
	if err != nil && err != io.EOF {
		s.c.srcErr = err
	}
	return chunk, err
}

// nextChunk returns the next borrowed payload chunk of the current message,
// spanning frames and dispatching control frames, or io.EOF at its end.
func (c *Conn) nextChunk() ([]byte, error) {
	for {
		chunk, done, err := c.rx.Payload()
		if err != nil {
			return nil, c.fail(err)
		}
		if len(chunk) != 0 {
			return chunk, nil
		}
		if !done {
			if err := c.fill(); err != nil {
				return nil, err
			}
			continue
		}
		if !c.rx.MessageOpen() {
			return nil, io.EOF
		}
		if err := c.nextFrame(); err != nil {
			return nil, err
		}
	}
}
