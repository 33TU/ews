package ws

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/33TU/ews/codec"
)

// NetConn presents a connection as a net.Conn for tunneling other protocols
// over WebSocket. Each Write sends one message of the given type, Text or
// Binary; Read delivers message payloads in order, continuing into the next
// message as one ends and never spanning a message in a single call. A
// message of the other type closes the connection with code 1003 and fails
// every later Read with ErrUnexpectedType. A peer close with code 1000 or
// 1001 reads as io.EOF; any other close code is returned as the CloseError.
//
// Deadlines and addresses are the transport's when it is a net.Conn, so a
// read deadline interrupts a blocked Read and leaves the connection usable,
// while a write deadline that expires mid-frame ends it, as it would any
// framed protocol. Other transports report errors.ErrUnsupported for
// deadlines and a placeholder address. Close sends a normal close frame and
// closes the transport.
//
// Reads are serialized; writes may come from any goroutine, as on Conn.
func NetConn(c *Conn, op codec.Opcode) net.Conn {
	if op != codec.Text && op != codec.Binary {
		panic("ews/ws: NetConn needs Text or Binary")
	}
	return &netConn{c: c, op: op}
}

// ErrUnexpectedType means a NetConn received a message of the other data type.
var ErrUnexpectedType = errors.New("ews/ws: message of unexpected type")

type netConn struct {
	c  *Conn
	op codec.Opcode

	rmu   sync.Mutex
	inMsg bool  // A message is open and partly delivered.
	err   error // Terminal read state: io.EOF after a normal close.

	closeOnce sync.Once
	closeErr  error
}

func (nc *netConn) Read(p []byte) (int, error) {
	nc.rmu.Lock()
	defer nc.rmu.Unlock()
	if nc.err != nil {
		return 0, nc.err
	}
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if !nc.inMsg {
			op, err := nc.c.NextMessage()
			if err != nil {
				return 0, nc.readErr(err)
			}
			if op != nc.op {
				_ = nc.c.Close(1003, "unexpected message type")
				nc.err = ErrUnexpectedType
				return 0, nc.err
			}
			nc.inMsg = true
		}
		n, err := nc.c.Read(p)
		if err == io.EOF {
			nc.inMsg = false
			if n != 0 {
				return n, nil
			}
			continue // An empty message; move on.
		}
		if err != nil {
			return n, nc.readErr(err)
		}
		return n, nil
	}
}

// readErr maps a normal peer close to io.EOF and keeps terminal errors;
// transport errors such as deadlines pass through and leave state intact.
func (nc *netConn) readErr(err error) error {
	if ce, ok := errors.AsType[*CloseError](err); ok {
		if ce.Code == 1000 || ce.Code == 1001 || ce.Code == NoStatus {
			err = io.EOF
		}
		nc.err = err
		return err
	}
	if _, ok := errors.AsType[*Error](err); ok {
		nc.err = err
	}
	return err
}

func (nc *netConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := nc.c.Write(nc.op, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (nc *netConn) Close() error {
	nc.closeOnce.Do(func() {
		if err := nc.c.Close(1000, ""); err != nil && err != ErrClosing {
			nc.closeErr = err
		}
		if cl, ok := nc.c.rw.(io.Closer); ok {
			if err := cl.Close(); err != nil && nc.closeErr == nil {
				nc.closeErr = err
			}
		}
	})
	return nc.closeErr
}

func (nc *netConn) transport() (net.Conn, bool) {
	t, ok := nc.c.rw.(net.Conn)
	return t, ok
}

func (nc *netConn) LocalAddr() net.Addr {
	if t, ok := nc.transport(); ok {
		return t.LocalAddr()
	}
	return websocketAddr{}
}

func (nc *netConn) RemoteAddr() net.Addr {
	if t, ok := nc.transport(); ok {
		return t.RemoteAddr()
	}
	return websocketAddr{}
}

func (nc *netConn) SetDeadline(t time.Time) error {
	if tr, ok := nc.transport(); ok {
		return tr.SetDeadline(t)
	}
	return errors.ErrUnsupported
}

func (nc *netConn) SetReadDeadline(t time.Time) error {
	if tr, ok := nc.transport(); ok {
		return tr.SetReadDeadline(t)
	}
	return errors.ErrUnsupported
}

func (nc *netConn) SetWriteDeadline(t time.Time) error {
	if tr, ok := nc.transport(); ok {
		return tr.SetWriteDeadline(t)
	}
	return errors.ErrUnsupported
}

// websocketAddr stands in when the transport is not a net.Conn.
type websocketAddr struct{}

func (websocketAddr) Network() string { return "websocket" }
func (websocketAddr) String() string  { return "websocket/unknown-addr" }
