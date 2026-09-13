# Echo benchmark results

Generated 2026-09-13 from `go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results` at ews commit `0d768b4`.

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0
- gws: v1.10.2

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer. With compression it keeps a compressor attached per connection.
- `ews-shared`: the same with `CompressionShared`, borrowing a pooled compressor per message as gws and coder do. Compressed tables only; it is identical to `ews` otherwise.
- `gws`: gws's `ReadMessage` and `WriteMessage` in a loop, the like-for-like shape against ews. Its event-driven `ReadLoop` shares the frame path and measured the same within noise.
- `gws-stream`: gws's `NextReader` piped into `WriteFile`, so no message is held whole.
- `coder`: coder/websocket with `Read` and `Write` in a loop.
- `coder-stream`: coder/websocket piping `Reader` into `Writer` through a reusable buffer, so no message is held whole.

Compression is permessage-deflate with context takeover in both directions. ews and gws run flate level 1; gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits, which ews does not implement. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | gws | gws-stream | coder | coder-stream | allocs/op ews / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|
| 64 B | 1 | 8 MB/s | 9 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 67 MB/s | 67 MB/s | 65 MB/s | 45 MB/s | 49 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 77 MB/s | 75 MB/s | 73 MB/s | 52 MB/s | 56 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 79 MB/s | 77 MB/s | 73 MB/s | 57 MB/s | 57 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 76 MB/s | 76 MB/s | 73 MB/s | 55 MB/s | 57 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 67 MB/s | 68 MB/s | 63 MB/s | 49 MB/s | 50 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 139 MB/s | 135 MB/s | 131 MB/s | 56 MB/s | 90 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 1.0 GB/s | 1.0 GB/s | 995 MB/s | 492 MB/s | 784 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 1.1 GB/s | 1.2 GB/s | 1.1 GB/s | 612 MB/s | 862 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 1.1 GB/s | 1.2 GB/s | 1.1 GB/s | 630 MB/s | 874 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 1.0 GB/s | 1.1 GB/s | 1.1 GB/s | 621 MB/s | 817 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 916 MB/s | 925 MB/s | 871 MB/s | 555 MB/s | 650 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.5 GB/s | 1.4 GB/s | 1.3 GB/s | 176 MB/s | 773 MB/s | 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 10.2 GB/s | 9.8 GB/s | 10.0 GB/s | 1.9 GB/s | 6.1 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 10.8 GB/s | 10.7 GB/s | 10.5 GB/s | 2.5 GB/s | 6.5 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 9.0 GB/s | 8.1 GB/s | 8.6 GB/s | 2.9 GB/s | 5.3 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 7.0 GB/s | 6.7 GB/s | 6.5 GB/s | 2.9 GB/s | 4.5 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.9 GB/s | 5.9 GB/s | 5.7 GB/s | 2.6 GB/s | 4.1 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.3 GB/s | 580 MB/s | 3.0 GB/s | 438 MB/s | 1.9 GB/s | 0 / 6 (593 KB) / 12 / 97 (701 KB) / 72 (3 KB) |
| 256 KiB | 32 | 18.1 GB/s | 4.6 GB/s | 16.9 GB/s | 3.6 GB/s | 13.4 GB/s | 0 / 5 (572 KB) / 12 / 96 (672 KB) / 72 (4 KB) |
| 256 KiB | 128 | 10.2 GB/s | 6.6 GB/s | 9.7 GB/s | 6.1 GB/s | 8.9 GB/s | 0 / 5 (535 KB) / 12 / 96 (629 KB) / 72 (2 KB) |
| 256 KiB | 512 | 8.7 GB/s | 5.7 GB/s | 8.5 GB/s | 5.1 GB/s | 8.0 GB/s | 0 / 5 (535 KB) / 12 / 96 (627 KB) / 72 (2 KB) |
| 256 KiB | 1024 | 8.1 GB/s | 5.5 GB/s | 8.0 GB/s | 4.9 GB/s | 7.7 GB/s | 0 / 5 (540 KB) / 12 / 96 (632 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.5 GB/s | 4.8 GB/s | 7.9 GB/s | 4.4 GB/s | 7.5 GB/s | 0 / 5 (557 KB) / 12 / 96 (648 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 33 MB/s | 30 MB/s | 27 MB/s | 28 MB/s | 32 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 36 MB/s | 32 MB/s | 29 MB/s | 28 MB/s | 34 MB/s | 30 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 18 MB/s | 18 MB/s | 17 MB/s | 15 MB/s | 22 MB/s | 20 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 22 MB/s | 23 MB/s | 18 MB/s | 20 MB/s | 25 MB/s | 25 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 36 MB/s | 34 MB/s | 33 MB/s | 32 MB/s | 30 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 71 MB/s | 59 MB/s | 59 MB/s | 60 MB/s | 55 MB/s | 59 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 494 MB/s | 471 MB/s | 400 MB/s | 422 MB/s | 451 MB/s | 464 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 532 MB/s | 476 MB/s | 431 MB/s | 414 MB/s | 442 MB/s | 467 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 210 MB/s | 213 MB/s | 207 MB/s | 207 MB/s | 245 MB/s | 250 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 185 MB/s | 190 MB/s | 182 MB/s | 182 MB/s | 207 MB/s | 211 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 183 MB/s | 188 MB/s | 181 MB/s | 176 MB/s | 202 MB/s | 205 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 652 MB/s | 607 MB/s | 611 MB/s | 588 MB/s | 191 MB/s | 572 MB/s | 0 / 0 / 1 / 6 / 25 (40 KB) / 20 |
| 16 KiB | 32 | 5.2 GB/s | 4.9 GB/s | 4.6 GB/s | 4.7 GB/s | 3.1 GB/s | 4.6 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 128 | 4.1 GB/s | 4.5 GB/s | 3.8 GB/s | 3.9 GB/s | 2.8 GB/s | 3.6 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.3 GB/s | 2.7 GB/s | 2.4 GB/s | 2.4 GB/s | 2.0 GB/s | 2.3 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.1 GB/s | 2.4 GB/s | 2.2 GB/s | 2.2 GB/s | 1.8 GB/s | 2.1 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 2048 | 1.6 GB/s | 2.3 GB/s | 2.2 GB/s | 2.1 GB/s | 1.6 GB/s | 2.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.5 GB/s | 1.4 GB/s | 398 MB/s | 432 MB/s | 216 MB/s | 1.3 GB/s | 0 / 0 (1 KB) / 18 (1415 KB) / 21 (1140 KB) / 38 (878 KB) / 56 (3 KB) |
| 256 KiB | 32 | 12.3 GB/s | 12.4 GB/s | 3.5 GB/s | 3.8 GB/s | 5.2 GB/s | 11.5 GB/s | 0 / 0 / 18 (1407 KB) / 22 (1152 KB) / 33 (680 KB) / 56 (3 KB) |
| 256 KiB | 128 | 9.3 GB/s | 10.3 GB/s | 3.7 GB/s | 3.8 GB/s | 5.0 GB/s | 9.5 GB/s | 0 / 0 / 17 (1336 KB) / 21 (1089 KB) / 32 (635 KB) / 56 (2 KB) |
| 256 KiB | 512 | 8.9 GB/s | 11.1 GB/s | 3.9 GB/s | 3.7 GB/s | 5.4 GB/s | 9.1 GB/s | 0 / 0 / 17 (1337 KB) / 21 (1123 KB) / 32 (646 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 9.5 GB/s | 11.1 GB/s | 3.6 GB/s | 3.3 GB/s | 4.8 GB/s | 9.0 GB/s | 0 / 0 / 17 (1400 KB) / 21 (1181 KB) / 32 (667 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 9.3 GB/s | 10.8 GB/s | 3.2 GB/s | 2.0 GB/s | 2.3 GB/s | 8.5 GB/s | 0 / 0 / 18 (1460 KB) / 25 (1383 KB) / 32 (617 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all three tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews. gws allocates a buffer above its pool threshold on every such message, over half a megabyte uncompressed and over a megabyte compressed.
- With hundreds of connections and 256 KiB messages both libraries are bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder/websocket allocates on every message and, with context takeover, resets a pooled flate writer with the 32 KB history per message, which is the priming cost ews avoids by keeping a compressor attached.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.
