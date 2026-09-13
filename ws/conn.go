package ws

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
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
	DefaultFragmentSize   = 128 << 10
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
	// FragmentSize is the chunk WriteFrom reads and sends per frame, and so
	// the largest content it sends as a single frame. Larger fragments mean
	// fewer frames and syscalls; two buffers of this size are in flight per
	// call. Zero uses DefaultFragmentSize.
	FragmentSize int
	// Compression holds negotiated permessage-deflate parameters, or nil.
	Compression *handshake.Compression
	// CompressionShared borrows a compressor from the shared pool for every
	// compressed write instead of keeping one attached to the connection.
	// With send context takeover an attached compressor continues one
	// stream and is the fastest per message, but holds about 800 KB per
	// connection; a shared one is primed from the 32 KB window on each
	// message and costs more CPU per message but almost no memory, which
	// wins once thousands of connections compete for cache. Without send
	// context takeover compressors are always shared.
	CompressionShared bool
	// CompressionIdle releases an attached compressor to the shared pool
	// after this long without a compressed write, keeping only the window;
	// the next write re-primes once. Zero never releases.
	CompressionIdle time.Duration
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

	rw           io.ReadWriter
	vectored     bool // rw is a kernel socket, so net.Buffers writes header and payload in one writev.
	limit        int
	fragmentSize int // WriteFrom chunk size.
	compression  *handshake.Compression
	shared       bool // Never attach a compressor.

	rx        proto.Receiver
	buf       []byte  // Transport read buffer.
	msg       *msgBuf // ReadMessage assembly, pooled; held until the next read.
	inMessage bool    // NextMessage returned and Read has not reached io.EOF.
	readErr   error   // Terminal read state.
	srcErr    error   // Error raised while feeding the inflater.

	decompressor *deflate.Decompressor // Pooled; attached until the next read so borrowed output holds.
	recvWindow   *deflate.Window       // Receive-direction history when takeover is negotiated.
	inflating    bool                  // Read is streaming the current message through the inflater.

	wmu          sync.Mutex
	tx           proto.Sender
	sendWindow   *deflate.Window     // Send-direction history when takeover is negotiated.
	compressor   *deflate.Compressor // Attached while continuing sendWindow's stream.
	fragOp       codec.Opcode        // Open fragmented message's opcode, or zero.
	fragFirst    bool                // The next fragment carries fragOp.
	fragCompress bool                // The open fragmented message is compressed.
	fragHeld     bool                // The compressor was taken for the message in shared mode.
	idle         time.Duration
	idleTimer    *time.Timer
	lastCompress time.Time
	bufArr       [2][]byte // Backing storage for bufs; WriteTo consumes the slice.
	bufs         net.Buffers
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
	if rw == nil || cfg.Role != Server && cfg.Role != Client || cfg.ReadBufferSize < 0 || cfg.MaxMessageSize < 0 || cfg.FragmentSize < 0 {
		return ErrInvalidConfig
	}
	if c := cfg.Compression; c != nil && (c.Level < -2 || c.Level > 9 || c.MinSize < 0 || c.SendWindowBits != 0 && (c.SendWindowBits < 8 || c.SendWindowBits > 15)) || cfg.CompressionIdle < 0 {
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
	c.fragmentSize = cfg.FragmentSize
	if c.fragmentSize == 0 {
		c.fragmentSize = DefaultFragmentSize
	}
	if cap(c.buf) < size {
		c.buf = make([]byte, size)
	}
	c.buf = c.buf[:size]

	c.rw = rw
	_, c.vectored = rw.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	c.ControlHandler = cfg.ControlHandler
	c.compression = cfg.Compression
	c.rx.Init(proto.Role(cfg.Role), cfg.Compression != nil)
	c.releaseMsg()
	c.inMessage, c.inflating = false, false
	c.readErr, c.srcErr = nil, nil
	c.releaseDecompressor()
	c.recvWindow = window(c.recvWindow, cfg.Compression != nil && cfg.Compression.ReceiveContextTakeover)

	c.wmu.Lock()
	c.tx.Init(proto.Role(cfg.Role))
	c.fragOp, c.fragHeld = 0, false
	c.releaseCompressor()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
	c.idle = cfg.CompressionIdle
	c.shared = cfg.CompressionShared
	c.sendWindow = window(c.sendWindow, cfg.Compression != nil && cfg.Compression.SendContextTakeover)
	c.wmu.Unlock()
	return nil
}

// window returns a cleared history window when wanted, reusing w, else nil.
func window(w *deflate.Window, wanted bool) *deflate.Window {
	if !wanted {
		return nil
	}
	if w == nil {
		return new(deflate.Window)
	}
	w.Reset()
	return w
}
