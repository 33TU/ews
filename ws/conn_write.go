package ws

import (
	"io"
	"net"
	"sync"

	"github.com/33TU/ews/codec"
)

// Write sends one text or binary message as a single frame, compressed when
// negotiated and at least Compression.MinSize bytes long.
// payload is not retained after Write returns.
func (c *Conn) Write(op codec.Opcode, payload []byte) error {
	if op != codec.Text && op != codec.Binary {
		return ErrProtocol
	}
	c.wmu.Lock()
	if c.frag.op != 0 {
		c.wmu.Unlock()
		return ErrMessageOpen
	}
	header, body, err := c.encodeData(op, payload)
	if err != nil {
		c.wmu.Unlock()
		return err
	}
	q, seq, err := c.sendFrame(header, body)
	c.wmu.Unlock()
	return await(q, seq, err)
}

// sendFrame writes a data frame directly, or on a connection with a Queue
// enqueues it in submission order and returns the sequence number for the
// caller to await after releasing wmu. Callers hold wmu.
func (c *Conn) sendFrame(header, body []byte) (*Queue, uint64, error) {
	if q := c.queue; q != nil {
		seq, err := q.enqueue(header, body, nil, nil)
		return q, seq, err
	}
	return nil, 0, c.write(header, body)
}

// encodeData encodes one data message, compressing per the connection's
// settings. The output is borrowed until the next encode. Callers hold wmu.
func (c *Conn) encodeData(op codec.Opcode, payload []byte) (header, body []byte, err error) {
	if c.comp.config == nil || len(payload) < c.comp.minSize {
		return c.tx.Encode(op, payload)
	}
	c.applyPending()
	comp, shared := c.compressorFor()
	compressed, err := comp.Compress(payload, c.comp.window)
	if err == nil {
		header, body, err = c.tx.EncodeCompressed(op, compressed)
	}
	if shared {
		// The body borrows the compressor's output; copy it so the compressor
		// can go back to the pool now rather than after the write.
		body = append(c.comp.scratch[:0], body...)
		c.comp.scratch = body
		c.putCompressor(comp)
	}
	return header, body, err
}

// fromPool holds WriteFrom chunk buffers; a buffer smaller than the
// connection's fragment size is regrown.
var fromPool sync.Pool

func (c *Conn) getChunk() *[]byte {
	bp, ok := fromPool.Get().(*[]byte)
	if !ok {
		bp = new([]byte)
	}
	if cap(*bp) < c.fragmentSize {
		*bp = make([]byte, c.fragmentSize)
	}
	*bp = (*bp)[:c.fragmentSize]
	return bp
}

// WriteFrom sends r's content as one message and returns the bytes sent.
// Content that fits one Config.FragmentSize read goes out as a single frame;
// longer content is fragmented as it is read, one chunk ahead so the last
// chunk carries the FIN and nothing is held in memory beyond two chunks. A
// read error is returned with the message still open and the chunk being
// read unsent, since a fragmented message cannot be withdrawn; close the
// connection in that case.
func (c *Conn) WriteFrom(op codec.Opcode, r io.Reader) (int64, error) {
	if op != codec.Text && op != codec.Binary {
		return 0, ErrProtocol
	}
	cur := c.getChunk()
	defer fromPool.Put(cur)

	n, err := io.ReadFull(r, *cur)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return int64(n), c.Write(op, (*cur)[:n])
	}
	if err != nil {
		return 0, err
	}
	next := c.getChunk()
	defer fromPool.Put(next)
	if err := c.BeginMessage(op); err != nil {
		return 0, err
	}
	var total int64
	for {
		// Look one chunk ahead so the current one can be final.
		m, err := io.ReadFull(r, *next)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return total, err
		}
		final := err == io.EOF
		if werr := c.writeFragment((*cur)[:n], final); werr != nil {
			return total, werr
		}
		total += int64(n)
		if final {
			return total, nil
		}
		if err == io.ErrUnexpectedEOF {
			if werr := c.writeFragment((*next)[:m], true); werr != nil {
				return total, werr
			}
			return total + int64(m), nil
		}
		cur, next, n = next, cur, m
	}
}

// BeginMessage starts a fragmented text or binary message. Each WriteChunk
// then sends one fragment and EndMessage finishes it. Until then Write and
// BeginMessage return ErrMessageOpen; Ping, Pong, and Close may interleave,
// as the protocol allows. The message is compressed whenever compression is
// negotiated, regardless of MinSize.
func (c *Conn) BeginMessage(op codec.Opcode) error {
	if op != codec.Text && op != codec.Binary {
		return ErrProtocol
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.frag.op != 0 {
		return ErrMessageOpen
	}
	if c.tx.CloseSent() {
		return ErrClosing
	}
	c.frag.op, c.frag.first, c.frag.compressed = op, true, c.comp.config != nil
	if c.frag.compressed && c.comp.attached == nil {
		// Hold one compressor for the whole message so its chunks continue one stream.
		c.comp.attached, c.frag.heldCompressor = c.getCompressor(), true
	}
	return nil
}

// WriteChunk sends one non-final fragment of the open message. Chunks are
// delivered to the peer as they are written; an empty chunk sends an empty frame.
func (c *Conn) WriteChunk(payload []byte) error {
	return c.writeFragment(payload, false)
}

// EndMessage sends the final, empty fragment of the open message.
func (c *Conn) EndMessage() error {
	return c.writeFragment(nil, true)
}

func (c *Conn) writeFragment(payload []byte, final bool) error {
	c.wmu.Lock()
	q, seq, err := c.fragment(payload, final)
	c.wmu.Unlock()
	return await(q, seq, err)
}

// fragment sends one frame of the open message, directly or through the
// queue. Callers hold wmu and await the returned sequence number.
func (c *Conn) fragment(payload []byte, final bool) (*Queue, uint64, error) {
	if c.frag.op == 0 {
		return nil, 0, ErrNoMessage
	}
	op := codec.Continuation
	if c.frag.first {
		op = c.frag.op
	}
	if c.frag.compressed {
		c.applyPending()
		var err error
		if payload, err = c.comp.attached.CompressChunk(payload, c.comp.window, final); err != nil {
			return nil, 0, c.endFragmented(err)
		}
	}
	header, body, err := c.tx.EncodeFragment(op, final, payload, c.frag.compressed)
	if err != nil {
		return nil, 0, c.endFragmented(err)
	}
	q, seq, err := c.sendFrame(header, body)
	if err != nil {
		return nil, 0, c.endFragmented(err)
	}
	c.frag.first = false
	if final {
		return q, seq, c.endFragmented(nil)
	}
	return q, seq, nil
}

// endFragmented closes the fragmented message state after its final frame or
// a failure, returning any held compressor. Callers hold wmu.
func (c *Conn) endFragmented(err error) error {
	c.frag.op = 0
	if c.frag.heldCompressor {
		c.frag.heldCompressor = false
		if c.comp.shared || c.comp.window == nil {
			c.releaseCompressor()
		}
	}
	return err
}

// Ping sends a ping with at most 125 payload bytes.
func (c *Conn) Ping(payload []byte) error { return c.send(codec.Ping, payload) }

// Pong sends a pong with at most 125 payload bytes. Pongs are allowed after Close.
func (c *Conn) Pong(payload []byte) error { return c.send(codec.Pong, payload) }

// Close sends a close frame once. Code zero sends an empty payload and requires
// an empty reason. Later data writes return ErrClosing. The transport stays
// open; reads deliver the peer's close as a *CloseError.
func (c *Conn) Close(code uint16, reason string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	header, body, err := c.tx.EncodeClose(code, reason)
	if err != nil {
		return err
	}
	return c.write(header, body)
}

// CloseSent reports whether a close frame has been sent or attempted.
func (c *Conn) CloseSent() bool {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.tx.CloseSent()
}

func (c *Conn) send(op codec.Opcode, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.frag.op != 0 && op != codec.Ping && op != codec.Pong && op != codec.Close {
		return ErrMessageOpen
	}
	header, body, err := c.tx.Encode(op, payload)
	if err != nil {
		return err
	}
	return c.write(header, body)
}

// fragmentState is the open fragmented message. Guarded by wmu.
type fragmentState struct {
	op             codec.Opcode // Opcode of the open message, or zero.
	first          bool         // The next fragment carries op.
	compressed     bool         // The message is compressed.
	heldCompressor bool         // A compressor was taken for the message in shared mode.
}

// writeBuffers backs the two-element writev of header and payload.
// net.Buffers.WriteTo consumes the slice it is given, so the array is kept
// and the consumable header rebuilt from it each time.
type writeBuffers struct {
	arr  [2][]byte
	bufs net.Buffers
}

// coalesceLimit is the largest payload copied next to its header for one
// write on transports without writev. It matches the TLS record size, so a
// coalesced frame is one record; above it the copy costs more than it saves.
const coalesceLimit = 16 << 10

// write sends header and body: one writev on kernel sockets, one copied
// write for small frames elsewhere, and two writes for large ones. Callers
// hold wmu; the transport lock keeps a Queue's writes from interleaving.
func (c *Conn) write(header, body []byte) error {
	c.iomu.Lock()
	defer c.iomu.Unlock()
	switch {
	case len(body) == 0:
		_, err := c.rw.Write(header)
		return err
	case c.vectored:
		c.out.bufs = append(net.Buffers(c.out.arr[:0]), header, body)
		_, err := c.out.bufs.WriteTo(c.rw)
		c.out.arr = [2][]byte{} // Do not retain the caller's payload.
		return err
	case len(body) <= coalesceLimit:
		buf := writePool.Get().(*[]byte)
		*buf = append(append((*buf)[:0], header...), body...)
		_, err := c.rw.Write(*buf)
		writePool.Put(buf)
		return err
	default:
		if _, err := c.rw.Write(header); err != nil {
			return err
		}
		_, err := c.rw.Write(body)
		return err
	}
}

// writePool holds coalescing buffers for transports without writev.
var writePool = sync.Pool{New: func() any { b := make([]byte, 0, coalesceLimit+codec.MaxHeaderSize); return &b }}

// sendClose is the best-effort close frame that accompanies a protocol failure.
func (c *Conn) sendClose(code uint16) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.tx.CloseSent() {
		return
	}
	if header, body, err := c.tx.EncodeClose(code, ""); err == nil {
		_ = c.write(header, body)
	}
}
