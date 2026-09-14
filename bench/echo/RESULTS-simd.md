# Echo benchmark results

Generated 2026-09-15 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `6cda9bb`.

![echo-plain-simd](echo-plain-simd.svg)

![echo-compressed-simd](echo-compressed-simd.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
- gws: v1.10.2
- coder/websocket: v1.8.15

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer. With compression it keeps a compressor attached per connection.
- `ews-shared`: the same with `CompressionShared`, borrowing a pooled compressor per message as gws and coder do. Compressed tables only; it is identical to `ews` otherwise.
- `ews-stream`: `NextMessage` then `WriteFrom` reading the connection itself, so no message is held whole; ews's streaming shape, against `gws-stream` and `coder-stream`.
- `gws`: gws's `ReadMessage` and `WriteMessage` in a loop, the like-for-like shape against ews. Its event-driven `ReadLoop` shares the frame path and measured the same within noise.
- `gws-stream`: gws's `NextReader` piped into `WriteFile`, so no message is held whole.
- `coder`: coder/websocket with `Read` and `Write` in a loop.
- `coder-stream`: coder/websocket piping `Reader` into `Writer` through a reusable buffer, so no message is held whole.

Compression is permessage-deflate with context takeover in both directions. ews and gws run flate level 1; gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | ews-stream | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-stream / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 9 MB/s | 9 MB/s | 9 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 65 MB/s | 66 MB/s | 65 MB/s | 63 MB/s | 45 MB/s | 47 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 75 MB/s | 70 MB/s | 63 MB/s | 63 MB/s | 47 MB/s | 53 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 75 MB/s | 75 MB/s | 76 MB/s | 70 MB/s | 55 MB/s | 57 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 74 MB/s | 67 MB/s | 69 MB/s | 69 MB/s | 53 MB/s | 54 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 60 MB/s | 59 MB/s | 55 MB/s | 57 MB/s | 48 MB/s | 48 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 125 MB/s | 125 MB/s | 133 MB/s | 120 MB/s | 66 MB/s | 92 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 1.0 GB/s | 918 MB/s | 955 MB/s | 960 MB/s | 559 MB/s | 729 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 1.1 GB/s | 1.0 GB/s | 1.1 GB/s | 1.0 GB/s | 574 MB/s | 715 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 977 MB/s | 660 MB/s | 821 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 816 MB/s | 892 MB/s | 1.0 GB/s | 962 MB/s | 574 MB/s | 710 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 813 MB/s | 838 MB/s | 882 MB/s | 754 MB/s | 556 MB/s | 575 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.4 GB/s | 1.3 GB/s | 1.4 GB/s | 1.3 GB/s | 169 MB/s | 692 MB/s | 0 / 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 8.8 GB/s | 9.0 GB/s | 9.1 GB/s | 9.2 GB/s | 2.0 GB/s | 4.9 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 10.2 GB/s | 10.3 GB/s | 9.9 GB/s | 10.0 GB/s | 2.3 GB/s | 6.1 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 7.9 GB/s | 8.0 GB/s | 8.2 GB/s | 7.7 GB/s | 2.9 GB/s | 4.9 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.2 GB/s | 5.5 GB/s | 5.6 GB/s | 5.9 GB/s | 2.8 GB/s | 4.2 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.3 GB/s | 5.0 GB/s | 4.4 GB/s | 5.3 GB/s | 2.2 GB/s | 3.7 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.2 GB/s | 2.8 GB/s | 580 MB/s | 2.9 GB/s | 423 MB/s | 1.7 GB/s | 0 / 0 / 6 (596 KB) / 12 / 97 (701 KB) / 72 (4 KB) |
| 256 KiB | 32 | 16.8 GB/s | 16.0 GB/s | 3.6 GB/s | 15.9 GB/s | 3.8 GB/s | 11.7 GB/s | 0 / 0 / 5 (572 KB) / 12 / 96 (665 KB) / 72 (3 KB) |
| 256 KiB | 128 | 8.9 GB/s | 8.4 GB/s | 6.1 GB/s | 9.2 GB/s | 5.2 GB/s | 8.5 GB/s | 0 / 0 / 5 (537 KB) / 12 / 96 (631 KB) / 72 (3 KB) |
| 256 KiB | 512 | 7.8 GB/s | 8.0 GB/s | 5.3 GB/s | 7.6 GB/s | 4.7 GB/s | 7.1 GB/s | 0 / 0 / 5 (541 KB) / 12 / 96 (634 KB) / 72 (2 KB) |
| 256 KiB | 1024 | 7.6 GB/s | 7.1 GB/s | 5.7 GB/s | 7.1 GB/s | 4.9 GB/s | 6.8 GB/s | 0 / 0 / 5 (551 KB) / 12 / 96 (640 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.3 GB/s | 7.4 GB/s | 4.9 GB/s | 7.1 GB/s | 4.8 GB/s | 6.5 GB/s | 0 / 0 / 5 (580 KB) / 12 / 96 (671 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | ews-stream | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 41 MB/s | 35 MB/s | 42 MB/s | 28 MB/s | 29 MB/s | 33 MB/s | 29 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 41 MB/s | 35 MB/s | 43 MB/s | 31 MB/s | 32 MB/s | 34 MB/s | 29 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 36 MB/s | 29 MB/s | 36 MB/s | 24 MB/s | 27 MB/s | 29 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 36 MB/s | 32 MB/s | 36 MB/s | 31 MB/s | 32 MB/s | 30 MB/s | 28 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 35 MB/s | 33 MB/s | 34 MB/s | 34 MB/s | 33 MB/s | 29 MB/s | 24 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 72 MB/s | 60 MB/s | 76 MB/s | 57 MB/s | 55 MB/s | 49 MB/s | 61 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 603 MB/s | 519 MB/s | 597 MB/s | 431 MB/s | 436 MB/s | 452 MB/s | 465 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 603 MB/s | 478 MB/s | 549 MB/s | 418 MB/s | 434 MB/s | 446 MB/s | 468 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 393 MB/s | 333 MB/s | 350 MB/s | 232 MB/s | 252 MB/s | 318 MB/s | 329 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 349 MB/s | 301 MB/s | 332 MB/s | 234 MB/s | 224 MB/s | 276 MB/s | 278 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 305 MB/s | 281 MB/s | 322 MB/s | 222 MB/s | 246 MB/s | 242 MB/s | 244 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 617 MB/s | 598 MB/s | 661 MB/s | 577 MB/s | 582 MB/s | 154 MB/s | 570 MB/s | 0 / 0 / 0 / 1 / 6 / 25 (44 KB) / 20 |
| 16 KiB | 32 | 4.7 GB/s | 4.9 GB/s | 4.5 GB/s | 4.3 GB/s | 4.5 GB/s | 2.8 GB/s | 4.5 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 3.2 GB/s | 3.0 GB/s | 3.0 GB/s | 3.4 GB/s | 3.3 GB/s | 2.5 GB/s | 3.3 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.2 GB/s | 2.5 GB/s | 2.2 GB/s | 2.3 GB/s | 2.3 GB/s | 1.9 GB/s | 2.2 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.1 GB/s | 2.2 GB/s | 2.2 GB/s | 2.2 GB/s | 2.0 GB/s | 1.7 GB/s | 2.0 GB/s | 0 / 0 / 0 / 1 / 6 (1 KB) / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.0 GB/s | 2.3 GB/s | 2.2 GB/s | 2.1 GB/s | 2.1 GB/s | 1.4 GB/s | 2.0 GB/s | 0 / 0 / 0 / 1 / 6 (1 KB) / 25 (38 KB) / 20 |
| 256 KiB | 1 | 1.5 GB/s | 1.4 GB/s | 1.2 GB/s | 344 MB/s | 455 MB/s | 217 MB/s | 1.3 GB/s | 0 / 0 (1 KB) / 0 (1 KB) / 18 (1393 KB) / 22 (1181 KB) / 42 (1090 KB) / 56 (2 KB) |
| 256 KiB | 32 | 11.5 GB/s | 11.3 GB/s | 10.5 GB/s | 3.1 GB/s | 3.6 GB/s | 4.7 GB/s | 10.9 GB/s | 0 / 0 (1 KB) / 0 / 17 (1381 KB) / 21 (1101 KB) / 33 (669 KB) / 56 (2 KB) |
| 256 KiB | 128 | 9.5 GB/s | 10.7 GB/s | 9.0 GB/s | 3.5 GB/s | 3.6 GB/s | 5.0 GB/s | 8.6 GB/s | 0 / 0 (3 KB) / 0 / 17 (1346 KB) / 21 (1101 KB) / 32 (642 KB) / 56 (3 KB) |
| 256 KiB | 512 | 9.0 GB/s | 10.5 GB/s | 8.7 GB/s | 3.6 GB/s | 3.5 GB/s | 4.9 GB/s | 7.6 GB/s | 0 / 0 / 0 / 17 (1377 KB) / 21 (1169 KB) / 32 (663 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.4 GB/s | 10.0 GB/s | 8.9 GB/s | 3.4 GB/s | 3.0 GB/s | 4.4 GB/s | 8.5 GB/s | 0 / 0 (1 KB) / 0 / 18 (1441 KB) / 23 (1253 KB) / 33 (715 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.4 GB/s | 9.2 GB/s | 8.7 GB/s | 3.4 GB/s | 2.6 GB/s | 2.6 GB/s | 8.3 GB/s | 0 / 0 / 0 / 19 (1505 KB) / 26 (1449 KB) / 32 (616 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

