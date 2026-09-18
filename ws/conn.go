package ws

import (
	"cmp"
	"io"
	"sync"
	"syscall"

	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
)

// Role identifies the local endpoint.
type Role uint8

// The two roles. A Server reads masked frames and writes unmasked ones; a
// Client does the reverse.
const (
	Server Role = iota
	Client
)

// Defaults for the zero values of the corresponding Config fields.
const (
	DefaultReadBufferSize = 4 << 10
	DefaultMaxMessageSize = 8 << 20
	DefaultFragmentSize   = 64 << 10
	// DefaultMinSize is the payload size below which messages go uncompressed
	// when Compression.MinSize is zero: flate encoders emit literals only for
	// smaller flushed blocks, so compressing them costs CPU and adds bytes.
	DefaultMinSize = 128
)

// poolKeep is the largest buffer returned to a pool. An arena, message
// buffer, decompressor or coalescing buffer that grew past it is dropped
// instead, so a rare huge message or burst does not pin memory in every
// pool it touched.
const poolKeep = 1 << 20

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
	// ValidateUTF8 makes ReadMessage check that text messages are valid
	// UTF-8 and fail the connection with close code 1007 when they are not,
	// as RFC 6455 requires. Off by default: it costs one pass over each text
	// message, and applications that decode text themselves catch invalid
	// input anyway. Read delivers chunks unvalidated either way, and outgoing
	// text is never checked: the sender knows its own data.
	ValidateUTF8 bool
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

	// Transport and configuration, fixed at construction.
	rw           io.ReadWriter
	role         Role
	vectored     bool // rw is a kernel socket, so net.Buffers writes header and payload in one writev.
	limit        int  // MaxMessageSize.
	fragmentSize int  // WriteFrom chunk size.
	validateUTF8 bool // Check text messages in ReadMessage.

	// Read side, used by the one goroutine reading at a time.
	rx        receiver
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
	tx    sender
	queue *Queue // Set by NewQueue; synchronous sends then join it.
	comp  compressorContext
	frag  fragmentState
	out   writeBuffers
}

// NewConn wraps an upgraded transport.
func NewConn(rw io.ReadWriter, cfg Config) (*Conn, error) {
	if rw == nil || cfg.Role != Server && cfg.Role != Client || cfg.ReadBufferSize < 0 || cfg.MaxMessageSize < 0 || cfg.FragmentSize < 0 {
		return nil, ErrInvalidConfig
	}
	comp := cfg.Compression
	if comp != nil && (comp.Level < -2 || comp.Level > 9 || comp.MinSize < 0 || !validBits(comp.SendWindowBits) || !validBits(comp.ReceiveWindowBits)) {
		return nil, ErrInvalidConfig
	}
	c := &Conn{
		ControlHandler: cfg.ControlHandler,
		rw:             rw,
		role:           cfg.Role,
		limit:          cmp.Or(cfg.MaxMessageSize, DefaultMaxMessageSize),
		fragmentSize:   cmp.Or(cfg.FragmentSize, DefaultFragmentSize),
		validateUTF8:   cfg.ValidateUTF8,
		buf:            make([]byte, cmp.Or(cfg.ReadBufferSize, DefaultReadBufferSize)),
	}
	_, c.vectored = rw.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	c.rx.Init(cfg.Role, comp != nil)
	c.tx.Init(cfg.Role)
	if comp != nil {
		c.comp.config = comp
		c.comp.minSize = cmp.Or(comp.MinSize, DefaultMinSize)
		c.comp.shared = cfg.CompressionShared
		c.comp.window = window(comp.SendContextTakeover, comp.SendWindowBits)
		c.decomp.window = window(comp.ReceiveContextTakeover, comp.ReceiveWindowBits)
	}
	return c, nil
}

// window returns a history window of the negotiated size when takeover was
// agreed for that direction, else nil.
func window(takeover bool, bits int) *deflate.Window {
	if !takeover {
		return nil
	}
	return &deflate.Window{Bits: bits}
}

func validBits(b int) bool { return b == 0 || b >= 8 && b <= 15 }
