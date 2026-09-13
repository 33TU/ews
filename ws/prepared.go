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
// immutable and safe for concurrent use.
type Prepared struct {
	op      codec.Opcode
	plain   []byte // Header and payload, contiguous.
	payload []byte // Aliases plain.

	mu         sync.Mutex
	compressed map[compressKey][]byte
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
	frame := make([]byte, 0, len(header)+len(payload))
	frame = append(append(frame, header...), payload...)
	return &Prepared{op: op, plain: frame, payload: frame[len(header):]}, nil
}

// Opcode returns the message type.
func (p *Prepared) Opcode() codec.Opcode { return p.op }

// Payload borrows the message payload.
func (p *Prepared) Payload() []byte { return p.payload }

// frameFor picks the bytes to send on a connection: the compressed variant
// when the connection compresses and the payload clears MinSize, else plain.
// It reports whether the compressed variant was chosen. Callers hold wmu.
func (p *Prepared) frameFor(c *Conn) ([]byte, bool) {
	if c.compression == nil || len(p.payload) < c.minSize {
		return p.plain, false
	}
	return p.compressedFor(c.compression), true
}

func (p *Prepared) compressedFor(comp *handshake.Compression) []byte {
	key := compressKey{comp.Level, comp.SendWindowBits}
	p.mu.Lock()
	defer p.mu.Unlock()
	if frame, ok := p.compressed[key]; ok {
		return frame
	}
	// Compressed against an empty dictionary, so any peer decodes it, with or
	// without history; a takeover connection then re-primes its compressor.
	pool := compressorPool(comp)
	cp, ok := pool.Get().(*deflate.Compressor)
	if !ok {
		cp = newCompressor(comp)
	}
	compressed, err := cp.Compress(p.payload, nil)
	var frame []byte
	if err == nil {
		var enc codec.Encoder
		if err = enc.EncodeCompressed(true, p.op, compressed, nil); err == nil {
			header := enc.HeaderBytes()
			frame = make([]byte, 0, len(header)+len(compressed))
			frame = append(append(frame, header...), compressed...)
		}
	}
	cp.Reset()
	pool.Put(cp)
	if err != nil {
		frame = p.plain // Compression cannot fail on valid input; fall back rather than error.
	}
	if p.compressed == nil {
		p.compressed = map[compressKey][]byte{}
	}
	p.compressed[key] = frame
	return frame
}

// WritePrepared sends a prepared message. On a server it writes the shared
// bytes with no encoding or copying; a client must mask, so it falls back to
// Write. A compressed variant advances the send window like any compressed
// message.
func (c *Conn) WritePrepared(p *Prepared) error {
	if c.role == Client {
		return c.Write(p.op, p.payload)
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.queue != nil {
		return ErrQueued
	}
	frame, err := c.preparedFrame(p)
	if err != nil {
		return err
	}
	c.iomu.Lock()
	_, err = c.rw.Write(frame)
	c.iomu.Unlock()
	return err
}

// preparedFrame checks the connection state and picks the variant to send,
// recording a compressed one in the send window. Callers hold wmu.
func (c *Conn) preparedFrame(p *Prepared) ([]byte, error) {
	if c.fragOp != 0 {
		return nil, ErrMessageOpen
	}
	if c.tx.CloseSent() {
		return nil, ErrClosing
	}
	frame, compressed := p.frameFor(c)
	if compressed && c.sendWindow != nil {
		c.sendWindow.Add(p.payload)
	}
	return frame, nil
}
