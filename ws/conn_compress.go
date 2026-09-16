package ws

import (
	"errors"
	"io"
	"sync"

	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
)

// compressorContext is the send side of permessage-deflate. Guarded by wmu.
type compressorContext struct {
	config   *handshake.Compression // Negotiated parameters, or nil.
	minSize  int                    // Payloads below this go uncompressed.
	shared   bool                   // Never keep a compressor attached.
	window   *deflate.Window        // Send-direction history when takeover is negotiated.
	attached *deflate.Compressor    // Continues window's stream between messages.
	scratch  []byte                 // Holds a shared compressor's output until written.

	// pending holds compressed Prepared payloads sent on this connection but
	// not yet copied into window, oldest first. Copying the
	// history is the whole cost of a broadcast to a takeover connection, and
	// it lands on the sender's goroutine, so it is deferred: entries whose
	// bytes the later ones already cover are dropped, and the rest are copied
	// only when this connection next compresses a message of its own. A
	// recipient that only ever receives broadcasts never copies at all.
	pending      [pendingHistory]*Prepared
	npending     int
	pendingBytes int
}

// pendingHistory bounds the prepared payloads a connection defers. Payloads
// smaller than the window accumulate until they cover it or fill this many
// slots, when they are copied in one go.
const pendingHistory = 8

// decompressorContext is the receive side. Used by the reading goroutine only.
type decompressorContext struct {
	dec      *deflate.Decompressor // Pooled; attached until the next read so borrowed output holds.
	window   *deflate.Window       // Receive-direction history when takeover is negotiated.
	rest     []byte                // Inflated bytes of the current message Read has not delivered.
	inflated bool                  // Read has inflated the current message.
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

// notePrepared records that p's compressed variant was sent, advancing the
// send window lazily. Callers hold wmu and have checked that window is set.
func (c *Conn) notePrepared(p *Prepared) {
	size := c.comp.window.Size()
	n := len(p.payload)
	if n >= size {
		// p's tail is the whole history; nothing older matters.
		c.dropPending()
	} else {
		for c.comp.npending > 0 && c.comp.pendingBytes-len(c.comp.pending[0].payload)+n >= size {
			c.dropOldestPending()
		}
		if c.comp.npending == pendingHistory {
			c.applyPending()
		}
	}
	c.comp.pending[c.comp.npending] = p
	c.comp.npending++
	c.comp.pendingBytes += n
}

// applyPending copies the deferred prepared payloads into the send window,
// in order. Callers hold wmu.
func (c *Conn) applyPending() {
	for i := 0; i < c.comp.npending; i++ {
		c.comp.window.Add(c.comp.pending[i].payload)
		c.comp.pending[i] = nil
	}
	c.comp.npending, c.comp.pendingBytes = 0, 0
}

// dropPending forgets every deferred payload without copying it.
func (c *Conn) dropPending() {
	for i := 0; i < c.comp.npending; i++ {
		c.comp.pending[i] = nil
	}
	c.comp.npending, c.comp.pendingBytes = 0, 0
}

func (c *Conn) dropOldestPending() {
	c.comp.pendingBytes -= len(c.comp.pending[0].payload)
	c.comp.npending--
	copy(c.comp.pending[:], c.comp.pending[1:c.comp.npending+1])
	c.comp.pending[c.comp.npending] = nil
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
			return nil, c.fail(&Error{Code: 1009, Err: ErrMessageTooLarge})
		}
		return nil, c.fail(&Error{Code: 1007, Err: ErrInvalidData})
	}
	return out, nil
}

// inflateWhole assembles and inflates the current compressed message on the
// first call for it, leaving the result in decomp.rest.
func (c *Conn) inflateWhole() error {
	if c.decomp.inflated {
		return nil
	}
	payload, err := c.assemble()
	if err != nil {
		return err
	}
	out, err := c.decompress(payload)
	if err != nil {
		return err
	}
	c.decomp.rest, c.decomp.inflated = out, true
	return nil
}

// readInflated serves Read for a compressed message. The inflater is
// pull-only and cannot resume after a short read, so the first call assembles
// and inflates the message whole, within MaxMessageSize as ReadMessage does,
// and later calls deliver chunks of the result.
func (c *Conn) readInflated(b []byte) (int, error) {
	if err := c.inflateWhole(); err != nil {
		return 0, err
	}
	if len(c.decomp.rest) == 0 {
		c.decomp.rest, c.decomp.inflated, c.inMessage = nil, false, false
		return 0, io.EOF
	}
	n := copy(b, c.decomp.rest)
	c.decomp.rest = c.decomp.rest[n:]
	return n, nil
}
