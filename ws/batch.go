package ws

import (
	"slices"

	"github.com/33TU/ews/codec"
)

// Batch collects messages and sends them with one write. Fan-out and
// bursty senders pay one syscall per Flush instead of one per message.
// Payloads are borrowed until Flush; a Batch is reusable and belongs to one
// goroutine at a time.
type Batch struct {
	c     *Conn
	msgs  []batchMsg
	arena []byte // Encoded frames of the current Flush.
}

type batchMsg struct {
	op      codec.Opcode
	payload []byte
}

// NewBatch returns an empty batch for c.
func (c *Conn) NewBatch() *Batch { return &Batch{c: c} }

// Write queues one text or binary message. payload must stay unchanged until Flush.
func (b *Batch) Write(op codec.Opcode, payload []byte) error {
	if op != codec.Text && op != codec.Binary {
		return ErrProtocol
	}
	b.msgs = append(b.msgs, batchMsg{op, payload})
	return nil
}

// Len returns the number of queued messages.
func (b *Batch) Len() int { return len(b.msgs) }

// Flush sends the queued messages in order with a single write and empties the
// batch. Compression follows the connection's settings per message. An
// encoding error sends nothing; a transport error may have sent a prefix.
func (b *Batch) Flush() error {
	if len(b.msgs) == 0 {
		return nil
	}
	defer func() {
		clear(b.msgs) // Drop payload references.
		b.msgs = b.msgs[:0]
		b.arena = b.arena[:0]
	}()
	c := b.c
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.fragOp != 0 {
		return ErrMessageOpen
	}

	var comp = (*compressorLease)(nil)
	compressed := false
	for _, m := range b.msgs {
		payload := m.payload
		useCompression := c.compression != nil && len(payload) >= c.compression.MinSize
		if useCompression {
			if comp == nil {
				comp = c.leaseCompressor()
				defer comp.release()
			}
			var err error
			if payload, err = comp.Compress(payload, c.sendWindow); err != nil {
				return err
			}
			compressed = true
		}
		var header, body []byte
		var err error
		if useCompression {
			header, body, err = c.tx.EncodeCompressed(m.op, payload)
		} else {
			header, body, err = c.tx.Encode(m.op, payload)
		}
		if err != nil {
			return err
		}
		b.arena = slices.Grow(b.arena, len(header)+len(body))
		b.arena = append(append(b.arena, header...), body...)
	}
	if _, err := c.rw.Write(b.arena); err != nil {
		return err
	}
	if compressed {
		c.noteCompressed()
	}
	return nil
}
