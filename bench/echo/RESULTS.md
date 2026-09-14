# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `e220206`.

![echo-plain](echo-plain.svg)

![echo-compressed](echo-compressed.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0, default build: SWAR masking and the shift-based UTF-8 validator
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
| 64 B | 32 | 52 MB/s | 49 MB/s | 50 MB/s | 38 MB/s | 39 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 53 MB/s | 53 MB/s | 51 MB/s | 42 MB/s | 41 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 54 MB/s | 51 MB/s | 50 MB/s | 42 MB/s | 44 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 1024 | 51 MB/s | 53 MB/s | 50 MB/s | 42 MB/s | 38 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 49 MB/s | 49 MB/s | 48 MB/s | 39 MB/s | 39 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 122 MB/s | 128 MB/s | 120 MB/s | 52 MB/s | 90 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 801 MB/s | 791 MB/s | 745 MB/s | 512 MB/s | 611 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 815 MB/s | 799 MB/s | 781 MB/s | 536 MB/s | 639 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 828 MB/s | 820 MB/s | 794 MB/s | 536 MB/s | 662 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 783 MB/s | 797 MB/s | 769 MB/s | 540 MB/s | 634 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 708 MB/s | 730 MB/s | 707 MB/s | 499 MB/s | 578 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.4 GB/s | 1.3 GB/s | 1.3 GB/s | 180 MB/s | 716 MB/s | 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 8.5 GB/s | 8.2 GB/s | 8.5 GB/s | 1.9 GB/s | 4.5 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 8.7 GB/s | 8.7 GB/s | 8.7 GB/s | 531 MB/s | 1.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 1.7 GB/s | 2.2 GB/s | 2.0 GB/s | 998 MB/s | 1.2 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 2.4 GB/s | 2.2 GB/s | 2.4 GB/s | 1.0 GB/s | 1.4 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 2.5 GB/s | 2.8 GB/s | 5.4 GB/s | 2.7 GB/s | 1.5 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.0 GB/s | 649 MB/s | 2.8 GB/s | 442 MB/s | 1.7 GB/s | 0 / 6 (598 KB) / 12 / 97 (707 KB) / 72 (5 KB) |
| 256 KiB | 32 | 16.4 GB/s | 5.0 GB/s | 15.2 GB/s | 4.1 GB/s | 10.1 GB/s | 0 / 5 (571 KB) / 12 / 96 (661 KB) / 72 (3 KB) |
| 256 KiB | 128 | 10.2 GB/s | 5.8 GB/s | 9.8 GB/s | 6.0 GB/s | 8.4 GB/s | 0 / 5 (536 KB) / 12 / 96 (630 KB) / 72 (3 KB) |
| 256 KiB | 512 | 8.4 GB/s | 5.7 GB/s | 8.0 GB/s | 5.1 GB/s | 7.5 GB/s | 0 / 5 (540 KB) / 12 / 96 (635 KB) / 72 (3 KB) |
| 256 KiB | 1024 | 8.1 GB/s | 5.6 GB/s | 7.3 GB/s | 5.1 GB/s | 7.0 GB/s | 0 / 5 (552 KB) / 12 / 96 (645 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.9 GB/s | 5.2 GB/s | 7.5 GB/s | 4.8 GB/s | 6.9 GB/s | 0 / 5 (574 KB) / 12 / 96 (669 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 38 MB/s | 32 MB/s | 26 MB/s | 27 MB/s | 31 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 37 MB/s | 32 MB/s | 28 MB/s | 29 MB/s | 32 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 35 MB/s | 29 MB/s | 26 MB/s | 26 MB/s | 29 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 34 MB/s | 32 MB/s | 30 MB/s | 32 MB/s | 30 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 33 MB/s | 33 MB/s | 32 MB/s | 32 MB/s | 27 MB/s | 26 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 76 MB/s | 60 MB/s | 56 MB/s | 52 MB/s | 44 MB/s | 60 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 552 MB/s | 475 MB/s | 399 MB/s | 408 MB/s | 419 MB/s | 440 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 510 MB/s | 435 MB/s | 373 MB/s | 397 MB/s | 419 MB/s | 416 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 389 MB/s | 347 MB/s | 271 MB/s | 274 MB/s | 306 MB/s | 321 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 332 MB/s | 305 MB/s | 242 MB/s | 241 MB/s | 275 MB/s | 282 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 288 MB/s | 282 MB/s | 232 MB/s | 229 MB/s | 259 MB/s | 268 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 674 MB/s | 589 MB/s | 590 MB/s | 567 MB/s | 163 MB/s | 571 MB/s | 0 / 0 / 1 / 6 / 25 (43 KB) / 20 |
| 16 KiB | 32 | 4.7 GB/s | 4.3 GB/s | 4.2 GB/s | 4.3 GB/s | 2.7 GB/s | 4.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 3.2 GB/s | 3.1 GB/s | 3.0 GB/s | 3.5 GB/s | 2.5 GB/s | 3.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.5 GB/s | 2.6 GB/s | 2.4 GB/s | 2.4 GB/s | 2.0 GB/s | 2.4 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.3 GB/s | 2.5 GB/s | 2.3 GB/s | 2.3 GB/s | 1.8 GB/s | 2.2 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.0 GB/s | 2.3 GB/s | 1.9 GB/s | 2.2 GB/s | 1.5 GB/s | 2.1 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 256 KiB | 1 | 1.3 GB/s | 1.3 GB/s | 337 MB/s | 371 MB/s | 348 MB/s | 1.2 GB/s | 0 (1 KB) / 0 (2 KB) / 19 (1451 KB) / 22 (1184 KB) / 39 (942 KB) / 56 (2 KB) |
| 256 KiB | 32 | 11.8 GB/s | 11.9 GB/s | 3.5 GB/s | 3.5 GB/s | 5.0 GB/s | 10.6 GB/s | 0 / 0 (1 KB) / 17 (1362 KB) / 20 (1091 KB) / 33 (658 KB) / 56 (3 KB) |
| 256 KiB | 128 | 9.6 GB/s | 10.4 GB/s | 3.5 GB/s | 3.6 GB/s | 5.3 GB/s | 8.6 GB/s | 0 / 0 (1 KB) / 17 (1350 KB) / 20 (1086 KB) / 32 (640 KB) / 56 (2 KB) |
| 256 KiB | 512 | 8.9 GB/s | 10.5 GB/s | 3.7 GB/s | 3.6 GB/s | 5.0 GB/s | 8.5 GB/s | 0 / 0 (1 KB) / 17 (1369 KB) / 22 (1183 KB) / 33 (668 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.8 GB/s | 10.2 GB/s | 3.7 GB/s | 3.3 GB/s | 4.8 GB/s | 8.6 GB/s | 0 / 0 / 18 (1438 KB) / 22 (1243 KB) / 33 (710 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.9 GB/s | 9.2 GB/s | 3.6 GB/s | 2.5 GB/s | 4.9 GB/s | 7.5 GB/s | 0 / 0 / 20 (1552 KB) / 26 (1460 KB) / 33 (674 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

