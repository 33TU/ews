# ws design

Message I/O over an already-upgraded WebSocket transport, built on `codec`. The
handshake and extension negotiation live in `handshake`, the connection comes
from `transport`, and `ws` receives the negotiated result through `Config`.
This document states how the library is built and, where a choice was
measured, what the measurement said. Numbers are from the author's machines
and dated in the benchmark results files; the ones here are kept for the
reasoning, not as a scoreboard.

## Principles

- The caller owns the transport: deadlines, `Close`, and TLS are outside `ws`.
- Byte slices in and out. No internal goroutines.
- Chunked reading is the primitive. Whole-message reading is built on it.
- Borrow when possible, copy only when a message spans frames or reads.
- Fragmentation is a wire detail. The API exposes messages, not frames.

## Layering

The core is push-style so that a non-blocking transport could drive it: an
event loop pushes bytes in and never blocks, and everything that is not
transport I/O is written once and shared. That kept the layering honest,
but a reactor package is not planned. The blocking `Conn` ties or beats the
event-loop libraries it was measured against, a second poller beside the
runtime's is awkward to use and to maintain in Go, and TLS cannot be driven
from raw file descriptors without forking the record layer. The layering
stays as is because it costs nothing, not because a reactor is coming.

```
codec, deflate             frame bytes, whole-message compression
ws: receiver, sender       push-style core: Feed(bytes) -> validated frame and message events
ws: Conn                   blocking loop: fill from io.ReadWriter, drive the core
a non-blocking transport   could feed the core from readiness callbacks; not planned
```

The push core owns the protocol rules: mask bit per role, RSV checks,
fragmentation state, control-frame limits, close-code validation, and the
close state machine. Text UTF-8 validation is not a frame rule: a chunk is not
a string, so whoever assembles a message validates it once, in one pass. It emits frame-level
events: a validated header, then unmasked payload chunks borrowed until the
next call. It returns
as an internal package with frame-level output rather than a public API.

`ws.Conn` is thin: each read method is a loop of "ask the core for the next
event, fill from the transport when it has none". The sender side is the same
split: the core validates and encodes, the transport wrapper writes.

## API

```go
type Role uint8 // Server, Client

type Config struct {
	Role              Role
	ReadBufferSize    int  // bytes per transport read; 0 = 4 KiB
	MaxMessageSize    int  // bound for ReadMessage and inflated chunked reads; 0 = 8 MiB
	FragmentSize      int  // WriteFrom chunk and largest single frame it sends; 0 = 64 KiB
	ValidateUTF8      bool // ReadMessage rejects invalid text with 1007; off by default
	Compression       *handshake.Compression // negotiated parameters, or nil
	CompressionShared bool // borrow a pooled compressor per message instead of attaching one
	ControlHandler    ControlHandler // nil: default ping, pong, and close behavior
}

type Conn struct {
	ControlHandler ControlHandler // set before reading
	UserData       any
}

func NewConn(rw io.ReadWriter, cfg Config) (*Conn, error)

// Reading: one goroutine at a time. Three shapes over one core.
func (c *Conn) ReadMessage() (codec.Opcode, []byte, error) // whole message, borrowed until the next read
func (c *Conn) NextMessage() (codec.Opcode, error)         // then Read until io.EOF
func (c *Conn) Read(b []byte) (int, error)                 // chunks as frames arrive; compressed messages inflate as they stream
func (c *Conn) WriteTo(w io.Writer) (int64, error)         // the rest of the message to w, frame by frame

// Writing: any goroutine.
func (c *Conn) Write(op codec.Opcode, payload []byte) error
func (c *Conn) BeginMessage(op codec.Opcode) error // then WriteChunk per frame and EndMessage for FIN
func (c *Conn) WriteChunk(payload []byte) error
func (c *Conn) EndMessage() error
func (c *Conn) WriteFrom(op codec.Opcode, r io.Reader) (int64, error) // fragments of FragmentSize as it reads
func (c *Conn) Ping(payload []byte) error
func (c *Conn) Pong(payload []byte) error
func (c *Conn) Close(code uint16, reason string) error // sends the frame once; the transport stays open
func (c *Conn) CloseSent() bool

// Fan-out and asynchronous sends.
func Prepare(op codec.Opcode, payload []byte) (*Prepared, error) // encoded once, immutable, shared by reference
func (c *Conn) WritePrepared(p *Prepared) error
func (c *Conn) NewQueue(limit int) *Queue // one per connection; returns the existing one
func (q *Queue) Send(op codec.Opcode, payload []byte) error // copies, returns before the write; ErrQueueFull past limit
func (q *Queue) SendPrepared(p *Prepared) error
func (q *Queue) Wait() error // until everything queued so far is written
func (q *Queue) Err() error
func (q *Queue) Pending() int

func NetConn(c *Conn, op codec.Opcode) net.Conn // one message per Write, reads across messages

// ControlHandler is called on the reading goroutine. Handlers may write but must
// not read from c. An error terminates the connection.
type ControlHandler interface {
	OnPing(c *Conn, payload []byte) error
	OnPong(c *Conn, payload []byte) error
	OnClose(c *Conn, code uint16, reason []byte) error
}

type DefaultControlHandler struct{} // embed to override a single method

type Error struct{ Code uint16; Err error }          // terminal protocol failure; the close code was sent
type CloseError struct{ Code uint16; Reason string } // peer's close, returned by reads

var (
	ErrClosing, ErrProtocol, ErrInvalidUTF8, ErrMessageTooLarge, ErrInvalidData,
	ErrInvalidConfig, ErrMessageOpen, ErrNoMessage, ErrQueueFull, ErrUnexpectedType error
)
```

## Transport

`io.ReadWriter`, not `net.Conn`. Nothing inside needs deadlines or `Close`; the
caller keeps the `net.Conn` for those, and `NetConn` hands them back when a
`net.Conn` view is wanted. The encoder exposes header and payload separately
so a server data frame can leave as one writev with no copy; the Write path
below says what happens on transports without writev.

## Read path

One fixed read buffer per connection plus, for `ReadMessage`, a message buffer
taken from a shared pool when assembly is needed and returned at the next
read, so an idle connection holds only its read buffer. That keeps the
working set small under many connections, which showed up as a measurable
gap at 16 KiB with 512 or more connections before pooling.

```go
func (c *Conn) fill() error {
	c.rx.Preserve() // at most a partial header is pending here
	n, err := c.rw.Read(c.buf)
	c.rx.Feed(c.buf[:n])
	// n == 0 && err == nil: io.ErrNoProgress. EOF mid-frame: io.ErrUnexpectedEOF.
}
```

Payload is always drained before the next fill, so `Preserve` copies only a few
header bytes. Buffered complete frames are consumed before the transport is
read again.

Core rules per frame, all failing with 1002 unless noted: mask bit must match
role; RSV2 and RSV3 clear; RSV1 clear until compression is negotiated; known
opcode; control frames final and at most 125 bytes; continuation only while a
message is open; no new data opcode while one is open. Control payloads go to a
fixed 125-byte array.

`NextMessage` drains any remaining payload of the current message, then loops
over core events until a text or binary header arrives, dispatching control
frames as they come.

`Read(b)` fills `b` from the current message:

- Core has buffered payload: copy it into `b`.
- Core has nothing buffered and the current frame has payload remaining: read
  from the transport directly into `b[:min(len(b), remaining)]`, then `Feed`
  and `PayloadN` so the core accounts for it and unmasks in place. Bytes never
  touch the read buffer. Reading at most the remaining payload keeps the next
  frame header out of `b`.
- Frame drained and not final: read the next header, dispatch control frames,
  continue.
- Frame drained and final: return `0, io.EOF`. Data and EOF are never
  returned together, so a caller loop needs no special case.
- Compressed message: the same loop feeds the inflater instead, one borrowed
  frame chunk at a time, and `b` receives inflated bytes; see Compression.

`ReadMessage` is `NextMessage` plus assembly with the size budget. When the
first frame is final and its payload completes in a single core chunk, the
chunk is returned borrowed, with no copy; this is the common case for small
messages. Otherwise chunks are appended to the message buffer, and once nothing is
buffered a remainder at least as large as the read buffer is read from the
transport straight into the message buffer's spare capacity, so a large frame
costs one copy of its first chunk and as few reads as the kernel allows. A frame whose
length exceeds the remaining budget fails with 1009 before its payload is read.
The budget does not apply to `Read`, since the caller controls memory there.
With `ValidateUTF8` set, off by default as in gws, a complete text message
is validated with one pass of `internal/utf8.Valid` and fails with 1007. That package skips the ASCII prefix with 32-byte word
loads, so pure ASCII returns without touching a validator, then checks the
rest with a shift-based DFA by default, or with SIMD lookups ported from
github.com/33TU/json-experiment under `GOEXPERIMENT=simd` on amd64. Against
the standard library, the DFA is 1.4 to 2 times faster on non-ASCII text and
the SIMD kernel 5 to 8 times. `Read` does not validate: chunks of a plain
message are delivered as they arrive, and chunked reads of compressed messages
follow the same rule; gws makes the same choice for its streaming reader.

Control frames complete: call the handler. After `OnClose` returns nil, the
current read returns `*CloseError` and every later read returns it again. An
empty close payload reports code 1005.

Failure: write a close frame carrying the code once, best effort, record the
`*Error`, and return it from every later read.

## Write path

`Write`, `Ping`, `Pong`, and `Close` serialize on a mutex and may be called from
any goroutine, including a control handler on the read goroutine. Steps:

1. Reject after `Close` except `Pong`. Reject control payloads over 125 bytes
   and malformed close payloads.
2. Client role: fresh key from `crypto/rand` per frame; server role: nil key.
3. `Encode`, then send. On a kernel socket, detected by `SyscallConn`,
   `net.Buffers{HeaderBytes(), PayloadBytes()}.WriteTo(rw)` is one writev.
   Elsewhere, such as `*tls.Conn` or a wrapper, a payload up to 16 KiB is
   copied next to its header into a pooled buffer for one write, so a frame
   is one TLS record; larger payloads take two writes, since the copy would
   cost more than it saves. An empty payload writes the header alone.

A burst of messages to one connection goes through its `Queue`, whose
writer coalesces everything accumulated into one write, so a fan-out burst
costs one syscall instead of one per message; a separate synchronous
`Batch` existed for the same purpose and was removed as a second way to do
it. `WriteFrom` streams an
`io.Reader` in `FragmentSize` chunks through the fragmented-send path, and
`WriteTo` is its mirror on the read side: the rest of the current message
goes to an `io.Writer` frame by frame, borrowed from the read buffer or read
into the pooled message buffer in `FragmentSize` pieces, so `io.Copy` moves
a message with no buffer of its own and an echo through the connection's
own streaming shape is `NextMessage` then `WriteFrom(op, c)`.

`NetConn` adapts a connection to `net.Conn` for tunneling, after
coder/websocket's `NetConn`, but simpler because ews never hides the
transport: deadlines and addresses delegate to the underlying `net.Conn`
instead of being rebuilt from timers and contexts, and a read deadline
leaves the connection usable since transport errors are not sticky on the
read side. One message per `Write`, reads continue across messages, the
wrong data type closes with 1003, a normal close reads as `io.EOF`.

`Prepared` encodes a message once for many recipients; server frames carry
no mask, so the bytes are shared, with a compressed variant per compression
configuration built on first use. That variant is compressed against an
empty dictionary, which any peer decodes; on a takeover connection the send
window is advanced and the attached compressor re-primes once on its next
message, through the window generation counter. Advancing the window is a
copy of up to 32 KB per recipient on the sender's goroutine, and it was the
whole cost of a compressed broadcast, so it is deferred: the connection
retains the `Prepared` in a small pending list instead, drops entries the
later ones already cover, and copies the rest only when it next compresses
a message of its own. A recipient that only receives broadcasts never
copies; measured at 512 connections that took compressed 64 KiB broadcasts
from 161k to 465k messages a second. `Queue` is the one place
`ws` runs a goroutine, and only while the queue is nonempty: `Send` encodes
under the write lock into an arena and returns, the writer swaps arenas and
writes everything accumulated in one writev, and a slow peer fails `Send`
with `ErrQueueFull` at the byte limit rather than stalling the sender. Once a
queue exists, synchronous sends join it: they enqueue under the encoder lock,
which makes enqueue order the encode order, then wait for their sequence
number to be written. Order and compressed-stream consistency hold, `Write`
keeps meaning written-on-return, and the encoder lock is never held while
waiting, so `Send` stays non-blocking. The writer goroutine exits when the
queue drains and is started again by the next `Send`; a lingering writer
with a timer was tried and cost more per wake than a fresh goroutine, 4
against 2 percent of CPU in a 2048-connection broadcast. `Prepared` is
nothing else: it is an immutable value the garbage collector keeps alive
while any queue refers to it, which costs one allocation per broadcast
rather than per recipient. A pooled, reference-counted version existed and
was removed: it saved that one allocation at the price of `Retain`,
`Release`, and a double-release panic in user code.

The queue limit is the memory bound to plan around: a server holds at most
connections times limit of unwritten frames, plus arena growth. Measured in
go-websocket-benchmark's rate test at fifty thousand connections, where the
single client falls behind and stops reading echoes, heap in use climbed to
2.1 GB with a 64 KiB limit and fell back to 45 MB the moment the window
ended, because the faster a server drains its input the more echoes it
holds for a peer that is not reading. The same server echoing through
`Write` stayed flat at 345 MB of heap: its read loop blocks on the full
socket and the backlog sits in kernel buffers instead. Choose by need:
`Queue` decouples the sender and bounds memory per connection, `Write`
gives free backpressure. A limit below the burst a peer can leave unread
(16 KiB there) fails `Send` and, in an echo, closes the connection.

Outgoing text is not UTF-8 validated; that is the caller's job.

A protocol failure writes its close frame from the reading goroutine. A peer
that never reads can stall that write, so callers who care set a write
deadline on the underlying `net.Conn`, as they would for any write.

## Close handshake

`Close` only sends the frame. The default `OnClose` echoes the peer's code if
no close has been sent yet. The caller finishes with a deadline:

```go
c.Close(1000, "")
nc.SetReadDeadline(time.Now().Add(5 * time.Second))
for {
	if _, err := c.NextMessage(); err != nil {
		break
	}
}
nc.Close()
```

## Concurrency contract

- One goroutine in `NextMessage`, `Read`, or `ReadMessage` at a time.
- Writers may run concurrently with reads and with each other.
- Control handlers run on the read goroutine and may write.
- A shared `ControlHandler` must tolerate concurrent calls from different
  connections.
- `ReadMessage` payloads are borrowed until the next read; copy
  before handing them to another goroutine.

## Compression

`Config.Compression` takes the negotiated `handshake.Compression`. The core
accepts RSV1 on a first data frame when negotiated. `ReadMessage` assembles
the compressed bytes through FIN and calls `deflate.Decompress`, bounded by
`MaxMessageSize`, so the limit covers both wire and inflated size. `Read` and
`WriteTo` stream: `deflate` offers `Begin` and `Read` over a `ChunkSource`,
and `ws` supplies borrowed frame chunks straight from the core, filling from
the transport and dispatching control frames as it goes, while the inflater
writes into the caller's buffer. A chunked read of a compressed message
therefore holds one frame and the inflater's window, and its first bytes come
out when the first frame lands, as with gorilla and coder; gws inflates whole.
A transport error during a compressed message ends the connection because the
inflater cannot resume. This mode was removed once as the most intricate code
in the library and brought back, because the alternative held wire plus
inflated size per connection and waited for the last frame before the first
byte; it costs the whole-message paths nothing and the chunked path about 3
percent on small messages, and the intricacy is now the price of the feature.
`Write` compresses when `len(payload) >= MinSize`. Takeover state is a 32 KB
`deflate.Window` per direction on the connection; compressors and
decompressors are pooled, one compressor pool per flate level. Priming an
encoder from a window costs about as much as compressing 32 KB, so a
connection with send takeover keeps a compressor attached and continues its
stream, about 800 KB. `CompressionShared` never attaches. The crossover
depends on message size as much as on connection count: attached is 15 to 30
percent faster on messages of a few KB at every connection count measured, and
shared is 20 to 60 percent faster on messages of 16 KiB and up once about a
hundred connections compete for cache, where misses on attached state outweigh
the priming. No automatic switch worked, since partial attachment gave no
middle ground and a cap needs a release path the API cannot guarantee, so the
choice is explicit. A pooled decompressor stays attached until the next read
so borrowed output holds; its per-message dictionary copy is inherent to
klauspost's reader. Reduced windows are honored: a server accepts
`server_max_window_bits` and a client offers `client_max_window_bits`, and
`handshake.Compression. SendWindowBits` selects a pooled
`deflate.NewCompressorWindow` encoder, which fixes the level. Both windows are
sized to the negotiated bits, the receive side from the peer's declared
window, so a 12-bit peer costs 4 KB of history per direction and an eighth of
the per-message dictionary copy. Messages under `MinSize`, 128 bytes by
default, go uncompressed: flate encoders emit flushed blocks that small as
literals, so compressing them only adds bytes. This passes Autobahn 13.3.x and
13.5.x and negotiates compression with a default gws server, whose windows are
12 bits.

## Fragmented send

Fragmented send is `BeginMessage(op)`, `WriteChunk(b)` as non-final
frames, `EndMessage()` as the FIN frame, so the sender never needs to know
which chunk is last. Data writes from other callers get `ErrMessageOpen`
until the message ends; control frames interleave, as the protocol
allows. Compressed fragments continue one deflate stream through
`deflate.CompressChunk`: middle chunks keep their sync-flush tail, only
the last is trimmed, and the connection holds one compressor for the
message even in shared mode. The final compressed fragment carries the
one header byte of the trimmed block, which is inherent to the format.

## Later

Open, each worth a few percent in one cell or some retained memory, to be
done one at a time with alternating before-and-after benchmarks:

- Update the receive history once per message instead of after every 32 KB
  of inflater output; only the last window survives a large message, and the
  mid-message reset for final DEFLATE blocks still needs the history so far.
- Skip the shared-compressor double copy on queue paths: `encodeData` copies
  the borrowed output into connection scratch so the compressor can return
  to the pool, then the queue copies it into its arena.
- Cap what pooled buffers retain: message buffers, decompressor output and
  the coalescing buffers keep their largest capacity; the queue arena cap is
  the precedent. Zero allocations per operation says nothing about retained
  memory after a burst of large messages.
- A cheaper per-connection mask key source than `crypto/rand` if profiling
  shows it matters.

Measured and left out, so they stay out unless the numbers change: a decoder
header scratch to avoid copying a read that completes a split header showed
no gain even when every read split a header; encoding straight into
destination storage only helps the client's masking path; a message-scoped
`io.Writer` would be ergonomics only beside `Read`, `WriteTo` and `WriteFrom`;
and automatic switching between attached and shared compression has no good
rule, since the crossover depends on message size and connection count.
- An epoll or io_uring write pump under `Queue.flush` for plain TCP: raw
  non-blocking writes from a fixed set of goroutines, chosen by the existing
  `SyscallConn` check, with TLS and wrapped transports keeping the goroutine
  writer. Measured as an upper bound at 8 percent more broadcast throughput
  at 512 connections and 14 at 2048, with fewer goroutines in flight. It is
  Linux-only, needs its own stall limit since write deadlines do not cover
  raw writes, and is the last optimization worth doing, if a deployment
  ever shows the profile for it. No user-facing event-loop API in any case.

## Handshake

`handshake` holds the opening handshake rules as pure functions over header
values, so a `net/http` server, a client, and any other transport share them.
The negotiated `handshake.Compression` is what `ws.Config` takes; `ws`
depends on `handshake`, never the reverse. The `transport` package is the only
place `net/http` appears: `Upgrade` validates, hijacks, writes the 101, and
returns the raw connection for `ws.NewConn`; `Dial` opens TCP or TLS, writes
the request, reads the response, confirms it, and returns the same pair, with
`DialOptions` holding the transport concerns the pure package cannot.
`Server` is the third entry point: an accept loop with a small HTTP/1.1
head parser feeding the same `handshake.Negotiate`, one goroutine per
connection, and no `net/http`. Measured in go-websocket-benchmark at ten
thousand connections, `net/http` kept about 10 KB per hijacked connection
alive for the handler's lifetime, its read and write buffers and request
state; `Server` keeps none of it, putting ews below gws on memory there
and 13 percent ahead on accepts per second. `examples/echo` is the Autobahn
target; the suite passes with 6.4.x non-strict by design.

## Testing

Each package has external tests by default and internal ones only where a test
needs unexported state, in their own `*_internal_test.go`. The protocol core
is driven with hand-built frames: round trips across roles and chunk sizes,
fragmentation with interleaved control frames, the invalid-frame table with
its close codes, discard of undrained messages, and transport cases over
`net.Pipe` for split reads, EOF mid-frame, and the close handshake in both
directions. Fault-injecting transports cover the write, queue, fragment and
read failure paths, including a transport dying inside an inflate. Fuzz
targets run against the frame decoder, the encoder round trip, the receiver
and the inflater. `examples/echo` is the Autobahn target and passes all 517
cases, 6.4.x non-strict by design. `just check` runs vet and the race suite
over both modules; statement coverage is about 94 percent, the remainder
being error returns from flate operations that cannot fail on valid input.

`net.Pipe` completes a write only when the peer reads it, so tests must keep a
reader on the other end of every write, including close echoes and the
best-effort close after a failure. Real sockets buffer those.
