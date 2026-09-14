# Echo benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Echo -benchtime 1s | go run ../cmd/results` at ews commit `d9b4a55`.

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
| 64 B | 1 | 8 MB/s | 8 MB/s | 6 MB/s | 5 MB/s | 5 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 32 | 52 MB/s | 52 MB/s | 49 MB/s | 37 MB/s | 39 MB/s | 0 / 1 / 6 / 13 (1 KB) / 16 |
| 64 B | 128 | 54 MB/s | 53 MB/s | 51 MB/s | 39 MB/s | 37 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 512 | 50 MB/s | 51 MB/s | 49 MB/s | 38 MB/s | 40 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 1024 | 47 MB/s | 44 MB/s | 46 MB/s | 39 MB/s | 39 MB/s | 0 / 1 / 6 / 13 / 16 |
| 64 B | 2048 | 44 MB/s | 45 MB/s | 42 MB/s | 32 MB/s | 35 MB/s | 0 / 1 / 6 / 13 / 16 |
| 1 KiB | 1 | 121 MB/s | 127 MB/s | 122 MB/s | 49 MB/s | 90 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 32 | 797 MB/s | 86 MB/s | 77 MB/s | 49 MB/s | 61 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 128 | 84 MB/s | 133 MB/s | 135 MB/s | 84 MB/s | 104 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 512 | 159 MB/s | 148 MB/s | 158 MB/s | 115 MB/s | 140 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 1024 | 679 MB/s | 742 MB/s | 711 MB/s | 522 MB/s | 594 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 1 KiB | 2048 | 632 MB/s | 633 MB/s | 624 MB/s | 445 MB/s | 470 MB/s | 0 / 1 / 6 / 24 (2 KB) / 16 |
| 16 KiB | 1 | 1.3 GB/s | 1.3 GB/s | 1.2 GB/s | 146 MB/s | 670 MB/s | 0 / 1 / 6 / 61 (40 KB) / 16 |
| 16 KiB | 32 | 8.9 GB/s | 8.6 GB/s | 8.5 GB/s | 1.8 GB/s | 4.4 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 128 | 8.9 GB/s | 8.6 GB/s | 8.5 GB/s | 2.3 GB/s | 3.5 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 512 | 6.0 GB/s | 5.9 GB/s | 6.1 GB/s | 2.4 GB/s | 3.6 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 1024 | 5.3 GB/s | 5.1 GB/s | 1.3 GB/s | 397 MB/s | 561 MB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 16 KiB | 2048 | 1.4 GB/s | 1.5 GB/s | 4.3 GB/s | 2.4 GB/s | 3.1 GB/s | 0 / 1 / 6 / 61 (39 KB) / 16 |
| 256 KiB | 1 | 2.9 GB/s | 446 MB/s | 2.6 GB/s | 336 MB/s | 1.4 GB/s | 0 / 6 (600 KB) / 12 / 97 (705 KB) / 72 (5 KB) |
| 256 KiB | 32 | 13.2 GB/s | 4.2 GB/s | 12.3 GB/s | 4.3 GB/s | 9.9 GB/s | 0 / 5 (572 KB) / 12 / 96 (656 KB) / 72 (3 KB) |
| 256 KiB | 128 | 7.8 GB/s | 4.5 GB/s | 8.0 GB/s | 5.0 GB/s | 7.3 GB/s | 0 / 5 (535 KB) / 12 / 96 (627 KB) / 72 (5 KB) |
| 256 KiB | 512 | 7.9 GB/s | 5.1 GB/s | 7.3 GB/s | 4.8 GB/s | 7.0 GB/s | 0 / 5 (536 KB) / 12 / 96 (627 KB) / 72 (2 KB) |
| 256 KiB | 1024 | 7.2 GB/s | 5.1 GB/s | 6.8 GB/s | 4.8 GB/s | 6.8 GB/s | 0 / 5 (541 KB) / 12 / 96 (632 KB) / 72 (2 KB) |
| 256 KiB | 2048 | 6.9 GB/s | 4.9 GB/s | 7.4 GB/s | 4.2 GB/s | 6.3 GB/s | 0 / 5 (554 KB) / 12 / 96 (652 KB) / 72 (2 KB) |

## Compressed

| Size | Conns | ews | ews-shared | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 37 MB/s | 30 MB/s | 25 MB/s | 26 MB/s | 29 MB/s | 25 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 37 MB/s | 29 MB/s | 26 MB/s | 25 MB/s | 28 MB/s | 24 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 30 MB/s | 20 MB/s | 20 MB/s | 20 MB/s | 23 MB/s | 21 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 24 MB/s | 22 MB/s | 19 MB/s | 20 MB/s | 21 MB/s | 18 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 31 MB/s | 25 MB/s | 24 MB/s | 25 MB/s | 25 MB/s | 22 MB/s | 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 74 MB/s | 60 MB/s | 56 MB/s | 56 MB/s | 55 MB/s | 59 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 542 MB/s | 369 MB/s | 255 MB/s | 269 MB/s | 279 MB/s | 289 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 374 MB/s | 309 MB/s | 277 MB/s | 283 MB/s | 294 MB/s | 304 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 343 MB/s | 280 MB/s | 237 MB/s | 231 MB/s | 250 MB/s | 257 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 314 MB/s | 266 MB/s | 216 MB/s | 217 MB/s | 250 MB/s | 252 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 264 MB/s | 280 MB/s | 174 MB/s | 228 MB/s | 229 MB/s | 236 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 661 MB/s | 580 MB/s | 553 MB/s | 579 MB/s | 189 MB/s | 573 MB/s | 0 / 0 / 1 / 6 / 25 (41 KB) / 20 |
| 16 KiB | 32 | 4.9 GB/s | 4.6 GB/s | 4.2 GB/s | 4.2 GB/s | 2.8 GB/s | 4.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 3.2 GB/s | 3.4 GB/s | 3.3 GB/s | 3.4 GB/s | 2.4 GB/s | 3.1 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.4 GB/s | 2.6 GB/s | 2.3 GB/s | 2.4 GB/s | 2.0 GB/s | 2.3 GB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.2 GB/s | 2.3 GB/s | 2.1 GB/s | 2.1 GB/s | 390 MB/s | 470 MB/s | 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 2048 | 742 MB/s | 2.1 GB/s | 2.1 GB/s | 2.1 GB/s | 1.6 GB/s | 2.0 GB/s | 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 385 MB/s | 440 MB/s | 296 MB/s | 1.3 GB/s | 0 / 0 (2 KB) / 18 (1440 KB) / 22 (1155 KB) / 37 (848 KB) / 56 (2 KB) |
| 256 KiB | 32 | 10.4 GB/s | 12.0 GB/s | 3.4 GB/s | 3.6 GB/s | 4.8 GB/s | 10.0 GB/s | 0 / 0 / 18 (1415 KB) / 22 (1141 KB) / 33 (661 KB) / 56 (2 KB) |
| 256 KiB | 128 | 9.1 GB/s | 10.2 GB/s | 3.6 GB/s | 3.8 GB/s | 5.0 GB/s | 8.7 GB/s | 0 / 0 / 17 (1339 KB) / 20 (1080 KB) / 32 (634 KB) / 56 (2 KB) |
| 256 KiB | 512 | 9.0 GB/s | 10.2 GB/s | 3.8 GB/s | 3.7 GB/s | 5.2 GB/s | 8.7 GB/s | 0 / 0 / 17 (1338 KB) / 21 (1118 KB) / 32 (646 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 9.0 GB/s | 2.5 GB/s | 1.5 GB/s | 1.5 GB/s | 1.8 GB/s | 2.5 GB/s | 0 / 0 / 18 (1405 KB) / 22 (1213 KB) / 33 (719 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 9.0 GB/s | 10.4 GB/s | 2.4 GB/s | 2.4 GB/s | 3.9 GB/s | 7.4 GB/s | 0 / 0 / 19 (1483 KB) / 25 (1413 KB) / 34 (747 KB) / 56 (2 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

