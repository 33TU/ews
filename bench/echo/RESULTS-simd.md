# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `e220206`.

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
- `gws`: gws's `ReadMessage` and `WriteMessage` in a loop, the like-for-like shape against ews. Its event-driven `ReadLoop` shares the frame path and measured the same within noise.
- `gws-stream`: gws's `NextReader` piped into `WriteFile`, so no message is held whole.
- `coder`: coder/websocket with `Read` and `Write` in a loop.
- `coder-stream`: coder/websocket piping `Reader` into `Writer` through a reusable buffer, so no message is held whole.

Compression is permessage-deflate with context takeover in both directions. ews and gws run flate level 1; gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | gws | gws-stream | coder | coder-stream | allocs/op ews / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|
| 64 B | 1 | 8 MB/s | 8 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 51 MB/s | 51 MB/s | 50 MB/s | 37 MB/s | 38 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 53 MB/s | 54 MB/s | 52 MB/s | 42 MB/s | 43 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 52 MB/s | 55 MB/s | 52 MB/s | 43 MB/s | 45 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 54 MB/s | 52 MB/s | 52 MB/s | 43 MB/s | 42 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 48 MB/s | 48 MB/s | 45 MB/s | 38 MB/s | 38 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 121 MB/s | 123 MB/s | 117 MB/s | 49 MB/s | 86 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 797 MB/s | 808 MB/s | 770 MB/s | 517 MB/s | 615 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 813 MB/s | 823 MB/s | 779 MB/s | 520 MB/s | 636 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 776 MB/s | 764 MB/s | 753 MB/s | 554 MB/s | 639 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 787 MB/s | 679 MB/s | 760 MB/s | 532 MB/s | 596 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 664 MB/s | 707 MB/s | 664 MB/s | 483 MB/s | 563 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.3 GB/s | 1.3 GB/s | 1.2 GB/s | 150 MB/s | 570 MB/s | 0 / 1 / 6 / 61 (40 KB) / 16 |
| 16 KiB | 32 | 8.4 GB/s | 7.5 GB/s | 8.4 GB/s | 2.0 GB/s | 4.1 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 8.6 GB/s | 8.4 GB/s | 8.6 GB/s | 2.2 GB/s | 4.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 7.5 GB/s | 7.5 GB/s | 6.9 GB/s | 2.7 GB/s | 4.0 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.0 GB/s | 5.9 GB/s | 5.8 GB/s | 2.8 GB/s | 3.7 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.3 GB/s | 5.2 GB/s | 5.0 GB/s | 2.7 GB/s | 3.4 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 2.9 GB/s | 651 MB/s | 2.7 GB/s | 565 MB/s | 1.6 GB/s | 0 / 6 (600 KB) / 12 / 97 (703 KB) / 72 (4 KB) |
| 256 KiB | 32 | 15.8 GB/s | 4.0 GB/s | 14.5 GB/s | 3.6 GB/s | 10.9 GB/s | 0 / 5 (572 KB) / 12 / 96 (669 KB) / 72 (3 KB) |
| 256 KiB | 128 | 9.9 GB/s | 6.2 GB/s | 9.0 GB/s | 5.7 GB/s | 8.1 GB/s | 0 / 5 (536 KB) / 12 / 96 (630 KB) / 72 (3 KB) |
| 256 KiB | 512 | 7.5 GB/s | 5.6 GB/s | 7.7 GB/s | 4.9 GB/s | 7.3 GB/s | 0 / 5 (541 KB) / 12 / 96 (633 KB) / 72 (3 KB) |
| 256 KiB | 1024 | 7.5 GB/s | 5.6 GB/s | 6.9 GB/s | 5.1 GB/s | 7.1 GB/s | 0 / 5 (550 KB) / 12 / 96 (643 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.1 GB/s | 5.0 GB/s | 7.4 GB/s | 4.8 GB/s | 6.8 GB/s | 0 / 5 (580 KB) / 12 / 96 (671 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 36 MB/s | 31 MB/s | 26 MB/s | 27 MB/s | 30 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 37 MB/s | 33 MB/s | 29 MB/s | 29 MB/s | 30 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 34 MB/s | 29 MB/s | 27 MB/s | 25 MB/s | 28 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 31 MB/s | 32 MB/s | 29 MB/s | 30 MB/s | 27 MB/s | 25 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 32 MB/s | 31 MB/s | 31 MB/s | 31 MB/s | 27 MB/s | 25 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 76 MB/s | 62 MB/s | 59 MB/s | 58 MB/s | 48 MB/s | 59 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 |
| 1 KiB | 32 | 526 MB/s | 475 MB/s | 386 MB/s | 393 MB/s | 404 MB/s | 436 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 512 MB/s | 433 MB/s | 368 MB/s | 381 MB/s | 416 MB/s | 436 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 381 MB/s | 320 MB/s | 256 MB/s | 265 MB/s | 298 MB/s | 308 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 333 MB/s | 291 MB/s | 234 MB/s | 230 MB/s | 267 MB/s | 271 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 317 MB/s | 288 MB/s | 224 MB/s | 226 MB/s | 246 MB/s | 259 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 648 MB/s | 595 MB/s | 579 MB/s | 573 MB/s | 166 MB/s | 520 MB/s | 0 / 0 / 1 / 6 / 25 (42 KB) / 20 |
| 16 KiB | 32 | 4.7 GB/s | 4.3 GB/s | 4.1 GB/s | 4.2 GB/s | 2.7 GB/s | 4.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 3.1 GB/s | 3.2 GB/s | 3.0 GB/s | 3.2 GB/s | 2.5 GB/s | 2.9 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.4 GB/s | 2.6 GB/s | 2.4 GB/s | 2.3 GB/s | 2.0 GB/s | 2.3 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.2 GB/s | 2.4 GB/s | 2.2 GB/s | 2.2 GB/s | 1.8 GB/s | 2.1 GB/s | 0 / 0 (3 KB) / 1 / 6 (1 KB) / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.1 GB/s | 2.4 GB/s | 2.1 GB/s | 2.1 GB/s | 1.6 GB/s | 1.9 GB/s | 0 / 0 / 1 / 6 (1 KB) / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.5 GB/s | 354 MB/s | 361 MB/s | 254 MB/s | 1.3 GB/s | 0 (1 KB) / 0 / 19 (1462 KB) / 22 (1158 KB) / 39 (942 KB) / 56 (2 KB) |
| 256 KiB | 32 | 11.5 GB/s | 12.0 GB/s | 3.3 GB/s | 3.5 GB/s | 4.8 GB/s | 9.7 GB/s | 0 / 0 (1 KB) / 17 (1351 KB) / 21 (1112 KB) / 33 (654 KB) / 56 (3 KB) |
| 256 KiB | 128 | 9.7 GB/s | 10.9 GB/s | 3.7 GB/s | 3.8 GB/s | 5.2 GB/s | 9.2 GB/s | 0 / 0 (1 KB) / 17 (1343 KB) / 20 (1085 KB) / 32 (645 KB) / 56 (2 KB) |
| 256 KiB | 512 | 9.3 GB/s | 10.6 GB/s | 3.8 GB/s | 3.6 GB/s | 5.1 GB/s | 8.8 GB/s | 0 / 0 (1 KB) / 17 (1360 KB) / 21 (1163 KB) / 33 (669 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.9 GB/s | 10.2 GB/s | 3.8 GB/s | 3.3 GB/s | 5.0 GB/s | 8.7 GB/s | 0 / 0 / 18 (1436 KB) / 22 (1225 KB) / 33 (702 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.9 GB/s | 9.2 GB/s | 3.6 GB/s | 2.6 GB/s | 3.6 GB/s | 8.5 GB/s | 0 / 0 / 20 (1534 KB) / 25 (1432 KB) / 33 (623 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

