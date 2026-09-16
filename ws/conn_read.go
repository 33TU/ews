package ws

import (
	"io"
	"slices"
	"sync"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/internal/proto"
	"github.com/33TU/ews/internal/utf8"
)

// NextMessage returns the opcode of the next text or binary message, dispatching
// control frames on the way. Any unread payload of the current message is
// discarded first.
func (c *Conn) NextMessage() (codec.Opcode, error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	if err := c.discard(); err != nil {
		return 0, err
	}
	c.releaseDecompressor()
	c.releaseMsg()
	if err := c.nextFrame(); err != nil {
		return 0, err
	}
	c.inMessage = true
	return c.rx.Header().Opcode(), nil
}

// Read copies payload of the current message into b, spanning continuation
// frames and dispatching interleaved control frames. It returns 0, io.EOF at
// the end of the message and before NextMessage has been called. Transport EOF
// mid-message is io.ErrUnexpectedEOF. Text read in chunks is never UTF-8
// validated; only complete messages are. A compressed message is assembled
// and inflated whole on the first Read, within Config.MaxMessageSize as for
// ReadMessage, and delivered from that buffer.
func (c *Conn) Read(b []byte) (int, error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	if !c.inMessage {
		return 0, io.EOF
	}
	if len(b) == 0 {
		return 0, nil
	}
	if c.rx.MessageCompressed() {
		return c.readInflated(b)
	}
	for {
		chunk, done, err := c.rx.PayloadN(len(b))
		if err != nil {
			return 0, c.fail(err)
		}
		if len(chunk) != 0 {
			return copy(b, chunk), nil
		}
		if !done {
			// Frame open, nothing buffered. Large reads go straight into b.
			n := len(b)
			if r := c.rx.Remaining(); r < uint64(n) {
				n = int(r)
			}
			if n >= len(c.buf) {
				return c.readDirect(b[:n])
			}
			if err := c.fill(); err != nil {
				return 0, err
			}
			continue
		}
		if !c.rx.MessageOpen() {
			c.inMessage = false
			return 0, io.EOF
		}
		if err := c.nextFrame(); err != nil {
			return 0, err
		}
	}
}

// WriteTo writes the rest of the current message to w, so io.Copy(w, c) moves
// a message without a buffer of its own. Plain frames go to w as they arrive:
// what the read buffer holds is borrowed, and a longer remainder is read from
// the transport into the pooled message buffer in pieces of up to
// Config.FragmentSize. A compressed message is inflated whole and written in
// one call. It returns nil at the end of the message and writes nothing when
// no message is open. An error from w is returned as is and leaves the rest
// of the message unread; NextMessage discards it.
func (c *Conn) WriteTo(w io.Writer) (int64, error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	if !c.inMessage {
		return 0, nil
	}
	if c.rx.MessageCompressed() {
		if err := c.inflateWhole(); err != nil {
			return 0, err
		}
		rest := c.decomp.rest
		c.decomp.rest, c.decomp.inflated, c.inMessage = nil, false, false
		if len(rest) == 0 {
			return 0, nil
		}
		n, err := w.Write(rest)
		return int64(n), err
	}
	var total int64
	for {
		chunk, done, err := c.rx.Payload()
		if err != nil {
			return total, c.fail(err)
		}
		if len(chunk) == 0 && !done {
			// Frame open, nothing buffered. A remainder at least as large as
			// the read buffer is read into the message buffer instead.
			remaining := c.rx.Remaining()
			if remaining < uint64(len(c.buf)) {
				if err := c.fill(); err != nil {
					return total, err
				}
				continue
			}
			var staging []byte
			if c.msg != nil {
				staging = c.msg.b[:0]
			}
			if chunk, err = c.readInto(staging, int(min(remaining, uint64(c.fragmentSize)))); err != nil {
				return total, err
			}
			done = c.rx.Remaining() == 0
		}
		if len(chunk) != 0 {
			n, err := w.Write(chunk)
			total += int64(n)
			if err != nil {
				return total, err
			}
			if n != len(chunk) {
				return total, io.ErrShortWrite
			}
		}
		if !done {
			continue
		}
		if !c.rx.MessageOpen() {
			c.inMessage = false
			return total, nil
		}
		if err := c.nextFrame(); err != nil {
			return total, err
		}
	}
}

// readInto reads up to n bytes of the open frame's payload from the transport
// into the message buffer's spare capacity, bypassing the read buffer. The
// size budget has already admitted n. The receiver has no buffered input.
func (c *Conn) readInto(msg []byte, n int) ([]byte, error) {
	msg = c.growMsg(msg, n)
	spare := msg[len(msg) : len(msg)+n]
	got, err := c.read(spare)
	if err != nil {
		return msg, err
	}
	c.rx.Feed(spare[:got])
	chunk, _, err := c.rx.PayloadN(got) // Unmasks in place; chunk aliases spare.
	if err != nil {
		return msg, c.fail(err)
	}
	return msg[:len(msg)+len(chunk)], nil
}

// Message buffers are pooled across connections and held only until the next
// read, so idle connections keep no assembly storage.
var msgPool sync.Pool

type msgBuf struct{ b []byte }

func (c *Conn) growMsg(msg []byte, n int) []byte {
	if c.msg == nil {
		if m, ok := msgPool.Get().(*msgBuf); ok {
			c.msg = m
		} else {
			c.msg = new(msgBuf)
		}
		msg = c.msg.b[:0]
	}
	msg = slices.Grow(msg, n)
	c.msg.b = msg
	return msg
}

func (c *Conn) appendMsg(msg, chunk []byte) []byte {
	msg = c.growMsg(msg, len(chunk))
	msg = append(msg, chunk...)
	c.msg.b = msg
	return msg
}

func (c *Conn) releaseMsg() {
	if c.msg != nil {
		c.msg.b = c.msg.b[:0]
		msgPool.Put(c.msg)
		c.msg = nil
	}
}

// readDirect reads frame payload from the transport into b, bypassing the read
// buffer. The receiver has no buffered input and len(b) is within the frame.
func (c *Conn) readDirect(b []byte) (int, error) {
	n, err := c.read(b)
	if err != nil {
		return 0, err
	}
	c.rx.Feed(b[:n])
	chunk, _, err := c.rx.PayloadN(n)
	if err != nil {
		return 0, c.fail(err)
	}
	return len(chunk), nil
}

// ReadMessage returns the next complete text or binary message. The payload is
// borrowed until the next read call. Messages larger than
// Config.MaxMessageSize, before or after decompression, fail with close code
// 1009; undecodable compressed data, and with Config.ValidateUTF8 text that
// is not valid UTF-8, fail with 1007.
func (c *Conn) ReadMessage() (codec.Opcode, []byte, error) {
	op, err := c.NextMessage()
	if err != nil {
		return 0, nil, err
	}
	payload, err := c.assemble()
	if err != nil {
		return 0, nil, err
	}
	return c.finishMessage(op, payload)
}

// assemble reads the rest of the current message through FIN, within
// MaxMessageSize. A single-frame message that arrived in one chunk is
// borrowed from the read buffer; anything else is gathered in the pooled
// message buffer. Either is held until the next read call.
func (c *Conn) assemble() ([]byte, error) {
	msg := []byte(nil)
	for {
		if c.rx.Header().PayloadLen() > uint64(c.limit-len(msg)) {
			return nil, c.fail(&proto.Error{Code: 1009, Err: ErrMessageTooLarge})
		}
		for {
			chunk, done, err := c.rx.Payload()
			if err != nil {
				return nil, c.fail(err)
			}
			if done && len(msg) == 0 && !c.rx.MessageOpen() {
				return chunk, nil
			}
			if len(chunk) != 0 || done {
				msg = c.appendMsg(msg, chunk)
				if done {
					break
				}
				continue
			}
			// Frame open, nothing buffered. A remainder at least as large as
			// the read buffer goes straight into the message buffer.
			if remaining := c.rx.Remaining(); remaining >= uint64(len(c.buf)) {
				var err error
				if msg, err = c.readInto(msg, int(remaining)); err != nil {
					return nil, err
				}
				continue
			}
			if err := c.fill(); err != nil {
				return nil, err
			}
		}
		if !c.rx.MessageOpen() {
			return msg, nil
		}
		if err := c.nextFrame(); err != nil {
			return nil, err
		}
	}
}

// finishMessage decompresses and validates an assembled message.
func (c *Conn) finishMessage(op codec.Opcode, payload []byte) (codec.Opcode, []byte, error) {
	c.inMessage = false
	if c.rx.MessageCompressed() {
		var err error
		if payload, err = c.decompress(payload); err != nil {
			return 0, nil, err
		}
	}
	if op == codec.Text && c.validateUTF8 && !utf8.Valid(payload) {
		return 0, nil, c.fail(&proto.Error{Code: 1007, Err: ErrInvalidUTF8})
	}
	return op, payload, nil
}

// discard drains the rest of the current message. A compressed message on a
// connection with receive context takeover is inflated rather than skipped,
// since the peer's next message may reference its content through the
// shared history; without takeover, or once Read has inflated it, the wire
// bytes are simply consumed.
func (c *Conn) discard() error {
	if c.inMessage && c.rx.MessageCompressed() && !c.decomp.inflated && c.decomp.window != nil {
		if err := c.inflateWhole(); err != nil {
			return err
		}
	}
	c.decomp.rest, c.decomp.inflated = nil, false
	for c.inMessage {
		chunk, done, err := c.rx.Payload()
		if err != nil {
			return c.fail(err)
		}
		if !done {
			if len(chunk) == 0 {
				if err := c.fill(); err != nil {
					return err
				}
			}
			continue
		}
		if !c.rx.MessageOpen() {
			c.inMessage = false
			return nil
		}
		if err := c.nextFrame(); err != nil {
			return err
		}
	}
	return nil
}

// nextFrame advances to the next validated data frame, filling from the
// transport and dispatching control frames as needed.
func (c *Conn) nextFrame() error {
	for {
		kind, err := c.rx.Next()
		if err != nil {
			return c.fail(err)
		}
		switch kind {
		case proto.DataFrame:
			return nil
		case proto.ControlFrame:
			if err := c.handleControl(); err != nil {
				return err
			}
		default:
			if err := c.fill(); err != nil {
				return err
			}
		}
	}
}

// fill reads from the transport into the read buffer. Payload is always
// drained first, so at most a partial header is preserved.
func (c *Conn) fill() error {
	c.rx.Preserve()
	n, err := c.read(c.buf)
	if err != nil {
		return err
	}
	c.rx.Feed(c.buf[:n])
	return nil
}

// read performs one transport read of at least one byte. Transport errors are
// not sticky: the receiver state is intact and the caller may retry.
func (c *Conn) read(b []byte) (int, error) {
	for range 100 { // Tolerate misbehaving readers like bufio does.
		n, err := c.rw.Read(b)
		if n != 0 {
			return n, nil
		}
		if err == io.EOF && !c.rx.Idle() {
			return 0, io.ErrUnexpectedEOF
		}
		if err != nil {
			return 0, err
		}
	}
	return 0, io.ErrNoProgress
}

func (c *Conn) handleControl() error {
	h := c.ControlHandler
	if h == nil {
		h = DefaultControlHandler{}
	}
	payload := c.rx.ControlPayload()
	var err error
	switch c.rx.ControlOpcode() {
	case codec.Ping:
		err = h.OnPing(c, payload)
	case codec.Pong:
		err = h.OnPong(c, payload)
	case codec.Close:
		code, reason := proto.ParseClose(payload)
		if err = h.OnClose(c, code, reason); err == nil {
			err = &CloseError{Code: code, Reason: string(reason)}
		}
	}
	if err != nil {
		c.readErr, c.inMessage = err, false
	}
	return err
}

// fail records a terminal failure. Protocol failures send their close code.
func (c *Conn) fail(err error) error {
	if pe, ok := err.(*proto.Error); ok {
		c.sendClose(pe.Code)
		err = &Error{Code: pe.Code, Err: pe.Err}
	}
	c.readErr, c.inMessage = err, false
	return err
}
