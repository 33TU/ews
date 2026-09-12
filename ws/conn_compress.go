package ws

import (
	"errors"
	"io"
	"sync"

	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/internal/proto"
)

// Helpers without context takeover carry no state between messages, so they
// are shared across connections. A flate writer holds hundreds of kilobytes.
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
	compressorPools[level+2].Put(c)
}

// acquireDecompressor returns the connection's decompressor or a pooled one,
// which stays attached until the next read so borrowed output remains valid.
func (c *Conn) acquireDecompressor() *deflate.Decompressor {
	if c.decompressor == nil {
		d, ok := decompressorPool.Get().(*deflate.Decompressor)
		if !ok {
			d = new(deflate.Decompressor)
		}
		c.decompressor, c.pooledDec = d, true
	}
	return c.decompressor
}

func (c *Conn) releaseDecompressor() {
	if c.pooledDec {
		c.decompressor.Reset()
		decompressorPool.Put(c.decompressor)
		c.decompressor, c.pooledDec = nil, false
	}
}

// decompress inflates a complete compressed message within the size limit.
func (c *Conn) decompress(payload []byte) ([]byte, error) {
	out, err := c.acquireDecompressor().Decompress(payload, c.limit)
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
		if err := d.Begin(chunkSource{c}); err != nil {
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
