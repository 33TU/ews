# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `ad8ef9d`.

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
| 64 B | 1 | 8 MB/s | 8 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 54 MB/s | 54 MB/s | 52 MB/s | 39 MB/s | 42 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 56 MB/s | 57 MB/s | 55 MB/s | 44 MB/s | 44 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 56 MB/s | 58 MB/s | 54 MB/s | 46 MB/s | 47 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 57 MB/s | 57 MB/s | 54 MB/s | 45 MB/s | 46 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 52 MB/s | 52 MB/s | 48 MB/s | 41 MB/s | 41 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 128 MB/s | 131 MB/s | 124 MB/s | 62 MB/s | 84 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 832 MB/s | 826 MB/s | 803 MB/s | 542 MB/s | 642 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 861 MB/s | 818 MB/s | 808 MB/s | 538 MB/s | 671 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 864 MB/s | 871 MB/s | 850 MB/s | 567 MB/s | 693 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 833 MB/s | 830 MB/s | 808 MB/s | 554 MB/s | 647 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 746 MB/s | 757 MB/s | 733 MB/s | 516 MB/s | 588 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 1.3 GB/s | 190 MB/s | 736 MB/s | 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 9.0 GB/s | 8.6 GB/s | 8.4 GB/s | 1.8 GB/s | 4.5 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 9.3 GB/s | 8.8 GB/s | 8.8 GB/s | 2.5 GB/s | 4.8 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 8.1 GB/s | 7.9 GB/s | 7.7 GB/s | 2.8 GB/s | 2.4 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.4 GB/s | 6.3 GB/s | 6.2 GB/s | 3.0 GB/s | 3.9 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.5 GB/s | 5.4 GB/s | 5.4 GB/s | 2.8 GB/s | 3.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.1 GB/s | 605 MB/s | 2.9 GB/s | 469 MB/s | 1.8 GB/s | 0 / 6 (598 KB) / 12 / 97 (709 KB) / 72 (5 KB) |
| 256 KiB | 32 | 16.0 GB/s | 4.4 GB/s | 15.2 GB/s | 4.3 GB/s | 11.0 GB/s | 0 / 5 (569 KB) / 12 / 96 (665 KB) / 72 (3 KB) |
| 256 KiB | 128 | 10.1 GB/s | 6.2 GB/s | 9.4 GB/s | 5.5 GB/s | 8.4 GB/s | 0 / 5 (535 KB) / 12 / 96 (630 KB) / 72 (3 KB) |
| 256 KiB | 512 | 8.3 GB/s | 3.7 GB/s | 7.9 GB/s | 2.5 GB/s | 4.6 GB/s | 0 / 5 (544 KB) / 12 / 96 (635 KB) / 72 (3 KB) |
| 256 KiB | 1024 | 7.8 GB/s | 5.9 GB/s | 7.6 GB/s | 5.0 GB/s | 7.1 GB/s | 0 / 5 (551 KB) / 12 / 96 (641 KB) / 72 (3 KB) |
| 256 KiB | 2048 | 7.6 GB/s | 5.0 GB/s | 7.4 GB/s | 5.0 GB/s | 6.9 GB/s | 0 / 5 (581 KB) / 12 / 96 (670 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 38 MB/s | 33 MB/s | 26 MB/s | 28 MB/s | 30 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 39 MB/s | 34 MB/s | 28 MB/s | 30 MB/s | 32 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 35 MB/s | 29 MB/s | 27 MB/s | 26 MB/s | 29 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 35 MB/s | 32 MB/s | 32 MB/s | 31 MB/s | 30 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 34 MB/s | 33 MB/s | 33 MB/s | 31 MB/s | 28 MB/s | 26 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 74 MB/s | 61 MB/s | 57 MB/s | 57 MB/s | 45 MB/s | 57 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 553 MB/s | 483 MB/s | 398 MB/s | 414 MB/s | 443 MB/s | 459 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 559 MB/s | 483 MB/s | 400 MB/s | 432 MB/s | 437 MB/s | 446 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 403 MB/s | 348 MB/s | 271 MB/s | 273 MB/s | 316 MB/s | 323 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 347 MB/s | 309 MB/s | 237 MB/s | 239 MB/s | 273 MB/s | 280 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 322 MB/s | 299 MB/s | 232 MB/s | 243 MB/s | 272 MB/s | 269 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 655 MB/s | 589 MB/s | 585 MB/s | 581 MB/s | 169 MB/s | 535 MB/s | 0 / 0 / 1 / 6 / 25 (42 KB) / 20 |
| 16 KiB | 32 | 4.8 GB/s | 4.5 GB/s | 4.4 GB/s | 4.5 GB/s | 2.8 GB/s | 4.2 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 3.5 GB/s | 3.6 GB/s | 3.5 GB/s | 3.5 GB/s | 2.7 GB/s | 3.2 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.6 GB/s | 2.7 GB/s | 2.5 GB/s | 2.5 GB/s | 2.1 GB/s | 2.4 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.4 GB/s | 2.6 GB/s | 2.3 GB/s | 2.4 GB/s | 1.9 GB/s | 2.3 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.3 GB/s | 2.5 GB/s | 2.3 GB/s | 2.3 GB/s | 1.6 GB/s | 2.2 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 369 MB/s | 442 MB/s | 292 MB/s | 1.3 GB/s | 0 (1 KB) / 0 (2 KB) / 18 (1428 KB) / 22 (1153 KB) / 38 (867 KB) / 56 (2 KB) |
| 256 KiB | 32 | 12.3 GB/s | 12.4 GB/s | 3.3 GB/s | 3.7 GB/s | 5.2 GB/s | 11.1 GB/s | 0 / 0 (1 KB) / 18 (1406 KB) / 22 (1145 KB) / 33 (684 KB) / 56 (3 KB) |
| 256 KiB | 128 | 10.1 GB/s | 11.4 GB/s | 3.9 GB/s | 3.6 GB/s | 5.5 GB/s | 9.6 GB/s | 0 / 0 (3 KB) / 17 (1349 KB) / 21 (1104 KB) / 32 (639 KB) / 56 (2 KB) |
| 256 KiB | 512 | 9.6 GB/s | 11.1 GB/s | 4.0 GB/s | 3.9 GB/s | 5.6 GB/s | 9.2 GB/s | 0 / 0 / 17 (1374 KB) / 21 (1155 KB) / 32 (664 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 9.5 GB/s | 10.9 GB/s | 3.9 GB/s | 3.5 GB/s | 5.0 GB/s | 9.2 GB/s | 0 / 0 (1 KB) / 18 (1424 KB) / 22 (1230 KB) / 33 (714 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 9.3 GB/s | 10.7 GB/s | 3.7 GB/s | 2.7 GB/s | 3.3 GB/s | 9.0 GB/s | 0 / 0 / 20 (1529 KB) / 25 (1425 KB) / 32 (617 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

