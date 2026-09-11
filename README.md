# ews

A lightweight WebSocket frame encoder and decoder for Go, built around plain `[]byte`.

**Work in progress.** The API is still taking shape.

The frame API lives in `github.com/33TU/ews/codec`, message compression in `github.com/33TU/ews/deflate`, and message protocol handling in `github.com/33TU/ews/protocol`. The root is reserved for the planned client/server API.

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
compressed, err := compressor.Compress([]byte("Hello"))
if err != nil {
    return err
}
var enc codec.Encoder
if err := enc.EncodeCompressed(true, codec.Text, compressed, nil); err != nil {
    return err
}
// Send enc.HeaderBytes(), then enc.PayloadBytes().
```

`EncodeCompressed` takes already-compressed bytes. It sets RSV1 on text/binary frames, leaves it clear on continuation frames, and rejects control frames. To fragment a compressed message, split the compressed bytes and encode the pieces with their own masking keys.

On receipt, use `header.RSV1()` on the first data frame to identify a compressed message. Unmask each frame, collect its data fragments through FIN, then decompress the assembled payload. Interleaved control frames are handled separately.

```go
var decompressor deflate.Decompressor // Reuse across messages.
message, err := decompressor.Decompress(compressed, 8<<20) // Maximum 8 MiB output.
if err != nil {
    return err
}
handle(message)
```

Both helpers return borrowed output valid until their next call or `Reset()`. Storage is reused in either mode. The frame decoder continues to expose raw payloads.

To retain history between compressed messages, configure each helper before use to match the negotiated mode for its direction:

```go
compressor.ContextTakeover = true
decompressor := deflate.Decompressor{ContextTakeover: true}
```

With takeover enabled, keep each helper dedicated to one connection direction and process compressed messages in order. Uncompressed messages bypass the helpers and don't change history. Call `Reset()` before reusing a helper for a new connection or changing its mode; this retains storage and configuration. Decode errors clear history, so the existing takeover stream cannot simply continue after an error.

## Protocol sender and receiver

`protocol.Receiver` assembles incoming messages and validates masking, fragmentation, UTF-8, and close codes. `protocol.Sender` appends complete outgoing frames into a caller-owned buffer. Neither owns the transport or an output queue.

```go
import "github.com/33TU/ews/protocol"

receiver, err := protocol.NewReceiver(protocol.ReceiverConfig{
    Role:           protocol.Server,
    MaxMessageSize: 8 << 20,
})
if err != nil {
    return err
}
sender, err := protocol.NewSender(protocol.SenderConfig{Role: protocol.Server})
if err != nil {
    return err
}

var output []byte
output, err = sender.Append(output, codec.Text, []byte("Hello"), false)
if err != nil {
    return err
}
```

`Append` grows the supplied buffer as needed and preserves its existing bytes, so frames can be batched. Its final argument requests compression. Errors return the original destination unchanged. The sender retains neither the destination nor the payload; client frames receive fresh masking keys. Reuse the buffer's capacity after its contents have been written. The caller manages partial writes, queue limits, and backpressure.

Feed received bytes with `receiver.Feed(input)`, then call `receiver.NextEvent()` until it needs more input. Input is borrowed without modification; keep it unchanged until consumed or the next `Feed` returns. Event payloads are borrowed until the next `NextEvent` or `Reset` call.

Messages arrive as complete `codec.Text` or `codec.Binary` events. Ping, pong, and close events are delivered separately, with no automatic output. Respond to a ping by appending a pong with the same payload. On a close event, stop sending data and append a close reply if one has not already been appended:

```go
switch event.Opcode {
case codec.Ping:
    output, err = sender.Append(output, codec.Pong, event.Payload, false)
case codec.Close:
    if !sender.CloseSent() {
        output, err = sender.Append(output, codec.Close, event.Payload, false)
    }
}
```

`sender.AppendClose(output, 1000, "bye")` initiates a close. `CloseSent()` means a close was appended, not yet written to the transport. `receiver.CloseReceived()` records the peer's close. The caller coordinates these states, flushes output, and handles transport shutdown and timeouts. A receiver stops processing input after a close or terminal error. A sender rejects further data after appending a close, but permits pong replies while waiting for the peer.

Enable negotiated receive compression with `ReceiverConfig.Compression` and `ContextTakeover`. Configure the send direction with `SenderConfig.Compression = &protocol.Compression{Level: flate.BestSpeed, ContextTakeover: true}`. Each direction is independent; configure it to match the handshake. Only the default 32 KB window is supported. The receiver's message limit applies to both assembled wire data and expanded payloads; the default is 8 MiB. Outgoing messages use one frame.

A `*protocol.Error` is a terminal receive failure with a suggested close `Code`; the caller can append a close and terminate the connection. The caller also handles EOF and deadlines. Senders and receivers share no mutable state and can run independently, but calls on each individual object must be serialized. Copy borrowed events before handing them to another goroutine. `Reset()` clears protocol and compression history for reuse on a new connection, retaining storage and configuration.

## Development

```sh
go test ./...
go vet ./...
go test ./... -run '^$' -bench . -benchmem
```

Masking uses 64-bit SWAR by default. On amd64, arm64, and wasm, `GOEXPERIMENT=simd` enables an optional 128-bit path for payloads of at least 512 bytes. SIMD builds require AVX on amd64. This uses Go's experimental `simd/archsimd` API.

```sh
GOEXPERIMENT=simd go test ./...
GOEXPERIMENT=simd go test ./codec -run '^$' -bench BenchmarkMask -benchmem
```
