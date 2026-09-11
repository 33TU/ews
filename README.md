# ews

A lightweight WebSocket frame encoder and decoder for Go, built around plain `[]byte`.

**Work in progress.** The API is still taking shape.

## Design

- Byte-oriented API without requiring `io.Reader` or `io.Writer`.
- Incremental decoding: input can contain partial frames or multiple frames.
- Borrow input when possible; buffer incomplete frames when needed.
- Keep networking and message assembly separate from frame encoding and decoding.

## Decoding

Read each header, then drain its payload before advancing to the next frame.
For example, with a `readChunk` function supplying incoming bytes:

```go
var dec ews.Decoder

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
    for {
        payload, done := dec.Payload()
        if len(payload) != 0 {
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

Payload bytes remain masked when the header's mask bit is set. Unmasking is the caller's responsibility, using `header.MaskKey()` and the running offset within that frame's payload. Headers are returned by value and remain valid across decoder calls.

`NextHeader` rejects invalid payload-length encodings. Other protocol checks, including reserved bits, opcodes, control-frame rules, and connection-specific masking requirements, belong to the caller.

Drain available payload bytes before feeding more input to avoid unnecessary buffering. `Payload` returns `nil, true` before the first header and after payload completion, including empty frames.

Call `Reset` to discard pending input while retaining reusable storage.

## Encoding

Encode a frame, then send its header followed by its payload:

```go
var enc ews.Encoder

if err := enc.Encode(true, ews.Text, []byte("Hello"), nil); err != nil {
    return err
}
header := enc.HeaderBytes()
payload := enc.PayloadBytes()
// Send both slices completely before calling Encode or Reset again.
```

`Encode` borrows unmasked input without copying. Keep that input unchanged until it has been sent. With a mask key, it copies and masks the payload into reusable scratch storage, leaving input unchanged. A nil key means unmasked; any non-nil key must have length four or `Encode` panics. Client frames need a fresh key from `crypto/rand` for each frame; server frames use nil.

Invalid opcodes and malformed control frames return errors without changing output. Message sequencing, UTF-8, and close status codes remain the caller's responsibility.

The output slices are borrowed. Each successful `Encode` replaces the current frame; it does not accumulate frames. Call `Reset` to clear the frame while retaining scratch capacity, or encode the next frame directly after sending the current one.

## Development

```sh
go test ./...
go vet ./...
go test -run '^$' -bench . -benchmem
```
