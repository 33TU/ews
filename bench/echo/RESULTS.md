# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 1s | go run ../cmd/results` at ews commit `3a81efb`.

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0
- gws: v1.10.2
- coder/websocket: v1.8.15

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer. With compression it keeps a compressor attached per connection.
- `ews-shared`: the same with `CompressionShared`, borrowing a pooled compressor per message as gws and coder do. Compressed tables only; it is identical to `ews` otherwise.
- `gws`: gws's `ReadMessage` and `WriteMessage` in a loop, the like-for-like shape against ews. Its event-driven `ReadLoop` shares the frame path and measured the same within noise.
- `gws-stream`: gws's `NextReader` piped into `WriteFile`, so no message is held whole.
- `coder`: coder/websocket with `Read` and `Write` in a loop.
- `coder-stream`: coder/websocket piping `Reader` into `Writer` through a reusable buffer, so no message is held whole.

Compression is permessage-deflate with context takeover in both directions. ews and gws run flate level 1; gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | gws | gws-stream | coder | coder-stream | allocs/op ews / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|
| 64 B | 1 | 8 MB/s | 8 MB/s | 7 MB/s | 5 MB/s | 6 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 52 MB/s | 54 MB/s | 46 MB/s | 39 MB/s | 41 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 56 MB/s | 56 MB/s | 55 MB/s | 43 MB/s | 43 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 54 MB/s | 54 MB/s | 51 MB/s | 43 MB/s | 43 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 54 MB/s | 53 MB/s | 53 MB/s | 42 MB/s | 43 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 50 MB/s | 49 MB/s | 46 MB/s | 38 MB/s | 39 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 127 MB/s | 128 MB/s | 120 MB/s | 53 MB/s | 87 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 805 MB/s | 776 MB/s | 758 MB/s | 500 MB/s | 483 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 698 MB/s | 668 MB/s | 606 MB/s | 498 MB/s | 489 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 662 MB/s | 741 MB/s | 736 MB/s | 78 MB/s | 144 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 179 MB/s | 189 MB/s | 192 MB/s | 129 MB/s | 159 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 188 MB/s | 215 MB/s | 703 MB/s | 508 MB/s | 584 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.4 GB/s | 1.3 GB/s | 1.3 GB/s | 178 MB/s | 681 MB/s | 0 / 1 / 6 / 61 (40 KB) / 16 |
| 16 KiB | 32 | 9.5 GB/s | 8.9 GB/s | 9.0 GB/s | 1.9 GB/s | 4.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 9.5 GB/s | 9.2 GB/s | 7.7 GB/s | 2.2 GB/s | 4.0 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 6.9 GB/s | 7.1 GB/s | 6.8 GB/s | 2.5 GB/s | 3.8 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.3 GB/s | 6.2 GB/s | 6.0 GB/s | 2.8 GB/s | 3.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.3 GB/s | 5.4 GB/s | 5.1 GB/s | 2.7 GB/s | 3.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.2 GB/s | 642 MB/s | 2.9 GB/s | 448 MB/s | 1.3 GB/s | 0 / 6 (598 KB) / 12 / 97 (705 KB) / 72 (3 KB) |
| 256 KiB | 32 | 2.0 GB/s | 1.2 GB/s | 2.4 GB/s | 1.1 GB/s | 2.1 GB/s | 0 / 5 (564 KB) / 12 (1 KB) / 96 (662 KB) / 72 (4 KB) |
| 256 KiB | 128 | 9.0 GB/s | 6.3 GB/s | 9.3 GB/s | 5.7 GB/s | 7.6 GB/s | 0 / 5 (535 KB) / 12 / 96 (629 KB) / 72 (3 KB) |
| 256 KiB | 512 | 8.0 GB/s | 5.5 GB/s | 7.5 GB/s | 4.9 GB/s | 6.7 GB/s | 0 / 5 (535 KB) / 12 / 96 (627 KB) / 72 (2 KB) |
| 256 KiB | 1024 | 7.2 GB/s | 5.2 GB/s | 7.1 GB/s | 4.7 GB/s | 6.7 GB/s | 0 / 5 (540 KB) / 12 / 96 (631 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.3 GB/s | 4.8 GB/s | 7.3 GB/s | 4.3 GB/s | 6.8 GB/s | 0 / 5 (555 KB) / 12 / 96 (650 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 39 MB/s | 32 MB/s | 26 MB/s | 27 MB/s | 30 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 36 MB/s | 31 MB/s | 28 MB/s | 28 MB/s | 30 MB/s | 26 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 30 MB/s | 24 MB/s | 20 MB/s | 21 MB/s | 25 MB/s | 24 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 30 MB/s | 8 MB/s | 23 MB/s | 22 MB/s | 25 MB/s | 23 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 32 MB/s | 29 MB/s | 29 MB/s | 29 MB/s | 27 MB/s | 24 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 72 MB/s | 59 MB/s | 49 MB/s | 53 MB/s | 42 MB/s | 56 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 386 MB/s | 461 MB/s | 384 MB/s | 404 MB/s | 408 MB/s | 421 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 521 MB/s | 421 MB/s | 369 MB/s | 373 MB/s | 409 MB/s | 416 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 385 MB/s | 339 MB/s | 258 MB/s | 257 MB/s | 305 MB/s | 312 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 331 MB/s | 292 MB/s | 224 MB/s | 214 MB/s | 250 MB/s | 257 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 281 MB/s | 264 MB/s | 220 MB/s | 223 MB/s | 247 MB/s | 243 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 660 MB/s | 573 MB/s | 577 MB/s | 573 MB/s | 193 MB/s | 561 MB/s | 0 / 0 / 1 / 6 / 25 (41 KB) / 20 |
| 16 KiB | 32 | 4.5 GB/s | 4.3 GB/s | 4.0 GB/s | 3.9 GB/s | 2.6 GB/s | 4.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 3.2 GB/s | 3.4 GB/s | 3.2 GB/s | 3.3 GB/s | 2.4 GB/s | 3.0 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.4 GB/s | 2.5 GB/s | 2.3 GB/s | 2.4 GB/s | 2.0 GB/s | 2.2 GB/s | 0 / 0 / 1 / 6 (1 KB) / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.3 GB/s | 2.3 GB/s | 2.2 GB/s | 2.1 GB/s | 1.8 GB/s | 2.2 GB/s | 0 / 0 (4 KB) / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 2048 | 2.2 GB/s | 2.4 GB/s | 2.1 GB/s | 2.0 GB/s | 1.6 GB/s | 2.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 351 MB/s | 411 MB/s | 275 MB/s | 1.3 GB/s | 0 (1 KB) / 0 (2 KB) / 18 (1432 KB) / 21 (1150 KB) / 37 (841 KB) / 56 (3 KB) |
| 256 KiB | 32 | 11.6 GB/s | 11.7 GB/s | 3.3 GB/s | 3.6 GB/s | 4.9 GB/s | 9.6 GB/s | 0 / 0 / 18 (1399 KB) / 21 (1109 KB) / 33 (669 KB) / 56 (2 KB) |
| 256 KiB | 128 | 8.4 GB/s | 10.3 GB/s | 3.5 GB/s | 3.6 GB/s | 5.1 GB/s | 8.9 GB/s | 0 / 0 / 17 (1339 KB) / 20 (1089 KB) / 32 (636 KB) / 56 (2 KB) |
| 256 KiB | 512 | 8.7 GB/s | 9.8 GB/s | 3.6 GB/s | 3.6 GB/s | 5.1 GB/s | 7.9 GB/s | 0 / 0 / 17 (1334 KB) / 21 (1128 KB) / 32 (644 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.8 GB/s | 9.8 GB/s | 3.8 GB/s | 3.4 GB/s | 4.8 GB/s | 8.7 GB/s | 0 / 0 / 17 (1373 KB) / 21 (1170 KB) / 32 (674 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 9.0 GB/s | 9.9 GB/s | 3.2 GB/s | 2.6 GB/s | 2.5 GB/s | 8.5 GB/s | 0 / 0 / 18 (1454 KB) / 24 (1303 KB) / 32 (615 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

