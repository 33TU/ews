package ws

import (
	"net"
	"slices"
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

	// Writer-side storage, reused across flushes. bufs keeps the backing
	// array; nb is the header net.Buffers.WriteTo consumes, kept as a field
	// so it does not escape to the heap on every flush. spare is a private
	// arena displaced by a pooled one, restored when that is released.
	flushArena    []byte
	flushSegments []segment
	spare         []byte
	bufs          [][]byte
	nb            net.Buffers
}

// segment is one queued frame: a range of the arena, or external bytes that
// may belong to a Prepared holding a reference for the segment's lifetime.
type segment struct {
	start, end int
	ext        []byte
	p          *Prepared
}

// queueRetain bounds the arena capacity a queue keeps for itself. Bursts
// beyond it use arenas from a shared pool, returned after the flush, so
// memory scales with the connections flushing at once rather than with every
// connection that ever saw a burst: ten thousand connections that each once
// queued 100 KB would otherwise hold a gigabyte.
const queueRetain = 4 << 10

var arenaPool sync.Pool // *[]byte with capacity above queueRetain.

// grow makes room for n more bytes in the arena. A queue owns a private
// arena of queueRetain bytes, allocated once; a burst that outgrows it
// continues in a pooled arena, and the private one is set aside to come
// back after the flush, so steady traffic allocates nothing.
func (q *Queue) grow(n int) {
	need := len(q.arena) + n
	if need <= cap(q.arena) {
		return
	}
	if cap(q.arena) == 0 && need <= queueRetain {
		q.arena = make([]byte, 0, queueRetain)
		return
	}
	if cap(q.arena) > queueRetain {
		q.arena = slices.Grow(q.arena, n) // Already pooled; let it grow in place.
		return
	}
	if q.spare == nil {
		q.spare = q.arena[:0] // Keep the private arena for after the flush.
	}
	var buf []byte
	if p, ok := arenaPool.Get().(*[]byte); ok {
		buf = (*p)[:0]
	}
	if cap(buf) < need {
		buf = make([]byte, 0, max(need, 64<<10))
	}
	q.arena = append(buf, q.arena...)
}

// release hands a pooled arena back after its flush and restores the private
// one it displaced. Callers hold mu.
func (q *Queue) release() {
	if cap(q.flushArena) > queueRetain {
		a := q.flushArena[:0]
		arenaPool.Put(&a)
		q.flushArena, q.spare = q.spare, nil
	}
	if cap(q.flushSegments) > queueRetain/64 {
		q.flushSegments = nil
	}
	if cap(q.bufs) > queueRetain/64 {
		q.bufs = nil
	}
}

// NewQueue attaches a queue to c, or returns the one it already has. limit
// is a high-water mark on bytes queued by Send and SendPrepared: an empty
// queue accepts any message, and a message that would push a nonempty queue
// past the limit is refused with ErrQueueFull rather than blocking, so a slow
// peer cannot stall the sender. Zero means 1 MiB.
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
	if c.frag.op != 0 {
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
	_, err = q.enqueue(header, body, nil, nil)
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
	_, err = q.enqueue(nil, nil, p, frame)
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

// reserve applies the limit before a message is encoded, since encoding
// advances compressor state that a refused message must not consume. n is
// the payload size, close enough to the frame size for a high-water mark.
// Callers hold wmu.
func (q *Queue) reserve(n int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	if q.size != 0 && q.size+n > q.limit {
		return ErrQueueFull
	}
	return nil
}

// enqueue appends one frame, copied from header and body or referenced as
// ext, and returns its sequence number. The limit is reserve's business, so
// synchronous callers that wait for the write skip it. Callers hold wmu,
// which makes enqueue order the encode order.
func (q *Queue) enqueue(header, body []byte, p *Prepared, ext []byte) (uint64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return 0, q.err
	}
	n := len(header) + len(body) + len(ext)
	if ext != nil {
		if p != nil {
			p.Retain()
		}
		q.segments = append(q.segments, segment{ext: ext, p: p})
	} else {
		q.grow(len(header) + len(body))
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
// connection holds no goroutine. Measured against a lingering writer with a
// timer, a fresh goroutine per burst costs less at every wake rate tried.
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
		q.release()
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
	if len(q.flushSegments) == 1 {
		// One frame, the common case: a plain write costs less than a
		// one-element writev.
		s := q.flushSegments[0]
		frame := s.ext
		if frame == nil {
			frame = q.flushArena[s.start:s.end]
		}
		_, err := c.rw.Write(frame)
		return err
	}
	if c.vectored {
		q.bufs = q.bufs[:0]
		for _, s := range q.flushSegments {
			if s.ext != nil {
				q.bufs = append(q.bufs, s.ext)
			} else {
				q.bufs = append(q.bufs, q.flushArena[s.start:s.end])
			}
		}
		q.nb = q.bufs
		_, err := q.nb.WriteTo(c.rw)
		clear(q.bufs) // Drop references to prepared frames.
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
