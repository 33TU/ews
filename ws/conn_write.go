package ws

import (
	"net"

	"github.com/33TU/ews/codec"
)

// Write sends one text or binary message as a single frame.
// payload is not retained after Write returns.
func (c *Conn) Write(op codec.Opcode, payload []byte) error {
	if op != codec.Text && op != codec.Binary {
		return ErrProtocol
	}
	return c.send(op, payload)
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

// write sends header and body with one writev on transports that support it.
func (c *Conn) write(header, body []byte) error {
	if len(body) == 0 {
		_, err := c.rw.Write(header)
		return err
	}
	c.bufs = append(net.Buffers(c.bufArr[:0]), header, body)
	_, err := c.bufs.WriteTo(c.rw)
	c.bufArr = [2][]byte{} // Do not retain the caller's payload.
	return err
}

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
