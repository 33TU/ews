# ews

A lightweight WebSocket frame encoder and decoder for Go, built around plain `[]byte`.

**Work in progress.** The API is still taking shape.

The frame API lives in `github.com/33TU/ews/codec`; message compression lives in `github.com/33TU/ews/deflate`. The root is reserved for the planned client/server API.

## Design

- Byte-oriented API without requiring `io.Reader` or `io.Writer`.
- Incremental decoding: input can contain partial frames or multiple frames.
- Borrow input when possible; buffer incomplete frames when needed.
- Keep networking and message assembly separate from frame encoding and decoding.

## Decoding

Read each header, then drain its payload before advancing to the next frame.
For example, with a `readChunk` function supplying incoming bytes:

```go
var dec codec.Decoder

for {
    header, ok, err := dec.NextHeader()
    if err != nil {
        return err
    }
    if !ok {
        chunk, err := readChunk()
        if err != nil {
            return err
        }
        dec.Feed(chunk)
        continue
    }

    handleHeader(header)
    var key [4]byte
    if header.Masked() {
        copy(key[:], header.MaskKey())
    }
    var offset uint8
    for {
        payload, done := dec.Payload()
        if len(payload) != 0 {
            if header.Masked() {
                offset = codec.Mask(payload, key, offset)
            }
            // Process before calling the decoder again.
            handlePayload(payload)
        }
        if done {
            break
        }
        chunk, err := readChunk()
        if err != nil {
            return err
        }
        dec.Feed(chunk)
    }
}
```

`Feed` may borrow the supplied slice. Keep those bytes unchanged until consumed or until the next `Feed` call returns.

Returned payloads reference decoder input or storage. Copy payloads you need to retain beyond the next decoder call.

Payload bytes remain masked when the header's mask bit is set. Use `Mask` to unmask in place, carrying its returned offset between chunks and starting at zero for each frame. This modifies the borrowed input; copy first if you need to preserve it. Headers are returned by value and remain valid across decoder calls.

`NextHeader` rejects invalid payload-length encodings. Other protocol checks, including reserved bits, opcodes, control-frame rules, and connection-specific masking requirements, belong to the caller.

Drain available payload bytes before feeding more input to avoid unnecessary buffering. `Payload` returns `nil, true` before the first header and after payload completion, including empty frames.

Call `Reset` to discard pending input while retaining reusable storage.

## Encoding

Encode a frame, then send its header followed by its payload:

```go
var enc codec.Encoder

if err := enc.Encode(true, codec.Text, []byte("Hello"), nil); err != nil {
    return err
}
header := enc.HeaderBytes()
payload := enc.PayloadBytes()
// Send both slices completely before calling Encode or Reset again.
```

`Encode` borrows unmasked input without copying. Keep that input unchanged until it has been sent. With a mask key, it copies and masks the payload into reusable scratch storage, leaving input unchanged. The key is a `*[4]byte`; nil means unmasked. Client frames need a fresh key from `crypto/rand` for each frame; server frames use nil.

Invalid opcodes and malformed control frames return errors without changing output. Message sequencing, UTF-8, and close status codes remain the caller's responsibility.

The output slices are borrowed. Each successful `Encode` replaces the current frame; it does not accumulate frames. Call `Reset` to clear the frame while retaining scratch capacity, or encode the next frame directly after sending the current one.

## Compression

The optional `ews/deflate` package uses `github.com/klauspost/compress/flate` for `permessage-deflate`. Both context-takeover modes are supported, using the default 32 KB window. The default is no context takeover; negotiate `no_context_takeover` for each direction using that mode. Extension negotiation remains outside this package.

Compress a complete message before framing and masking it:

```go
import (
    "github.com/33TU/ews/codec"
    "github.com/33TU/ews/deflate"
    "github.com/klauspost/compress/flate"
)

// Create once and reuse across messages.
compressor, err := deflate.NewCompressor(flate.BestSpeed)
if err != nil {
    return err
}
compressed, err := compressor.Compress([]byte("Hello"), nil)
if err != nil {
    return err
}
var enc codec.Encoder
if err := enc.EncodeCompressed(true, codec.Text, compressed, nil); err != nil {
    return err
}
// Send enc.HeaderBytes(), then enc.PayloadBytes().
```

With `ws`, compression is a matter of passing the negotiated parameters through: `ws.Config{Compression: res.Compression}` from the handshake result. `Write` then compresses messages of at least `MinSize` bytes, `ReadMessage` decompresses within `MaxMessageSize`, and `Read` inflates as the message streams. Decompressors are shared across connections. So are compressors, except that a connection with send context takeover keeps one attached, about 800 KB, so its messages continue one stream at full speed. Servers with thousands of compressed connections set `Config.CompressionShared` to borrow a pooled compressor per message instead, trading some CPU per message for almost no memory per connection; `Config.CompressionIdle` is the middle ground, releasing an attached compressor after that long without a write.

`EncodeCompressed` takes already-compressed bytes. It sets RSV1 on text/binary frames, leaves it clear on continuation frames, and rejects control frames. To fragment a compressed message, split the compressed bytes and encode the pieces with their own masking keys.

On receipt, use `header.RSV1()` on the first data frame to identify a compressed message. Unmask each frame, collect its data fragments through FIN, then decompress the assembled payload. Interleaved control frames are handled separately.

```go
var decompressor deflate.Decompressor // Reuse across messages.
message, err := decompressor.Decompress(compressed, 8<<20, nil) // Maximum 8 MiB output.
if err != nil {
    return err
}
handle(message)
```

Both helpers return borrowed output valid until their next call or `Reset()`, carry no state between messages, and can serve many connections in turn. The frame decoder continues to expose raw payloads.

Context takeover state is a `deflate.Window`, the last 32 KB of payload in one direction. Give each direction of a connection its own window and pass it to every call for that direction; nil means no takeover:

```go
var send, recv deflate.Window // One pair per connection.
compressed, err := compressor.Compress(payload, &send)
message, err := decompressor.Decompress(compressed, 8<<20, &recv)
```

Process compressed messages in order. Uncompressed messages bypass the helpers and don't change the window. A decode error clears the window, since the peers' histories have diverged. Priming an encoder from a window costs about as much as compressing 32 KB, so a compressor that keeps serving the same window continues its stream instead and pays nothing; a compressor shared between connections primes when it switches. Streaming decompression is available through `Begin` and `Read` over a `ChunkSource`.

## Handshake and upgrade

`handshake` implements the opening handshake rules over header values without I/O: accept keys, request and response validation, `permessage-deflate` negotiation, and subprotocol selection. The root package connects it to `net/http`:

```go
conn, res, err := ews.Upgrade(w, r, handshake.Options{Protocols: []string{"chat"}})
if err != nil {
    return // An HTTP error has been written.
}
defer conn.Close()
c, err := ws.NewConn(conn, ws.Config{Role: ws.Server, Compression: res.Compression})
```

The client side is `Dial`, which takes a ws or wss URL and returns the same pair:

```go
conn, res, err := ews.Dial(ctx, "wss://example.com/socket", ews.DialOptions{
    Handshake: handshake.Options{Compression: &handshake.Compress{Level: flate.BestSpeed}},
})
if err != nil {
    return err // A *ews.HandshakeError carries the status and headers of a refusal.
}
defer conn.Close()
c, err := ws.NewConn(conn, ws.Config{Role: ws.Client, Compression: res.Compression})
```

Both return the raw connection; the caller keeps it for deadlines and closing. `ws.Config.ReadBufferSize` defaults to 4 KiB; frames that fit in it are returned without copying, and larger remainders are read straight into a pooled message buffer that the connection holds only until the next read. Deployments with few connections and large messages can raise it so more messages take the zero-copy path. Headers set on the `ResponseWriter` before the call are sent with the 101 response. Origin checks belong to the caller.

## Development

```sh
go test ./...
go vet ./...
go test ./... -run '^$' -bench . -benchmem
```

`examples/echo` serves the endpoint the Autobahn test suite expects. With a container runtime:

```sh
go run ./examples/echo &
docker run --rm --network host -v "$PWD/autobahn:/config" -v "$PWD/autobahn/reports:/reports" \
  crossbario/autobahn-testsuite wstest -m fuzzingclient -s /config/fuzzingclient.json
```

All cases pass. 6.4.x report non-strict, since text is validated per message rather than per chunk. 13.3.x and 13.5.x report unimplemented, since offers asking the server for a window smaller than 32 KB are declined and those connections run uncompressed.

`bench` is a separate module comparing echo servers end to end against other libraries over loopback TCP, driven by the same ews client. Results from a recent run are in `bench/RESULTS.md`:

```sh
cd bench && go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results > RESULTS.md
```

Masking uses 64-bit SWAR by default. On amd64, arm64, and wasm, `GOEXPERIMENT=simd` enables an optional 128-bit path for payloads of at least 512 bytes. SIMD builds require AVX on amd64. This uses Go's experimental `simd/archsimd` API. Text messages are UTF-8 validated with a shift-based DFA after skipping the ASCII prefix in 32-byte words; on amd64 the same flag replaces the DFA with SIMD lookups, using a 256-bit path when AVX2 is available.

```sh
GOEXPERIMENT=simd go test ./...
GOEXPERIMENT=simd go test ./codec -run '^$' -bench BenchmarkMask -benchmem
```
