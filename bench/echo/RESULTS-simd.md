# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `de99c22`.

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
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
| 64 B | 1 | 8 MB/s | 8 MB/s | 7 MB/s | 6 MB/s | 6 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 54 MB/s | 53 MB/s | 53 MB/s | 39 MB/s | 42 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 57 MB/s | 57 MB/s | 55 MB/s | 44 MB/s | 45 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 58 MB/s | 57 MB/s | 56 MB/s | 45 MB/s | 45 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 55 MB/s | 57 MB/s | 54 MB/s | 46 MB/s | 45 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 51 MB/s | 48 MB/s | 50 MB/s | 42 MB/s | 41 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 123 MB/s | 126 MB/s | 117 MB/s | 58 MB/s | 87 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 820 MB/s | 776 MB/s | 796 MB/s | 493 MB/s | 626 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 797 MB/s | 839 MB/s | 784 MB/s | 550 MB/s | 652 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 799 MB/s | 811 MB/s | 759 MB/s | 518 MB/s | 618 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 756 MB/s | 747 MB/s | 801 MB/s | 468 MB/s | 647 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 744 MB/s | 738 MB/s | 633 MB/s | 466 MB/s | 515 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 1.3 GB/s | 170 MB/s | 734 MB/s | 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 8.7 GB/s | 8.4 GB/s | 8.1 GB/s | 1.9 GB/s | 4.2 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 8.9 GB/s | 8.5 GB/s | 8.8 GB/s | 2.3 GB/s | 4.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 7.8 GB/s | 7.7 GB/s | 6.7 GB/s | 2.8 GB/s | 4.1 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.2 GB/s | 6.3 GB/s | 6.2 GB/s | 2.9 GB/s | 3.9 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.4 GB/s | 5.4 GB/s | 5.3 GB/s | 2.7 GB/s | 3.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.1 GB/s | 585 MB/s | 2.9 GB/s | 504 MB/s | 1.7 GB/s | 0 / 6 (596 KB) / 12 / 97 (704 KB) / 72 (5 KB) |
| 256 KiB | 32 | 16.3 GB/s | 4.5 GB/s | 14.9 GB/s | 4.0 GB/s | 10.6 GB/s | 0 / 5 (572 KB) / 12 / 96 (667 KB) / 72 (4 KB) |
| 256 KiB | 128 | 10.1 GB/s | 6.1 GB/s | 9.5 GB/s | 5.6 GB/s | 8.5 GB/s | 0 / 5 (536 KB) / 12 / 96 (629 KB) / 72 (2 KB) |
| 256 KiB | 512 | 8.3 GB/s | 5.8 GB/s | 2.8 GB/s | 5.1 GB/s | 7.5 GB/s | 0 / 5 (540 KB) / 12 / 96 (636 KB) / 72 (2 KB) |
| 256 KiB | 1024 | 7.9 GB/s | 5.8 GB/s | 3.7 GB/s | 5.0 GB/s | 7.0 GB/s | 0 / 5 (551 KB) / 12 / 96 (645 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.5 GB/s | 5.1 GB/s | 7.5 GB/s | 5.1 GB/s | 6.9 GB/s | 0 / 5 (582 KB) / 12 / 96 (670 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 38 MB/s | 32 MB/s | 27 MB/s | 28 MB/s | 31 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 38 MB/s | 33 MB/s | 29 MB/s | 29 MB/s | 32 MB/s | 29 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 35 MB/s | 29 MB/s | 26 MB/s | 21 MB/s | 30 MB/s | 24 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 35 MB/s | 32 MB/s | 32 MB/s | 32 MB/s | 30 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 34 MB/s | 33 MB/s | 32 MB/s | 33 MB/s | 29 MB/s | 26 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 77 MB/s | 65 MB/s | 62 MB/s | 61 MB/s | 48 MB/s | 59 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 558 MB/s | 491 MB/s | 406 MB/s | 426 MB/s | 447 MB/s | 464 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 567 MB/s | 498 MB/s | 423 MB/s | 439 MB/s | 443 MB/s | 450 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 405 MB/s | 351 MB/s | 270 MB/s | 274 MB/s | 308 MB/s | 324 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 341 MB/s | 306 MB/s | 237 MB/s | 239 MB/s | 272 MB/s | 279 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 326 MB/s | 293 MB/s | 233 MB/s | 242 MB/s | 262 MB/s | 268 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 655 MB/s | 616 MB/s | 612 MB/s | 582 MB/s | 193 MB/s | 560 MB/s | 0 / 0 / 1 / 6 / 25 (41 KB) / 20 |
| 16 KiB | 32 | 5.0 GB/s | 4.5 GB/s | 4.3 GB/s | 4.5 GB/s | 3.0 GB/s | 4.4 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 128 | 3.1 GB/s | 3.6 GB/s | 3.5 GB/s | 3.5 GB/s | 2.6 GB/s | 3.3 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.4 GB/s | 2.6 GB/s | 2.4 GB/s | 2.4 GB/s | 2.0 GB/s | 2.3 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.3 GB/s | 2.5 GB/s | 2.2 GB/s | 2.3 GB/s | 1.8 GB/s | 2.2 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.2 GB/s | 2.4 GB/s | 2.2 GB/s | 2.2 GB/s | 1.6 GB/s | 2.1 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 256 KiB | 1 | 1.5 GB/s | 1.4 GB/s | 384 MB/s | 503 MB/s | 289 MB/s | 1.3 GB/s | 0 / 0 (2 KB) / 19 (1449 KB) / 21 (1144 KB) / 38 (893 KB) / 56 (3 KB) |
| 256 KiB | 32 | 12.2 GB/s | 12.5 GB/s | 3.2 GB/s | 3.5 GB/s | 5.0 GB/s | 11.4 GB/s | 0 / 0 (1 KB) / 18 (1419 KB) / 21 (1133 KB) / 34 (705 KB) / 56 (3 KB) |
| 256 KiB | 128 | 9.9 GB/s | 11.3 GB/s | 3.7 GB/s | 3.7 GB/s | 5.3 GB/s | 9.4 GB/s | 0 / 0 (1 KB) / 17 (1352 KB) / 21 (1101 KB) / 32 (641 KB) / 56 (2 KB) |
| 256 KiB | 512 | 9.3 GB/s | 10.9 GB/s | 3.7 GB/s | 3.7 GB/s | 5.2 GB/s | 8.9 GB/s | 0 / 0 (1 KB) / 17 (1373 KB) / 21 (1153 KB) / 33 (668 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 9.1 GB/s | 10.6 GB/s | 3.7 GB/s | 3.2 GB/s | 4.9 GB/s | 8.6 GB/s | 0 / 0 / 18 (1424 KB) / 23 (1289 KB) / 33 (715 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 9.0 GB/s | 10.5 GB/s | 3.5 GB/s | 2.4 GB/s | 4.0 GB/s | 8.7 GB/s | 0 / 0 / 20 (1532 KB) / 25 (1415 KB) / 32 (618 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

