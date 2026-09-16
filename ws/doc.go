// Package ws provides message I/O over already-upgraded WebSocket connections.
//
// A Conn wraps any io.ReadWriter; the caller keeps the net.Conn for deadlines,
// TLS and closing, and NetConn hands it back as a net.Conn view. The handshake
// is elsewhere: ws takes the negotiated result through Config.
//
// # Reading
//
// One goroutine reads at a time. ReadMessage returns whole messages, borrowed
// until the next read call; NextMessage then Read delivers a message in chunks
// as its frames arrive, with io.EOF at the end; WriteTo relays the rest of a
// message to an io.Writer frame by frame. Compressed messages inflate as they
// stream through Read and WriteTo, holding one frame and the inflater's
// window, and whole through ReadMessage. Control frames are dispatched on the
// way through the ControlHandler, whose default answers pings and echoes
// close codes. A protocol failure sends its close code once and fails every
// later read with the same *Error; the peer's close arrives as *CloseError.
//
// # Writing
//
// Writes may come from any goroutine. Write sends one frame; BeginMessage,
// WriteChunk and EndMessage send a message of unknown length as fragments,
// and WriteFrom does that for an io.Reader. On a kernel socket a frame leaves
// as one writev of header and payload with no copy; elsewhere small frames
// are coalesced into one write. A Queue sends asynchronously through a
// goroutine that runs only while the queue is nonempty and coalesces
// everything accumulated into one write; once a connection has a queue its
// synchronous writes join it in order. Prepared encodes a message once for
// many recipients. Outgoing text is never validated: the sender knows its
// data.
//
// # Borrowing
//
// Payloads from ReadMessage, chunks from Read, and the bytes of a Prepared
// are valid until the next call on the object that produced them and are
// then reused. Nothing enforces this and the failure is silent corruption:
// copy before keeping a payload past the next read or handing it to another
// goroutine.
//
// # Memory
//
// An idle connection holds its read buffer, 4 KiB by default, and nothing
// else; message buffers, compressors and decompressors come from pools and
// go back after use. A Queue holds at most its limit of unwritten bytes, a
// high-water mark that fails Send with ErrQueueFull rather than blocking, so
// a server's bound is connections times limit; a synchronous Write instead
// blocks on the full socket and leaves the backlog in kernel buffers. With
// send context takeover a compressor attached to the connection continues
// one stream at about 800 KB per connection; CompressionShared borrows a
// pooled one per message and primes it from the 32 KB window instead, which
// costs CPU per message and wins once many connections with large messages
// compete for cache.
package ws
