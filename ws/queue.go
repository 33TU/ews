package ws

import (
	"net"
	"sync"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/internal/bufpool"
)

// Queue sends messages asynchronously: Send encodes a message and returns
// without touching the transport, and a goroutine that runs only while the
// queue is nonempty writes what has accumulated, coalescing a burst into one
// write. Prepared messages are queued by reference.
//
// Once a connection has a queue, its other data writes join the queue too:
// Write, WritePrepared, and fragmented sends enqueue their frames in
// submission order and return when those frames have been written, so
// message order and compressed-stream order are preserved and the two
// styles mix freely. Close joins it as well, so the close frame follows
// every message queued before it. Pings and pongs join it too, but Ping and
// Pong return once the frame is queued, so a reader answering pings never
// waits on a slow peer.
//
// A write error fails the queue for good: every later send returns it, and
// the queue closes the transport, so a read in progress returns it too and
// the connection ends even when the peer is silent.
//
// A Queue is safe for concurrent use.
type Queue struct {
	c     *Conn
	limit int

	mu       sync.Mutex
	cond     sync.Cond
	cur      *arena    // Encoded frames of queued messages; nil when none are copied in.
	segments []segment // Queued frames in order.
	size     int       // Bytes queued, counted against limit.
	running  bool
	err      error
	enqueued uint64 // Frames ever enqueued; a frame's sequence number.
	written  uint64 // Frames written so far.
	pong     uint64 // Sequence number of the latest pong. Guarded by wmu.
	// start is q.run as a func value, made once here so that starting the
	// writer allocates nothing: a go statement with a receiver or arguments
	// heap-allocates a closure to carry them, one with none does not.
	start func()

	// Writer-side storage, reused across flushes. bufs keeps the backing
	// array; nb is the header net.Buffers.WriteTo consumes, kept as a field
	// so it does not escape to the heap on every flush.
	flushing      *arena
	flushSegments []segment
	bufs          [][]byte
	nb            net.Buffers
}

// segment is one queued frame: a range of the arena, or the shared bytes of
// a Prepared, which stay alive as long as a segment refers to them.
type segment struct {
	start, end int
	ext        []byte
}

// arena is a frame buffer shared through a pool. A queue holds one while it
// has frames waiting and one while the writer is flushing, and none while
// idle, so memory scales with connections that are busy at the same moment
// rather than with every connection. The handle is reused, so a burst costs
// no allocation.
type arena struct {
	b     []byte
	large *bufpool.Buffer // The size-class buffer b lives in past poolKeep.
}

var arenaPool = sync.Pool{New: func() any { return &arena{b: make([]byte, 0, 4<<10)} }}

// NewQueue attaches a queue to c, or returns the one it already has.
//
// limit is a high-water mark on bytes queued by Send and SendPrepared: an
// empty queue accepts any message, and a message that would push a nonempty
// queue past the limit is refused with ErrQueueFull rather than blocking, so
// a slow peer cannot stall the sender. The application decides what a full
// queue means, usually closing the connection.
//
// Zero means 1 MiB on a new queue and leaves an existing queue's limit
// alone, so a hub can look a connection's queue up again with NewQueue(0); a
// nonzero limit on an existing queue adjusts it, taking effect on the next
// Send.
func (c *Conn) NewQueue(limit int) *Queue {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if q := c.queue; q != nil {
		if limit > 0 {
			q.mu.Lock()
			q.limit = limit
			q.mu.Unlock()
		}
		return q
	}

	if limit <= 0 {
		limit = 1 << 20
	}
	q := &Queue{c: c, limit: limit}
	q.cond.L = &q.mu
	q.start = q.run
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

	_, err = q.enqueue(header, body, nil)
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
	_, err = q.enqueue(nil, nil, frame)
	return err
}

// Err returns the first write error, after which every Send fails with it
// and the transport is closed.
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

// control queues a ping or pong without waiting for it. Both skip the
// limit, except that past it a pong replaces one the writer has not taken
// yet: RFC 6455 allows answering only the most recent ping, and a peer that
// pings without reading then adds one pong rather than one per ping.
// Callers hold wmu.
func (q *Queue) control(op codec.Opcode, header, body []byte) error {
	if op == codec.Pong && q.replacePong(header, body) {
		return nil
	}
	seq, err := q.enqueue(header, body, nil)
	if op == codec.Pong {
		q.pong = seq
	}
	return err
}

// replacePong swaps the latest pong for this one if the queue is past its
// limit and the pong is still waiting. Waiting frames hold the last
// len(segments) sequence numbers. Callers hold wmu.
func (q *Queue) replacePong(header, body []byte) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	after := q.enqueued - q.pong // Frames queued after the pong.
	if q.err != nil || q.size <= q.limit || q.pong == 0 || after >= uint64(len(q.segments)) {
		return false
	}

	i := len(q.segments) - 1 - int(after)
	old := q.segments[i]
	if old.end == len(q.cur.b) {
		q.cur.b = q.cur.b[:old.start] // Reuse its bytes, so repeated swaps do not grow the arena.
	}
	start := len(q.cur.b)
	if need := start + len(header) + len(body); need > cap(q.cur.b) && need > poolKeep {
		q.growLarge(need)
	}
	q.cur.b = append(append(q.cur.b, header...), body...)
	q.segments[i] = segment{start: start, end: len(q.cur.b)}
	q.size += len(q.cur.b) - start - (old.end - old.start)
	return true
}

// enqueue appends one frame, copied from header and body or referenced as
// ext, and returns its sequence number. The limit is reserve's business, so
// synchronous callers that wait for the write and control frames skip it.
// Callers hold wmu, which makes enqueue order the encode order.
func (q *Queue) enqueue(header, body, ext []byte) (uint64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return 0, q.err
	}
	n := len(header) + len(body) + len(ext)
	if ext != nil {
		q.segments = append(q.segments, segment{ext: ext})
	} else {
		if q.cur == nil {
			q.cur = arenaPool.Get().(*arena)
		}
		start := len(q.cur.b)
		if need := start + n; need > cap(q.cur.b) && need > poolKeep {
			q.growLarge(need)
		}
		q.cur.b = append(append(q.cur.b, header...), body...)
		q.segments = append(q.segments, segment{start: start, end: len(q.cur.b)})
	}

	q.size += n
	q.enqueued++
	if !q.running {
		q.running = true
		go q.start()
	}
	return q.enqueued, nil
}

// growLarge moves the producer's arena to size-class storage of at least
// need bytes, which it uses past poolKeep. Kept out of enqueue, where the
// rest of the copy stays inline. Callers hold mu.
func (q *Queue) growLarge(need int) {
	large := bufpool.Get(need)
	large.B = append(large.B, q.cur.b...)
	bufpool.Put(q.cur.large)
	q.cur.large, q.cur.b = large, large.B
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

		// Take the producer's storage for writing, so enqueue continues into
		// fresh storage while the write is in progress.
		q.cur, q.flushing = nil, q.cur
		q.segments, q.flushSegments = q.flushSegments[:0], q.segments
		q.size = 0
		q.mu.Unlock()

		err := q.flush()

		q.mu.Lock()
		for i := range q.flushSegments {
			q.flushSegments[i] = segment{}
		}
		q.written += uint64(len(q.flushSegments))
		if a := q.flushing; a != nil {
			q.flushing = nil
			if a.large == nil {
				a.b = a.b[:0]
				arenaPool.Put(a)
			} else {
				bufpool.Put(a.large)
			}
		}

		if err != nil {
			q.err = err
			q.segments, q.size = q.segments[:0], 0
			q.running = false
		}
		q.cond.Broadcast()
		q.mu.Unlock()
		if err != nil {
			q.closeTransport(err)
			return
		}
	}
}

// closeTransport ends the connection after a write error. Nothing more can
// be sent, not even a close frame, so the transport is closed: the reader's
// next read returns err, and the application's disconnect path runs even if
// the peer is silent. Storing &err in run itself would move err to the heap
// on every flush; here only a failure pays for it.
//
// A transport layered over another, as tls.Conn is, has the one underneath
// closed instead: closing a tls.Conn first sends close_notify under its own
// five-second deadline, which a peer that stopped reading makes it wait out,
// and after a failed write there is nothing left to tell the peer.
func (q *Queue) closeTransport(err error) {
	q.c.failed.Store(&err)
	conn := q.c.conn
	if layered, ok := conn.(interface{ NetConn() net.Conn }); ok {
		conn = layered.NetConn()
	}
	conn.Close()
}

// flush writes the writer-side frames under the transport lock only, so
// enqueue keeps going while a slow peer is being written to: one writev on a
// socket, one coalesced write elsewhere.
func (q *Queue) flush() error {
	c := q.c
	c.iomu.Lock()
	defer c.iomu.Unlock()
	if err := c.armWrite(); err != nil {
		return err
	}
	if len(q.flushSegments) == 1 {
		// One frame, the common case: a plain write costs less than a
		// one-element writev.
		s := q.flushSegments[0]
		frame := s.ext
		if frame == nil {
			frame = q.flushing.b[s.start:s.end]
		}
		_, err := c.conn.Write(frame)
		return err
	}

	if c.vectored {
		q.bufs = q.bufs[:0]
		for _, s := range q.flushSegments {
			if s.ext != nil {
				q.bufs = append(q.bufs, s.ext)
			} else {
				q.bufs = append(q.bufs, q.flushing.b[s.start:s.end])
			}
		}
		q.nb = q.bufs
		_, err := q.nb.WriteTo(c.conn)
		clear(q.bufs) // Drop references to prepared frames.
		return err
	}

	total := 0
	for _, s := range q.flushSegments {
		total += len(s.ext) + s.end - s.start
	}
	// A coalescing buffer holds a burst of small frames; anything larger
	// is served from the size-class pool for the one write.
	buf := writePool.Get().(*[]byte)
	out := (*buf)[:0]
	var large *bufpool.Buffer
	if total > cap(out) {
		large = bufpool.Get(total)
		out = large.B
	}
	for _, s := range q.flushSegments {
		if s.ext != nil {
			out = append(out, s.ext...)
		} else {
			out = append(out, q.flushing.b[s.start:s.end]...)
		}
	}
	_, err := c.conn.Write(out)
	if large != nil {
		bufpool.Put(large)
	} else {
		*buf = out
	}
	writePool.Put(buf)
	return err
}
