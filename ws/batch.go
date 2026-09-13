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
	op       codec.Opcode
	payload  []byte
	prepared *Prepared
}

// NewBatch returns an empty batch for c.
func (c *Conn) NewBatch() *Batch { return &Batch{c: c} }

// Write queues one text or binary message. payload must stay unchanged until Flush.
func (b *Batch) Write(op codec.Opcode, payload []byte) error {
	if op != codec.Text && op != codec.Binary {
		return ErrProtocol
	}
	b.msgs = append(b.msgs, batchMsg{op: op, payload: payload})
	return nil
}

// WritePrepared queues a prepared message, holding a reference to it until
// Flush; on a server its shared bytes are copied into the batch's single write.
func (b *Batch) WritePrepared(p *Prepared) {
	p.Retain()
	b.msgs = append(b.msgs, batchMsg{prepared: p})
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
		for _, m := range b.msgs {
			if m.prepared != nil {
				m.prepared.Release()
			}
		}
		clear(b.msgs) // Drop payload references.
		b.msgs = b.msgs[:0]
		b.arena = b.arena[:0]
	}()
	c := b.c
	c.wmu.Lock()
	if c.frag.op != 0 {
		c.wmu.Unlock()
		return ErrMessageOpen
	}
	for _, m := range b.msgs {
		if m.prepared != nil && c.role == Server {
			frame, err := c.preparedFrame(m.prepared)
			if err != nil {
				c.wmu.Unlock()
				return err
			}
			b.arena = append(b.arena, frame...)
			continue
		}
		op, payload := m.op, m.payload
		if m.prepared != nil {
			op, payload = m.prepared.op, m.prepared.payload
		}
		header, body, err := c.encodeData(op, payload)
		if err != nil {
			c.wmu.Unlock()
			return err
		}
		b.arena = slices.Grow(b.arena, len(header)+len(body))
		b.arena = append(append(b.arena, header...), body...)
	}
	if q := c.queue; q != nil {
		// The arena is reused only after this returns, so it can be referenced.
		seq, err := q.enqueue(nil, nil, nil, b.arena)
		c.wmu.Unlock()
		return await(q, seq, err)
	}
	c.iomu.Lock()
	_, err := c.rw.Write(b.arena)
	c.iomu.Unlock()
	c.wmu.Unlock()
	return err
}
