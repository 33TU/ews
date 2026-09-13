package ws

import (
	"net"
	"sync"

	"github.com/33TU/ews/codec"
)

// Queue sends messages asynchronously: Send encodes a message and returns
// without touching the transport, and a goroutine that runs only while the
// queue is nonempty writes what has accumulated, coalescing a burst into one
// write. Prepared messages are
// queued by reference. While a connection has a queue, its data frames must
// go through it, so that compressed streams reach the wire in encoding
// order; control frames may still be written directly. A Queue is safe for
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

	// Writer-side storage, reused across flushes.
	flushArena    []byte
	flushSegments []segment
	bufs          net.Buffers
}

// segment is one queued frame: a range of the arena or a shared prepared frame.
type segment struct {
	start, end int
	ext        []byte
}

// NewQueue attaches a queue to c. limit bounds queued bytes; Send returns
// ErrQueueFull beyond it rather than blocking, so a slow peer cannot stall
// the sender. Zero means 1 MiB.
func (c *Conn) NewQueue(limit int) *Queue {
	if limit <= 0 {
		limit = 1 << 20
	}
	q := &Queue{c: c, limit: limit}
	q.cond.L = &q.mu
	c.wmu.Lock()
	c.queue = q
	c.wmu.Unlock()
	return q
}

// Send encodes a text or binary message, compressing per the connection's
// settings, and queues it. payload is copied and may be reused at once.
func (q *Queue) Send(op codec.Opcode, payload []byte) error {
	if op != codec.Text && op != codec.Binary {
		return ErrProtocol
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	if q.size+len(payload) > q.limit {
		return ErrQueueFull
	}
	c := q.c
	c.wmu.Lock()
	header, body, err := c.encodeData(op, payload)
	if err != nil {
		c.wmu.Unlock()
		return err
	}
	start := len(q.arena)
	q.arena = append(append(q.arena, header...), body...)
	c.wmu.Unlock()
	q.segments = append(q.segments, segment{start: start, end: len(q.arena)})
	q.size += len(q.arena) - start
	q.wake()
	return nil
}

// SendPrepared queues a prepared message by reference, without copying.
func (q *Queue) SendPrepared(p *Prepared) error {
	c := q.c
	if c.role == Client {
		return q.Send(p.op, p.payload)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	if q.size+len(p.payload) > q.limit {
		return ErrQueueFull
	}
	c.wmu.Lock()
	frame, err := c.preparedFrame(p)
	c.wmu.Unlock()
	if err != nil {
		return err
	}
	q.segments = append(q.segments, segment{ext: frame})
	q.size += len(frame)
	q.wake()
	return nil
}

// Err returns the first write error, after which every Send fails with it.
func (q *Queue) Err() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.err
}

// Pending returns the bytes queued but not yet written.
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

// Wait blocks until the queue has drained or failed.
func (q *Queue) Wait() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.running {
		q.cond.Wait()
	}
	return q.err
}

// wake starts the writer if it is not running. Callers hold mu.
func (q *Queue) wake() {
	if !q.running {
		q.running = true
		go q.run()
	}
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
		// Swap the producer's storage with the writer's, so Send continues
		// while the write is in progress.
		q.arena, q.flushArena = q.flushArena[:0], q.arena
		q.segments, q.flushSegments = q.flushSegments[:0], q.segments
		q.size = 0
		q.mu.Unlock()

		err := q.flush()
		if err != nil {
			q.mu.Lock()
			q.err = err
			q.segments, q.size = q.segments[:0], 0
			q.running = false
			q.cond.Broadcast()
			q.mu.Unlock()
			return
		}
	}
}

// flush writes the writer-side frames under the transport lock only, so Send
// keeps encoding while a slow peer is being written to: one writev on a
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
