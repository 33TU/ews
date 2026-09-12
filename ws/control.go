package ws

// ControlHandler handles control frames during reads.
// Shared handlers must support concurrent calls from different connections.
// Payloads are borrowed for the duration of each call.
// Handlers replace defaults: OnPing must send a Pong, and OnClose a close response
// if needed. They may write but must not read from c. An error terminates the connection.
type ControlHandler interface {
	OnPing(c *Conn, payload []byte) error
	OnPong(c *Conn, payload []byte) error
	OnClose(c *Conn, code uint16, reason []byte) error
}

// DefaultControlHandler answers pings, ignores pongs, and echoes close codes.
// Embed it to override a single method.
type DefaultControlHandler struct{}

func (DefaultControlHandler) OnPing(c *Conn, payload []byte) error { return c.Pong(payload) }

func (DefaultControlHandler) OnPong(*Conn, []byte) error { return nil }

func (DefaultControlHandler) OnClose(c *Conn, code uint16, _ []byte) error {
	if code == NoStatus {
		code = 0
	}
	if err := c.Close(code, ""); err != nil && err != ErrClosing {
		return err
	}
	return nil
}
