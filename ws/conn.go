package ws

import (
	"io"
	"sync"
	"syscall"
	"time"

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
	DefaultFragmentSize   = 64 << 10
	// DefaultMinSize is the payload size below which messages go uncompressed
	// when Compression.MinSize is zero: flate encoders emit literals only for
	// smaller flushed blocks, so compressing them costs CPU and adds bytes.
	DefaultMinSize = 128
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

	// Transport and configuration, fixed by Reset.
	rw           io.ReadWriter
	role         Role
	vectored     bool // rw is a kernel socket, so net.Buffers writes header and payload in one writev.
	limit        int  // MaxMessageSize.
	fragmentSize int  // WriteFrom chunk size.

	// Read side, used by the one goroutine reading at a time.
	rx        proto.Receiver
	buf       []byte  // Transport read buffer.
	msg       *msgBuf // ReadMessage assembly, pooled; held until the next read.
	inMessage bool    // NextMessage returned and Read has not reached io.EOF.
	readErr   error   // Terminal read state.
	decomp    decompressorContext

	// Write side. wmu guards encoder and compressor state and is held only
	// briefly; iomu serializes transport writes, taken inside wmu by direct
	// writers and alone by a Queue's writer.
	wmu   sync.Mutex
	iomu  sync.Mutex
	tx    proto.Sender
	queue *Queue // Set by NewQueue; synchronous sends then join it.
	comp  compressorContext
	frag  fragmentState
	out   writeBuffers
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
	if c := cfg.Compression; c != nil && (c.Level < -2 || c.Level > 9 || c.MinSize < 0 || !validBits(c.SendWindowBits) || !validBits(c.ReceiveWindowBits)) || cfg.CompressionIdle < 0 {
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
	c.role = cfg.Role
	_, c.vectored = rw.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	c.ControlHandler = cfg.ControlHandler
	c.comp.config = cfg.Compression
	if cfg.Compression != nil {
		c.comp.minSize = cfg.Compression.MinSize
		if c.comp.minSize == 0 {
			c.comp.minSize = DefaultMinSize
		}
	}
	c.rx.Init(proto.Role(cfg.Role), cfg.Compression != nil)
	c.releaseMsg()
	c.inMessage, c.decomp.streaming = false, false
	c.readErr, c.decomp.srcErr = nil, nil
	c.releaseDecompressor()
	c.decomp.window = window(c.decomp.window, cfg.Compression != nil && cfg.Compression.ReceiveContextTakeover, bits(cfg.Compression, false))

	c.wmu.Lock()
	c.tx.Init(proto.Role(cfg.Role))
	c.frag.op, c.frag.heldCompressor = 0, false
	c.queue = nil
	c.releaseCompressor()
	if c.comp.releaseTimer != nil {
		c.comp.releaseTimer.Stop()
	}
	c.comp.releaseAfter = cfg.CompressionIdle
	c.comp.shared = cfg.CompressionShared
	c.comp.window = window(c.comp.window, cfg.Compression != nil && cfg.Compression.SendContextTakeover, bits(cfg.Compression, true))
	c.wmu.Unlock()
	return nil
}

// window returns a cleared history window of the negotiated size when wanted,
// reusing w, else nil.
func window(w *deflate.Window, wanted bool, bits int) *deflate.Window {
	if !wanted {
		return nil
	}
	if w == nil {
		w = new(deflate.Window)
	}
	w.Reset()
	w.Bits = bits
	return w
}

func bits(c *handshake.Compression, send bool) int {
	if c == nil {
		return 0
	}
	if send {
		return c.SendWindowBits
	}
	return c.ReceiveWindowBits
}

func validBits(b int) bool { return b == 0 || b >= 8 && b <= 15 }
