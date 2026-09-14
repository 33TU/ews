# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `0c18028`.

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
| 64 B | 1 | 8 MB/s | 9 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 62 MB/s | 60 MB/s | 58 MB/s | 41 MB/s | 40 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 68 MB/s | 67 MB/s | 64 MB/s | 48 MB/s | 46 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 70 MB/s | 65 MB/s | 63 MB/s | 48 MB/s | 51 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 67 MB/s | 67 MB/s | 65 MB/s | 46 MB/s | 49 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 57 MB/s | 63 MB/s | 59 MB/s | 45 MB/s | 44 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 135 MB/s | 134 MB/s | 130 MB/s | 56 MB/s | 87 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 981 MB/s | 1.0 GB/s | 976 MB/s | 567 MB/s | 686 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 1.1 GB/s | 1.0 GB/s | 1.0 GB/s | 606 MB/s | 775 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 1.1 GB/s | 1.0 GB/s | 1.0 GB/s | 645 MB/s | 801 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 1.0 GB/s | 1.0 GB/s | 964 MB/s | 584 MB/s | 721 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 805 MB/s | 857 MB/s | 741 MB/s | 523 MB/s | 567 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.4 GB/s | 1.2 GB/s | 1.3 GB/s | 201 MB/s | 732 MB/s | 0 / 1 / 6 / 61 (41 KB) / 16 |
| 16 KiB | 32 | 9.2 GB/s | 8.5 GB/s | 8.2 GB/s | 2.1 GB/s | 4.3 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 9.7 GB/s | 8.8 GB/s | 8.9 GB/s | 2.3 GB/s | 5.4 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 8.4 GB/s | 7.6 GB/s | 8.3 GB/s | 2.6 GB/s | 4.9 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 6.4 GB/s | 5.6 GB/s | 6.5 GB/s | 2.9 GB/s | 4.2 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 5.1 GB/s | 4.9 GB/s | 5.0 GB/s | 2.5 GB/s | 3.5 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 3.1 GB/s | 636 MB/s | 2.9 GB/s | 455 MB/s | 1.6 GB/s | 0 / 6 (598 KB) / 12 / 97 (703 KB) / 72 (5 KB) |
| 256 KiB | 32 | 18.0 GB/s | 3.8 GB/s | 16.8 GB/s | 3.3 GB/s | 12.9 GB/s | 0 / 5 (571 KB) / 12 / 96 (670 KB) / 72 (3 KB) |
| 256 KiB | 128 | 10.4 GB/s | 6.1 GB/s | 9.7 GB/s | 5.8 GB/s | 9.0 GB/s | 0 / 5 (535 KB) / 12 / 96 (631 KB) / 72 (2 KB) |
| 256 KiB | 512 | 8.7 GB/s | 5.9 GB/s | 8.1 GB/s | 5.2 GB/s | 7.8 GB/s | 0 / 5 (541 KB) / 12 / 96 (634 KB) / 72 (2 KB) |
| 256 KiB | 1024 | 8.1 GB/s | 5.6 GB/s | 7.8 GB/s | 5.1 GB/s | 7.5 GB/s | 0 / 5 (548 KB) / 12 / 96 (639 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 7.8 GB/s | 5.0 GB/s | 7.7 GB/s | 3.4 GB/s | 7.3 GB/s | 0 / 5 (580 KB) / 12 / 96 (666 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 42 MB/s | 37 MB/s | 30 MB/s | 32 MB/s | 36 MB/s | 32 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 44 MB/s | 37 MB/s | 33 MB/s | 33 MB/s | 36 MB/s | 33 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 39 MB/s | 32 MB/s | 26 MB/s | 27 MB/s | 31 MB/s | 30 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 39 MB/s | 34 MB/s | 34 MB/s | 34 MB/s | 32 MB/s | 30 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 38 MB/s | 37 MB/s | 36 MB/s | 36 MB/s | 31 MB/s | 28 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 80 MB/s | 61 MB/s | 59 MB/s | 58 MB/s | 53 MB/s | 61 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 638 MB/s | 542 MB/s | 451 MB/s | 466 MB/s | 485 MB/s | 518 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 618 MB/s | 505 MB/s | 462 MB/s | 476 MB/s | 506 MB/s | 491 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 429 MB/s | 368 MB/s | 282 MB/s | 286 MB/s | 332 MB/s | 342 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 359 MB/s | 321 MB/s | 245 MB/s | 245 MB/s | 284 MB/s | 290 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 338 MB/s | 304 MB/s | 242 MB/s | 240 MB/s | 269 MB/s | 277 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 669 MB/s | 605 MB/s | 596 MB/s | 599 MB/s | 179 MB/s | 582 MB/s | 0 / 0 / 1 / 6 / 25 (43 KB) / 20 |
| 16 KiB | 32 | 5.3 GB/s | 4.9 GB/s | 4.7 GB/s | 4.6 GB/s | 3.1 GB/s | 4.8 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 128 | 3.4 GB/s | 3.7 GB/s | 3.5 GB/s | 3.5 GB/s | 2.7 GB/s | 3.5 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.4 GB/s | 2.7 GB/s | 2.5 GB/s | 2.5 GB/s | 2.0 GB/s | 2.4 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.3 GB/s | 2.5 GB/s | 2.3 GB/s | 2.3 GB/s | 1.8 GB/s | 2.2 GB/s | 0 / 0 (1 KB) / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.3 GB/s | 2.4 GB/s | 2.2 GB/s | 2.2 GB/s | 1.6 GB/s | 2.2 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 380 MB/s | 441 MB/s | 278 MB/s | 1.3 GB/s | 0 / 0 (2 KB) / 18 (1445 KB) / 21 (1144 KB) / 38 (866 KB) / 56 (2 KB) |
| 256 KiB | 32 | 12.2 GB/s | 12.6 GB/s | 3.1 GB/s | 3.4 GB/s | 5.1 GB/s | 11.5 GB/s | 0 / 0 (1 KB) / 18 (1417 KB) / 22 (1170 KB) / 34 (700 KB) / 56 (2 KB) |
| 256 KiB | 128 | 10.3 GB/s | 10.7 GB/s | 3.9 GB/s | 4.0 GB/s | 5.7 GB/s | 9.7 GB/s | 0 / 0 / 17 (1346 KB) / 21 (1092 KB) / 32 (641 KB) / 56 (2 KB) |
| 256 KiB | 512 | 9.7 GB/s | 11.2 GB/s | 4.0 GB/s | 3.9 GB/s | 5.6 GB/s | 9.2 GB/s | 0 / 0 / 17 (1364 KB) / 21 (1151 KB) / 33 (671 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 9.5 GB/s | 11.1 GB/s | 4.0 GB/s | 3.5 GB/s | 5.4 GB/s | 9.2 GB/s | 0 / 0 (1 KB) / 18 (1423 KB) / 22 (1235 KB) / 33 (708 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 9.5 GB/s | 10.9 GB/s | 3.8 GB/s | 2.6 GB/s | 3.2 GB/s | 9.2 GB/s | 0 / 0 / 20 (1537 KB) / 26 (1421 KB) / 32 (616 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

