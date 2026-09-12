package ws

import (
	"io"
	"net"
	"sync"

	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/internal/proto"
)

// Role identifies the local endpoint.
type Role uint8

const (
	Server Role = iota
	Client
)

const (
	DefaultReadBufferSize = 4 << 10
	DefaultMaxMessageSize = 8 << 20
	// NoStatus is the close code reported when the peer sent none.
	NoStatus = proto.NoStatus
)

// Config describes the local endpoint and negotiated parameters.
type Config struct {
	Role Role
	// ReadBufferSize is the transport read size. Zero uses DefaultReadBufferSize.
	ReadBufferSize int
	// MaxMessageSize bounds ReadMessage. Zero uses DefaultMaxMessageSize.
	MaxMessageSize int
	// Compression must be nil; permessage-deflate is not supported yet.
	Compression *handshake.Compression
	// ControlHandler replaces the defaults. Nil uses DefaultControlHandler.
	ControlHandler ControlHandler
}

// Conn is a WebSocket connection over an upgraded transport.
// One goroutine may read at a time; writes may come from any goroutine.
type Conn struct {
	// ControlHandler overrides default control handling. Set before reading.
	ControlHandler ControlHandler

	// UserData holds application state. Concurrent access is the caller's responsibility.
	UserData any

	rw    io.ReadWriter
	limit int

	rx        proto.Receiver
	buf       []byte // Transport read buffer.
	msg       []byte // ReadMessage assembly.
	inMessage bool   // NextMessage returned and Read has not reached io.EOF.
	readErr   error  // Terminal read state.

	wmu    sync.Mutex
	tx     proto.Sender
	bufArr [2][]byte // Backing storage for bufs; WriteTo consumes the slice.
	bufs   net.Buffers
}

// NewConn wraps an upgraded transport.
func NewConn(rw io.ReadWriter, cfg Config) (*Conn, error) {
	c := new(Conn)
	if err := c.Reset(rw, cfg); err != nil {
		return nil, err
	}
	return c, nil
}

// Reset prepares c for a new transport, retaining storage.
// The previous transport must no longer be in use by any goroutine.
func (c *Conn) Reset(rw io.ReadWriter, cfg Config) error {
	if rw == nil || cfg.Role != Server && cfg.Role != Client || cfg.ReadBufferSize < 0 || cfg.MaxMessageSize < 0 || cfg.Compression != nil {
		return ErrInvalidConfig
	}
	size := cfg.ReadBufferSize
	if size == 0 {
		size = DefaultReadBufferSize
	}
	c.limit = cfg.MaxMessageSize
	if c.limit == 0 {
		c.limit = DefaultMaxMessageSize
	}
	if cap(c.buf) < size {
		c.buf = make([]byte, size)
	}
	c.buf = c.buf[:size]

	c.rw = rw
	c.ControlHandler = cfg.ControlHandler
	c.rx.Init(proto.Role(cfg.Role))
	c.msg = c.msg[:0]
	c.inMessage = false
	c.readErr = nil

	c.wmu.Lock()
	c.tx.Init(proto.Role(cfg.Role))
	c.wmu.Unlock()
	return nil
}
