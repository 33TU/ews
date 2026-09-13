package ws

import (
	"errors"
	"io"
	"sync"

	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/internal/proto"
)

// compressorContext is the send side of permessage-deflate. Guarded by wmu.
type compressorContext struct {
	config   *handshake.Compression // Negotiated parameters, or nil.
	minSize  int                    // Payloads below this go uncompressed.
	shared   bool                   // Never keep a compressor attached.
	window   *deflate.Window        // Send-direction history when takeover is negotiated.
	attached *deflate.Compressor    // Continues window's stream between messages.
	scratch  []byte                 // Holds a shared compressor's output until written.
}

// decompressorContext is the receive side. Used by the reading goroutine only.
type decompressorContext struct {
	dec       *deflate.Decompressor // Pooled; attached until the next read so borrowed output holds.
	window    *deflate.Window       // Receive-direction history when takeover is negotiated.
	streaming bool                  // Read is streaming the current message through the inflater.
	srcErr    error                 // Error raised while feeding the inflater.
}

// Context takeover state is a 32 KB window per direction on the connection.
// Decompressors are always shared across connections. Compressors are shared
// too, except that a connection with send takeover keeps one attached so its
// messages continue one stream instead of re-priming from the window on every
// message. Attached is faster per message; shared is far smaller per
// connection and wins once connections outnumber what the cache can hold.
// Config.CompressionShared picks.
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
	if comp, ok := compressorPool(c.comp.config).Get().(*deflate.Compressor); ok {
		return comp
	}
	return newCompressor(c.comp.config)
}

// newCompressor builds the compressor a validated configuration calls for.
func newCompressor(comp *handshake.Compression) *deflate.Compressor {
	if bits := comp.SendWindowBits; bits != 0 && bits != 15 {
		c, _ := deflate.NewCompressorWindow(bits)
		return c
	}
	c, _ := deflate.NewCompressor(comp.Level)
	return c
}

func (c *Conn) putCompressor(comp *deflate.Compressor) {
	comp.Reset()
	compressorPool(c.comp.config).Put(comp)
}

// compressorFor returns the compressor for the next message, attaching a
// pooled one when the send direction has takeover. When shared is true the
// caller returns it to the pool after the write, since the frame body borrows
// its output. Callers hold wmu.
func (c *Conn) compressorFor() (comp *deflate.Compressor, shared bool) {
	if c.comp.attached != nil {
		return c.comp.attached, false
	}
	comp = c.getCompressor()
	if c.comp.window == nil || c.comp.shared {
		return comp, true
	}
	c.comp.attached = comp
	return comp, false
}

// releaseCompressor returns an attached compressor to the pool. Callers hold wmu.
func (c *Conn) releaseCompressor() {
	if c.comp.attached != nil {
		c.putCompressor(c.comp.attached)
		c.comp.attached = nil
	}
}

// acquireDecompressor attaches a pooled decompressor to the connection until
// the next read, so borrowed output remains valid.
func (c *Conn) acquireDecompressor() *deflate.Decompressor {
	if c.decomp.dec == nil {
		d, ok := decompressorPool.Get().(*deflate.Decompressor)
		if !ok {
			d = new(deflate.Decompressor)
		}
		c.decomp.dec = d
	}
	return c.decomp.dec
}

func (c *Conn) releaseDecompressor() {
	if c.decomp.dec != nil {
		c.decomp.dec.Reset()
		decompressorPool.Put(c.decomp.dec)
		c.decomp.dec = nil
	}
}

// decompress inflates a complete compressed message within the size limit.
func (c *Conn) decompress(payload []byte) ([]byte, error) {
	out, err := c.acquireDecompressor().Decompress(payload, c.limit, c.decomp.window)
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
	if !c.decomp.streaming {
		if err := d.Begin(chunkSource{c}, c.decomp.window); err != nil {
			return 0, c.fail(&proto.Error{Code: 1007, Err: ErrInvalidData})
		}
		c.decomp.streaming = true
	}
	n, err := d.Read(b)
	switch {
	case err == nil:
		return n, nil
	case err == io.EOF:
		c.decomp.streaming, c.inMessage = false, false
		return 0, io.EOF
	case c.readErr != nil:
		// The source hit a protocol failure or the peer's close; both already recorded.
		return 0, c.readErr
	case err == c.decomp.srcErr:
		// A transport error poisoned the inflater, so this message cannot resume.
		c.readErr, c.inMessage, c.decomp.streaming = err, false, false
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
		s.c.decomp.srcErr = err
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
