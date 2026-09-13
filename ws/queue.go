package ws

import (
	"net"
	"sync"

	"github.com/33TU/ews/codec"
)

// Queue sends messages asynchronously: Send encodes a message and returns
// without touching the transport, and a goroutine that runs only while the
// queue is nonempty writes what has accumulated, coalescing a burst into one
// write. Prepared messages are queued by reference.
//
// Once a connection has a queue, its other data writes join the queue too:
// Write, WritePrepared, Batch.Flush, and fragmented sends enqueue their
// frames in submission order and return when those frames have been written,
// so message order and compressed-stream order are preserved and the two
// styles mix freely. Control frames bypass the queue. A Queue is safe for
// concurrent use.
type Queue struct {
	c     *Conn
	limit int

	mu       sync.Mutex
	cond     sync.Cond
	arena    []byte    // Encoded frames of queued messages.
	segments []segment // Queued frames in order.
	size     int       // Bytes queued, counted against limit.
	running  bool
	err      error
	enqueued uint64 // Frames ever enqueued; a frame's sequence number.
	written  uint64 // Frames written so far.

	// Writer-side storage, reused across flushes.
	flushArena    []byte
	flushSegments []segment
	bufs          net.Buffers
}

// segment is one queued frame: a range of the arena, or external bytes that
// may belong to a Prepared holding a reference for the segment's lifetime.
type segment struct {
	start, end int
	ext        []byte
	p          *Prepared
}

// NewQueue attaches a queue to c, or returns the one it already has. limit
// bounds bytes queued by Send; beyond it Send returns ErrQueueFull rather
// than blocking, so a slow peer cannot stall the sender. Zero means 1 MiB.
func (c *Conn) NewQueue(limit int) *Queue {
	if limit <= 0 {
		limit = 1 << 20
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.queue != nil {
		return c.queue
	}
	q := &Queue{c: c, limit: limit}
	q.cond.L = &q.mu
	c.queue = q
	return q
}

// Send encodes a text or binary message, compressing per the connection's
// settings, and queues it. payload is copied and may be reused at once.
func (q *Queue) Send(op codec.Opcode, payload []byte) error {
	if op != codec.Text && op != codec.Binary {
		return ErrProtocol
	}
	c := q.c
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.fragOp != 0 {
		return ErrMessageOpen
	}
	// Check the limit before encoding: an encode advances compressor state,
	// which must not happen for a frame that is then dropped.
	if err := q.reserve(len(payload)); err != nil {
		return err
	}
	header, body, err := c.encodeData(op, payload)
	if err != nil {
		return err
	}
	_, err = q.enqueue(header, body, nil, nil, false)
	return err
}

// SendPrepared queues a prepared message by reference, without copying,
// holding a reference to it until it has been written.
func (q *Queue) SendPrepared(p *Prepared) error {
	c := q.c
	if c.role == Client {
		return q.Send(p.op, p.payload)
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := q.reserve(len(p.payload)); err != nil {
		return err
	}
	frame, err := c.preparedFrame(p)
	if err != nil {
		return err
	}
	_, err = q.enqueue(nil, nil, p, frame, false)
	return err
}

// Err returns the first write error, after which every Send fails with it.
func (q *Queue) Err() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.err
}

// Pending returns the bytes queued but not yet handed to the transport.
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

// Wait blocks until every queued frame has been written or the queue failed.
func (q *Queue) Wait() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.waitLocked(q.enqueued)
}

// reserve fails when n more bytes would exceed the limit. Callers hold wmu.
func (q *Queue) reserve(n int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	if q.size+n > q.limit {
		return ErrQueueFull
	}
	return nil
}

// enqueue appends one frame, copied from header and body or referenced as
// ext, and returns its sequence number. force skips the limit, for
// synchronous callers that wait for the write anyway. Callers hold wmu, which
// makes enqueue order the encode order.
func (q *Queue) enqueue(header, body []byte, p *Prepared, ext []byte, force bool) (uint64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return 0, q.err
	}
	n := len(header) + len(body) + len(ext)
	if !force && q.size+n > q.limit {
		return 0, ErrQueueFull
	}
	if ext != nil {
		if p != nil {
			p.Retain()
		}
		q.segments = append(q.segments, segment{ext: ext, p: p})
	} else {
		start := len(q.arena)
		q.arena = append(append(q.arena, header...), body...)
		q.segments = append(q.segments, segment{start: start, end: len(q.arena)})
	}
	q.size += n
	q.enqueued++
	if !q.running {
		q.running = true
		go q.run()
	}
	return q.enqueued, nil
}

// await completes a synchronous write: a nil queue means it was written
// directly, otherwise wait for its sequence number.
func await(q *Queue, seq uint64, err error) error {
	if err != nil || q == nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.waitLocked(seq)
}

// waitLocked blocks until frame seq has been written or the queue failed. Callers hold mu.
func (q *Queue) waitLocked(seq uint64) error {
	for q.written < seq && q.err == nil {
		q.cond.Wait()
	}
	return q.err
}

// run writes queued frames until the queue is empty, then exits, so an idle
// connection holds no goroutine.
func (q *Queue) run() {
	for {
		q.mu.Lock()
		if len(q.segments) == 0 || q.err != nil {
			q.running = false
			q.cond.Broadcast()
			q.mu.Unlock()
			return
		}
		// Swap the producer's storage with the writer's, so enqueue continues
		// while the write is in progress.
		q.arena, q.flushArena = q.flushArena[:0], q.arena
		q.segments, q.flushSegments = q.flushSegments[:0], q.segments
		q.size = 0
		q.mu.Unlock()

		err := q.flush()

		q.mu.Lock()
		for i := range q.flushSegments {
			if p := q.flushSegments[i].p; p != nil {
				p.Release()
			}
			q.flushSegments[i] = segment{}
		}
		q.written += uint64(len(q.flushSegments))
		if err != nil {
			q.err = err
			for i := range q.segments {
				if p := q.segments[i].p; p != nil {
					p.Release()
				}
			}
			q.segments, q.size = q.segments[:0], 0
			q.running = false
		}
		q.cond.Broadcast()
		q.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// flush writes the writer-side frames under the transport lock only, so
// enqueue keeps going while a slow peer is being written to: one writev on a
// socket, one coalesced write elsewhere.
func (q *Queue) flush() error {
	c := q.c
	c.iomu.Lock()
	defer c.iomu.Unlock()
	if c.vectored {
		q.bufs = q.bufs[:0]
		for _, s := range q.flushSegments {
			if s.ext != nil {
				q.bufs = append(q.bufs, s.ext)
			} else {
				q.bufs = append(q.bufs, q.flushArena[s.start:s.end])
			}
		}
		_, err := q.bufs.WriteTo(c.rw)
		clear(q.bufs)
		return err
	}
	buf := writePool.Get().(*[]byte)
	defer writePool.Put(buf)
	out := (*buf)[:0]
	for _, s := range q.flushSegments {
		if s.ext != nil {
			out = append(out, s.ext...)
		} else {
			out = append(out, q.flushArena[s.start:s.end]...)
		}
	}
	*buf = out
	_, err := c.rw.Write(out)
	return err
}
