package ws

import "github.com/33TU/ews/codec"

// MessageHandler receives the data messages of one connection.
type MessageHandler interface {
	// OnMessage is called on the goroutine running Serve with each complete
	// text or binary message. payload is borrowed until it returns; copy it
	// to keep it. It may write to c. Returning an error stops Serve, which
	// returns that error.
	OnMessage(c *Conn, op codec.Opcode, payload []byte) error
}

// MessageFunc adapts a function to MessageHandler.
type MessageFunc func(c *Conn, op codec.Opcode, payload []byte) error

func (f MessageFunc) OnMessage(c *Conn, op codec.Opcode, payload []byte) error {
	return f(c, op, payload)
}

// Serve reads messages from c and hands each to h until a read fails or h
// returns an error, and returns that error: a *CloseError when the peer
// closed, a *Error after a protocol failure, the transport's error otherwise.
// It is the read loop most servers write by hand, on the caller's goroutine;
// control frames still go through c's ControlHandler on the way.
func Serve(c *Conn, h MessageHandler) error {
	for {
		op, payload, err := c.ReadMessage()
		if err != nil {
			return err
		}
		if err := h.OnMessage(c, op, payload); err != nil {
			return err
		}
	}
}
