# Echo benchmark results

Generated 2026-09-15 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `6cda9bb`.

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
| 64 B | 1 | 8 MB/s | 8 MB/s | 8 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 66 MB/s | 65 MB/s | 65 MB/s | 63 MB/s | 43 MB/s | 48 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 73 MB/s | 71 MB/s | 73 MB/s | 71 MB/s | 51 MB/s | 52 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 74 MB/s | 70 MB/s | 70 MB/s | 69 MB/s | 49 MB/s | 56 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 73 MB/s | 68 MB/s | 72 MB/s | 70 MB/s | 45 MB/s | 47 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 49 MB/s | 51 MB/s | 59 MB/s | 50 MB/s | 39 MB/s | 35 MB/s | 0 / 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 127 MB/s | 132 MB/s | 136 MB/s | 121 MB/s | 60 MB/s | 80 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 1.0 GB/s | 927 MB/s | 983 MB/s | 917 MB/s | 548 MB/s | 714 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 1.0 GB/s | 1.1 GB/s | 1.1 GB/s | 1.0 GB/s | 612 MB/s | 776 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 1.1 GB/s | 1.0 GB/s | 1.1 GB/s | 881 MB/s | 541 MB/s | 767 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 878 MB/s | 798 MB/s | 957 MB/s | 887 MB/s | 566 MB/s | 721 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 782 MB/s | 814 MB/s | 747 MB/s | 695 MB/s | 538 MB/s | 583 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 1.4 GB/s | 1.3 GB/s | 174 MB/s | 552 MB/s | 0 / 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 9.0 GB/s | 9.7 GB/s | 9.3 GB/s | 8.9 GB/s | 1.7 GB/s | 5.4 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 10.7 GB/s | 9.5 GB/s | 9.4 GB/s | 9.4 GB/s | 2.2 GB/s | 4.9 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 8.6 GB/s | 7.4 GB/s | 8.3 GB/s | 8.6 GB/s | 3.1 GB/s | 5.1 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.6 GB/s | 6.5 GB/s | 6.6 GB/s | 6.5 GB/s | 2.7 GB/s | 4.3 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.5 GB/s | 5.5 GB/s | 5.6 GB/s | 5.5 GB/s | 2.4 GB/s | 3.9 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 2.9 GB/s | 2.5 GB/s | 523 MB/s | 3.2 GB/s | 524 MB/s | 1.8 GB/s | 0 / 0 / 6 (597 KB) / 12 / 97 (702 KB) / 72 (4 KB) |
| 256 KiB | 32 | 17.7 GB/s | 16.8 GB/s | 3.6 GB/s | 16.0 GB/s | 4.0 GB/s | 13.1 GB/s | 0 / 0 / 5 (572 KB) / 12 / 96 (670 KB) / 72 (3 KB) |
| 256 KiB | 128 | 8.7 GB/s | 8.1 GB/s | 5.1 GB/s | 8.7 GB/s | 5.4 GB/s | 8.0 GB/s | 0 / 0 / 5 (537 KB) / 12 / 96 (630 KB) / 72 (3 KB) |
| 256 KiB | 512 | 7.4 GB/s | 7.3 GB/s | 4.5 GB/s | 7.3 GB/s | 4.5 GB/s | 6.9 GB/s | 0 / 0 / 5 (545 KB) / 12 / 96 (635 KB) / 72 (3 KB) |
| 256 KiB | 1024 | 7.2 GB/s | 7.1 GB/s | 5.0 GB/s | 7.1 GB/s | 4.9 GB/s | 6.9 GB/s | 0 / 0 / 5 (556 KB) / 12 / 96 (644 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.3 GB/s | 7.2 GB/s | 5.0 GB/s | 6.5 GB/s | 4.1 GB/s | 6.6 GB/s | 0 / 0 / 5 (579 KB) / 12 / 96 (673 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | ews-stream | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 41 MB/s | 34 MB/s | 41 MB/s | 28 MB/s | 29 MB/s | 32 MB/s | 28 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 42 MB/s | 32 MB/s | 38 MB/s | 28 MB/s | 31 MB/s | 35 MB/s | 29 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 32 MB/s | 27 MB/s | 34 MB/s | 23 MB/s | 25 MB/s | 29 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 34 MB/s | 32 MB/s | 31 MB/s | 31 MB/s | 31 MB/s | 30 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 34 MB/s | 32 MB/s | 34 MB/s | 34 MB/s | 31 MB/s | 27 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 76 MB/s | 62 MB/s | 78 MB/s | 59 MB/s | 58 MB/s | 53 MB/s | 60 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 537 MB/s | 481 MB/s | 561 MB/s | 382 MB/s | 398 MB/s | 423 MB/s | 484 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 552 MB/s | 445 MB/s | 485 MB/s | 391 MB/s | 353 MB/s | 393 MB/s | 437 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 339 MB/s | 287 MB/s | 329 MB/s | 215 MB/s | 227 MB/s | 246 MB/s | 270 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 340 MB/s | 266 MB/s | 302 MB/s | 205 MB/s | 213 MB/s | 272 MB/s | 244 MB/s | 0 / 0 (3 KB) / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 267 MB/s | 250 MB/s | 278 MB/s | 203 MB/s | 208 MB/s | 215 MB/s | 240 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 667 MB/s | 593 MB/s | 614 MB/s | 575 MB/s | 553 MB/s | 189 MB/s | 520 MB/s | 0 / 0 / 0 / 1 / 6 / 25 (45 KB) / 20 |
| 16 KiB | 32 | 4.6 GB/s | 4.6 GB/s | 4.7 GB/s | 4.2 GB/s | 4.3 GB/s | 2.6 GB/s | 4.3 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 2.7 GB/s | 3.1 GB/s | 2.8 GB/s | 2.7 GB/s | 3.1 GB/s | 2.0 GB/s | 2.9 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.1 GB/s | 2.2 GB/s | 2.1 GB/s | 2.0 GB/s | 2.1 GB/s | 1.7 GB/s | 2.0 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.0 GB/s | 2.0 GB/s | 1.8 GB/s | 1.9 GB/s | 1.8 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 (1 KB) / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 1.9 GB/s | 2.1 GB/s | 1.9 GB/s | 1.9 GB/s | 1.8 GB/s | 1.3 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 (1 KB) / 25 (38 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 1.2 GB/s | 415 MB/s | 469 MB/s | 343 MB/s | 1.2 GB/s | 0 (1 KB) / 0 (1 KB) / 0 / 18 (1400 KB) / 22 (1165 KB) / 40 (964 KB) / 56 (3 KB) |
| 256 KiB | 32 | 12.2 GB/s | 11.4 GB/s | 9.5 GB/s | 3.4 GB/s | 3.4 GB/s | 4.5 GB/s | 9.2 GB/s | 0 / 0 (1 KB) / 0 / 17 (1358 KB) / 20 (1087 KB) / 33 (658 KB) / 56 (2 KB) |
| 256 KiB | 128 | 9.1 GB/s | 10.8 GB/s | 9.0 GB/s | 3.5 GB/s | 3.6 GB/s | 5.2 GB/s | 9.2 GB/s | 0 / 0 (1 KB) / 0 / 17 (1343 KB) / 20 (1095 KB) / 32 (643 KB) / 56 (2 KB) |
| 256 KiB | 512 | 8.8 GB/s | 11.0 GB/s | 9.5 GB/s | 3.7 GB/s | 3.5 GB/s | 5.2 GB/s | 8.9 GB/s | 0 / 0 / 0 / 17 (1368 KB) / 21 (1173 KB) / 33 (668 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 9.1 GB/s | 10.4 GB/s | 9.2 GB/s | 3.7 GB/s | 3.2 GB/s | 4.3 GB/s | 8.4 GB/s | 0 / 0 (1 KB) / 0 / 18 (1442 KB) / 23 (1273 KB) / 33 (724 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.7 GB/s | 10.6 GB/s | 8.8 GB/s | 3.4 GB/s | 2.2 GB/s | 1.4 GB/s | 8.1 GB/s | 0 / 0 / 0 / 20 (1562 KB) / 26 (1453 KB) / 33 (625 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

