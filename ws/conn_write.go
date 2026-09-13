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
	if c.compression != nil && len(payload) >= c.compression.MinSize {
		return c.sendCompressed(op, payload)
	}
	return c.send(op, payload)
}

// fromChunk is the fragment size WriteFrom reads and sends.
const fromChunk = 32 << 10

var fromPool = sync.Pool{New: func() any { b := make([]byte, fromChunk); return &b }}

// WriteFrom sends r's content as one message and returns the bytes sent.
// Content that fits one 32 KiB read goes out as a single frame; longer
// content is fragmented as it is read, so nothing is held in memory beyond
// one chunk. A read error is returned with the message still open and the
// chunk being read unsent, since a fragmented message cannot be withdrawn;
// close the connection in that case.
func (c *Conn) WriteFrom(op codec.Opcode, r io.Reader) (int64, error) {
	if op != codec.Text && op != codec.Binary {
		return 0, ErrProtocol
	}
	bp := fromPool.Get().(*[]byte)
	defer fromPool.Put(bp)
	buf := *bp

	n, err := io.ReadFull(r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return int64(n), c.Write(op, buf[:n])
	}
	if err != nil {
		return 0, err
	}
	if err := c.BeginMessage(op); err != nil {
		return 0, err
	}
	var total int64
	for {
		if err := c.WriteChunk(buf[:n]); err != nil {
			return total, err
		}
		total += int64(n)
		n, err = io.ReadFull(r, buf)
		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF {
			if err := c.WriteChunk(buf[:n]); err != nil {
				return total, err
			}
			total += int64(n)
			break
		}
		if err != nil {
			return total, err
		}
	}
	return total, c.EndMessage()
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
	if c.fragOp != 0 {
		return ErrMessageOpen
	}
	if c.tx.CloseSent() {
		return ErrClosing
	}
	c.fragOp, c.fragFirst, c.fragCompress = op, true, c.compression != nil
	if c.fragCompress && c.compressor == nil {
		// Hold one compressor for the whole message so its chunks continue one stream.
		c.compressor, c.fragHeld = c.getCompressor(), true
	}
	return nil
}

// WriteChunk sends one non-final fragment of the open message. Chunks are
// delivered to the peer as they are written; an empty chunk sends an empty frame.
func (c *Conn) WriteChunk(payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.fragment(payload, false)
}

// EndMessage sends the final, empty fragment of the open message.
func (c *Conn) EndMessage() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.fragment(nil, true)
}

// fragment sends one frame of the open message. Callers hold wmu.
func (c *Conn) fragment(payload []byte, final bool) error {
	if c.fragOp == 0 {
		return ErrNoMessage
	}
	op := codec.Continuation
	if c.fragFirst {
		op = c.fragOp
	}
	if c.fragCompress {
		var err error
		if payload, err = c.compressor.CompressChunk(payload, c.sendWindow, final); err != nil {
			return c.endFragmented(err)
		}
	}
	header, body, err := c.tx.EncodeFragment(op, final, payload, c.fragCompress)
	if err != nil {
		return c.endFragmented(err)
	}
	if err := c.write(header, body); err != nil {
		return c.endFragmented(err)
	}
	c.fragFirst = false
	if final {
		return c.endFragmented(nil)
	}
	return nil
}

// endFragmented closes the fragmented message state after its final frame or
// a failure, returning any held compressor. Callers hold wmu.
func (c *Conn) endFragmented(err error) error {
	c.fragOp = 0
	if c.fragHeld {
		c.fragHeld = false
		if c.shared || c.sendWindow == nil {
			c.releaseCompressor()
		}
	}
	if err == nil && c.fragCompress {
		c.noteCompressed()
	}
	return err
}

func (c *Conn) sendCompressed(op codec.Opcode, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.fragOp != 0 {
		return ErrMessageOpen
	}
	comp, shared := c.compressorFor()
	if shared {
		defer c.putCompressor(comp) // After the write: the body borrows its output.
	}
	compressed, err := comp.Compress(payload, c.sendWindow)
	if err != nil {
		return err
	}
	header, body, err := c.tx.EncodeCompressed(op, compressed)
	if err != nil {
		return err
	}
	if err := c.write(header, body); err != nil {
		return err
	}
	c.noteCompressed()
	return nil
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
	if c.fragOp != 0 && op != codec.Ping && op != codec.Pong && op != codec.Close {
		return ErrMessageOpen
	}
	header, body, err := c.tx.Encode(op, payload)
	if err != nil {
		return err
	}
	return c.write(header, body)
}

// coalesceLimit is the largest payload copied next to its header for one
// write on transports without writev. It matches the TLS record size, so a
// coalesced frame is one record; above it the copy costs more than it saves.
const coalesceLimit = 16 << 10

// write sends header and body: one writev on kernel sockets, one copied
// write for small frames elsewhere, and two writes for large ones.
func (c *Conn) write(header, body []byte) error {
	switch {
	case len(body) == 0:
		_, err := c.rw.Write(header)
		return err
	case c.vectored:
		c.bufs = append(net.Buffers(c.bufArr[:0]), header, body)
		_, err := c.bufs.WriteTo(c.rw)
		c.bufArr = [2][]byte{} // Do not retain the caller's payload.
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
