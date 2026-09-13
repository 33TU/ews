package ws

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/internal/proto"
)

// Context takeover state is a 32 KB window per direction on the connection.
// Decompressors are always shared across connections. Compressors are shared
// too, except that a connection with send takeover keeps one attached so its
// messages continue one stream instead of re-priming, which costs about as
// much as compressing 32 KB; Config.CompressionIdle bounds how long an idle
// connection holds it.
var (
	compressorPools  [12]sync.Pool // Indexed by flate level + 2.
	decompressorPool sync.Pool
)

func getCompressor(level int) *deflate.Compressor {
	if c, ok := compressorPools[level+2].Get().(*deflate.Compressor); ok {
		return c
	}
	c, _ := deflate.NewCompressor(level) // The level was validated by Reset.
	return c
}

func putCompressor(level int, c *deflate.Compressor) {
	c.Reset()
	compressorPools[level+2].Put(c)
}

// compressorFor returns the compressor for the next message, attaching a
// pooled one when the send direction has takeover. When shared is true the
// caller returns it to the pool after the write, since the frame body borrows
// its output. Callers hold wmu.
func (c *Conn) compressorFor() (comp *deflate.Compressor, shared bool) {
	if c.compressor != nil {
		return c.compressor, false
	}
	comp = getCompressor(c.compression.Level)
	if c.sendWindow == nil {
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
		putCompressor(c.compression.Level, c.compressor)
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
