package ws

import (
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

func (c *Conn) sendCompressed(op codec.Opcode, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
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
