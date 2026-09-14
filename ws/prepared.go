package ws

import (
	"sync"
	"sync/atomic"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
)

// Prepared is a message encoded once for sending to many connections. Server
// frames carry no mask, so one encoding serves every recipient; a compressed
// variant is built on first use per compression configuration.
//
// Storage is pooled and reference counted. Prepare returns one reference,
// owned by the caller; Queue.SendPrepared and Batch.WritePrepared take their
// own for as long as they hold the message. Call Release when done handing
// the message out and the storage returns to the pool once every reference is
// dropped. Releasing is optional: an unreleased Prepared is collected
// normally. A Prepared is safe for concurrent use.
type Prepared struct {
	op      codec.Opcode
	refs    atomic.Int32
	frame   []byte // Plain header and payload, contiguous.
	payload []byte // Aliases frame.

	mu       sync.Mutex
	variants []variant // Compressed frames by configuration; the first nvar are live.
	nvar     int
}

type variant struct {
	key   compressKey
	frame []byte
}

type compressKey struct{ level, bits int }

var preparedPool sync.Pool

// preparedPoolLimit keeps very large messages out of the pool so they do not
// pin memory.
const preparedPoolLimit = 1 << 20

// Prepare encodes a text or binary message for WritePrepared. payload is copied.
func Prepare(op codec.Opcode, payload []byte) (*Prepared, error) {
	if op != codec.Text && op != codec.Binary {
		return nil, ErrProtocol
	}
	var enc codec.Encoder
	if err := enc.Encode(true, op, payload, nil); err != nil {
		return nil, err
	}
	p, ok := preparedPool.Get().(*Prepared)
	if !ok {
		p = new(Prepared)
	}
	p.op = op
	p.refs.Store(1)
	header := enc.HeaderBytes()
	p.frame = append(append(p.frame[:0], header...), payload...)
	p.payload = p.frame[len(header):]
	return p, nil
}

// Opcode returns the message type.
func (p *Prepared) Opcode() codec.Opcode { return p.op }

// Payload borrows the message payload.
func (p *Prepared) Payload() []byte { return p.payload }

// Retain adds a reference, for handing the message to another owner that
// will Release it independently.
func (p *Prepared) Retain() { p.refs.Add(1) }

// Release drops the caller's reference. At zero the storage returns to the
// pool and the message must not be used again; releasing more times than
// retained panics, so call it once per Prepare or Retain, or not at all.
func (p *Prepared) Release() {
	n := p.refs.Add(-1)
	if n > 0 {
		return
	}
	if n < 0 {
		panic("ews/ws: Prepared released more times than retained")
	}
	if cap(p.frame) > preparedPoolLimit {
		return
	}
	p.payload = nil
	for i := 0; i < p.nvar; i++ {
		if cap(p.variants[i].frame) > preparedPoolLimit {
			p.variants[i].frame = nil
		}
	}
	p.nvar = 0
	preparedPool.Put(p)
}

// frameFor picks the bytes to send on a connection: the compressed variant
// when the connection compresses and the payload clears MinSize, else plain.
// It reports whether the compressed variant was chosen. Callers hold wmu.
func (p *Prepared) frameFor(c *Conn) ([]byte, bool) {
	if c.comp.config == nil || len(p.payload) < c.comp.minSize {
		return p.frame, false
	}
	return p.compressedFor(c.comp.config), true
}

func (p *Prepared) compressedFor(comp *handshake.Compression) []byte {
	key := compressKey{comp.Level, comp.SendWindowBits}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < p.nvar; i++ {
		if p.variants[i].key == key {
			return p.variants[i].frame
		}
	}
	// Compressed against an empty dictionary, so any peer decodes it, with or
	// without history; a takeover connection then re-primes its compressor.
	pool := compressorPool(comp)
	cp, ok := pool.Get().(*deflate.Compressor)
	if !ok {
		cp = newCompressor(comp)
	}
	if p.nvar == len(p.variants) {
		p.variants = append(p.variants, variant{})
	}
	v := &p.variants[p.nvar]
	v.key = key
	compressed, err := cp.Compress(p.payload, nil)
	if err == nil {
		var enc codec.Encoder
		if err = enc.EncodeCompressed(true, p.op, compressed, nil); err == nil {
			v.frame = append(append(v.frame[:0], enc.HeaderBytes()...), compressed...)
		}
	}
	cp.Reset()
	pool.Put(cp)
	if err != nil {
		// Compression cannot fail on valid input; fall back rather than error.
		v.frame = append(v.frame[:0], p.frame...)
	}
	p.nvar++
	return v.frame
}

// WritePrepared sends a prepared message. On a server it writes the shared
// bytes with no encoding or copying; a client must mask, so it falls back to
// Write. A compressed variant advances the send window like any compressed
// message. On a connection with a Queue the message joins the queue in order
// and WritePrepared returns once it has been written.
func (c *Conn) WritePrepared(p *Prepared) error {
	if c.role == Client {
		return c.Write(p.op, p.payload)
	}
	c.wmu.Lock()
	frame, err := c.preparedFrame(p)
	if err != nil {
		c.wmu.Unlock()
		return err
	}
	if q := c.queue; q != nil {
		seq, err := q.enqueue(nil, nil, p, frame)
		c.wmu.Unlock()
		return await(q, seq, err)
	}
	c.iomu.Lock()
	_, err = c.rw.Write(frame)
	c.iomu.Unlock()
	c.wmu.Unlock()
	return err
}

// preparedFrame checks the connection state and picks the variant to send,
// recording a compressed one in the send window. Callers hold wmu.
func (c *Conn) preparedFrame(p *Prepared) ([]byte, error) {
	if c.frag.op != 0 {
		return nil, ErrMessageOpen
	}
	if c.tx.CloseSent() {
		return nil, ErrClosing
	}
	frame, compressed := p.frameFor(c)
	if compressed && c.comp.window != nil {
		c.notePrepared(p)
	}
	return frame, nil
}
