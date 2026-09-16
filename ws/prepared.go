package ws

import (
	"sync"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
)

// Prepared is a message encoded once for sending to many connections. Server
// frames carry no mask, so one encoding serves every recipient; a compressed
// variant is built on first use per compression configuration. A Prepared is
// immutable once made, safe for concurrent use, and garbage collected like
// any value: queues and connections that still refer to it keep it alive.
type Prepared struct {
	op      codec.Opcode
	frame   []byte // Plain header and payload, contiguous.
	payload []byte // Aliases frame.

	mu       sync.Mutex
	variants []variant // Compressed frames by configuration.
}

type variant struct {
	key   compressKey
	frame []byte
}

type compressKey struct{ level, bits int }

// Prepare encodes a text or binary message for WritePrepared. payload is copied.
func Prepare(op codec.Opcode, payload []byte) (*Prepared, error) {
	if op != codec.Text && op != codec.Binary {
		return nil, ErrProtocol
	}
	var enc codec.Encoder
	if err := enc.Encode(true, op, payload, nil); err != nil {
		return nil, err
	}
	header := enc.HeaderBytes()
	p := &Prepared{op: op, frame: make([]byte, 0, len(header)+len(payload))}
	p.frame = append(append(p.frame, header...), payload...)
	p.payload = p.frame[len(header):]
	return p, nil
}

// Opcode returns the message type.
func (p *Prepared) Opcode() codec.Opcode { return p.op }

// Payload borrows the message payload.
func (p *Prepared) Payload() []byte { return p.payload }

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
	for i := range p.variants {
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
	v := variant{key: key}
	compressed, err := cp.Compress(p.payload, nil)
	if err == nil {
		var enc codec.Encoder
		if err = enc.EncodeCompressed(true, p.op, compressed, nil); err == nil {
			v.frame = append(append(make([]byte, 0, len(enc.HeaderBytes())+len(compressed)), enc.HeaderBytes()...), compressed...)
		}
	}
	cp.Reset()
	pool.Put(cp)
	if err != nil {
		// Compression cannot fail on valid input; fall back rather than error.
		v.frame = p.frame
	}
	p.variants = append(p.variants, v)
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
		seq, err := q.enqueue(nil, nil, frame)
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
