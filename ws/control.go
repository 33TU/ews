package ws

// ControlHandler handles control frames during reads. Payloads are borrowed
// for the duration of each call, and a shared handler must support
// concurrent calls from different connections.
//
// Handlers replace the defaults: OnPing must send a Pong, and OnClose a close
// response if needed. They may write to c but must not read from it. An
// error terminates the connection.
type ControlHandler interface {
	OnPing(c *Conn, payload []byte) error
	OnPong(c *Conn, payload []byte) error
	OnClose(c *Conn, code uint16, reason []byte) error
}

// DefaultControlHandler answers pings, ignores pongs, and echoes close codes.
// Embed it to override a single method.
type DefaultControlHandler struct{}

// OnPing answers with a pong carrying the same payload.
func (DefaultControlHandler) OnPing(c *Conn, payload []byte) error { return c.Pong(payload) }

// OnPong ignores the frame.
func (DefaultControlHandler) OnPong(*Conn, []byte) error { return nil }

// OnClose echoes the peer's close code. A failed echo is not an error: the
// peer has closed and may already have dropped the transport, and the read
// that dispatched this frame returns the peer's code, which is what the
// caller needs. Callers who care about the echo itself send their own.
func (DefaultControlHandler) OnClose(c *Conn, code uint16, _ []byte) error {
	if code == NoStatus {
		code = 0
	}
	_ = c.Close(code, "")
	return nil
}
