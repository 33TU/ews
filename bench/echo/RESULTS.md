# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `32e2917`.

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
| 64 B | 1 | 9 MB/s | 9 MB/s | 8 MB/s | 7 MB/s | 6 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 67 MB/s | 66 MB/s | 65 MB/s | 45 MB/s | 50 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 79 MB/s | 68 MB/s | 63 MB/s | 54 MB/s | 56 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 80 MB/s | 78 MB/s | 74 MB/s | 58 MB/s | 59 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 77 MB/s | 75 MB/s | 75 MB/s | 58 MB/s | 57 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 68 MB/s | 64 MB/s | 65 MB/s | 50 MB/s | 49 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 137 MB/s | 139 MB/s | 132 MB/s | 61 MB/s | 96 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 1.1 GB/s | 1.0 GB/s | 997 MB/s | 564 MB/s | 775 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 1.2 GB/s | 1.2 GB/s | 1.2 GB/s | 639 MB/s | 840 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 1.2 GB/s | 1.2 GB/s | 1.1 GB/s | 676 MB/s | 893 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 650 MB/s | 816 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 910 MB/s | 929 MB/s | 836 MB/s | 586 MB/s | 665 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.5 GB/s | 1.4 GB/s | 1.4 GB/s | 186 MB/s | 773 MB/s | 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 10.6 GB/s | 10.2 GB/s | 9.9 GB/s | 1.9 GB/s | 5.8 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 11.2 GB/s | 10.9 GB/s | 11.0 GB/s | 2.3 GB/s | 6.2 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 9.2 GB/s | 9.0 GB/s | 8.6 GB/s | 3.2 GB/s | 5.3 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.8 GB/s | 6.7 GB/s | 6.7 GB/s | 2.8 GB/s | 4.4 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.8 GB/s | 5.7 GB/s | 5.6 GB/s | 2.7 GB/s | 4.0 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.2 GB/s | 608 MB/s | 3.0 GB/s | 437 MB/s | 1.9 GB/s | 0 / 6 (600 KB) / 12 / 97 (708 KB) / 72 (5 KB) |
| 256 KiB | 32 | 18.1 GB/s | 3.1 GB/s | 17.3 GB/s | 3.8 GB/s | 13.6 GB/s | 0 / 5 (569 KB) / 12 / 96 (669 KB) / 72 (3 KB) |
| 256 KiB | 128 | 10.1 GB/s | 5.9 GB/s | 9.7 GB/s | 5.8 GB/s | 9.0 GB/s | 0 / 5 (537 KB) / 12 / 96 (631 KB) / 72 (3 KB) |
| 256 KiB | 512 | 8.6 GB/s | 5.8 GB/s | 8.2 GB/s | 4.8 GB/s | 7.9 GB/s | 0 / 5 (541 KB) / 12 / 96 (638 KB) / 72 (2 KB) |
| 256 KiB | 1024 | 7.9 GB/s | 5.5 GB/s | 7.6 GB/s | 4.8 GB/s | 7.5 GB/s | 0 / 5 (549 KB) / 12 / 96 (639 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.6 GB/s | 5.1 GB/s | 7.6 GB/s | 4.7 GB/s | 7.3 GB/s | 0 / 5 (577 KB) / 12 / 96 (673 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 44 MB/s | 37 MB/s | 30 MB/s | 31 MB/s | 33 MB/s | 31 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 45 MB/s | 37 MB/s | 32 MB/s | 34 MB/s | 36 MB/s | 33 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 39 MB/s | 33 MB/s | 26 MB/s | 27 MB/s | 32 MB/s | 29 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 39 MB/s | 34 MB/s | 34 MB/s | 33 MB/s | 33 MB/s | 29 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 38 MB/s | 38 MB/s | 36 MB/s | 36 MB/s | 31 MB/s | 29 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 79 MB/s | 63 MB/s | 60 MB/s | 58 MB/s | 55 MB/s | 60 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 655 MB/s | 537 MB/s | 444 MB/s | 469 MB/s | 499 MB/s | 513 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 630 MB/s | 522 MB/s | 437 MB/s | 458 MB/s | 485 MB/s | 511 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 448 MB/s | 376 MB/s | 291 MB/s | 296 MB/s | 343 MB/s | 355 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 380 MB/s | 331 MB/s | 255 MB/s | 257 MB/s | 295 MB/s | 298 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 355 MB/s | 313 MB/s | 250 MB/s | 250 MB/s | 282 MB/s | 290 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 695 MB/s | 576 MB/s | 602 MB/s | 584 MB/s | 173 MB/s | 595 MB/s | 0 / 0 / 1 / 6 / 25 (42 KB) / 20 |
| 16 KiB | 32 | 5.3 GB/s | 5.2 GB/s | 4.7 GB/s | 4.8 GB/s | 3.1 GB/s | 4.7 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 128 | 3.6 GB/s | 3.6 GB/s | 3.5 GB/s | 3.8 GB/s | 2.8 GB/s | 3.5 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.6 GB/s | 2.8 GB/s | 2.6 GB/s | 2.6 GB/s | 2.1 GB/s | 2.5 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.4 GB/s | 2.6 GB/s | 2.4 GB/s | 2.4 GB/s | 1.9 GB/s | 2.3 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.4 GB/s | 2.6 GB/s | 2.3 GB/s | 2.3 GB/s | 1.7 GB/s | 2.3 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 392 MB/s | 445 MB/s | 281 MB/s | 1.3 GB/s | 0 / 0 (1 KB) / 19 (1452 KB) / 22 (1149 KB) / 38 (867 KB) / 56 (2 KB) |
| 256 KiB | 32 | 12.4 GB/s | 12.8 GB/s | 3.4 GB/s | 3.6 GB/s | 5.2 GB/s | 11.3 GB/s | 0 / 0 (1 KB) / 18 (1402 KB) / 22 (1160 KB) / 34 (699 KB) / 56 (3 KB) |
| 256 KiB | 128 | 10.3 GB/s | 11.7 GB/s | 3.9 GB/s | 3.8 GB/s | 5.6 GB/s | 9.8 GB/s | 0 / 0 (1 KB) / 17 (1352 KB) / 21 (1100 KB) / 32 (645 KB) / 56 (2 KB) |
| 256 KiB | 512 | 9.8 GB/s | 11.2 GB/s | 4.0 GB/s | 3.9 GB/s | 5.6 GB/s | 9.3 GB/s | 0 / 0 / 17 (1369 KB) / 21 (1152 KB) / 32 (663 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 9.6 GB/s | 11.1 GB/s | 3.9 GB/s | 3.4 GB/s | 5.1 GB/s | 9.3 GB/s | 0 / 0 / 18 (1431 KB) / 23 (1240 KB) / 33 (714 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 9.5 GB/s | 11.0 GB/s | 3.9 GB/s | 2.7 GB/s | 4.0 GB/s | 9.2 GB/s | 0 / 0 / 20 (1530 KB) / 26 (1436 KB) / 32 (618 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

