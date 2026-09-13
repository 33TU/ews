# Echo benchmark results

Generated 2026-09-13 from `go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results` at ews commit `5e86471`.

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0
- gws: v1.10.2

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer.
- `gws`: event-driven `ReadLoop` with an `OnMessage` echo, gws's documented server shape.
- `gws-pull`: gws's `ReadMessage` in a loop, the like-for-like shape against ews.

Compression is permessage-deflate at flate level 1 with context takeover in both directions. gws is configured for 15-bit windows to match the 32 KB window ews uses; its default is 12 bits, which ews does not implement. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | gws | gws-pull | allocs/op ews / gws / gws-pull |
|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 5 MB/s | 5 MB/s | 0 / 1 / 1 |
| 64 B | 32 | 45 MB/s | 45 MB/s | 46 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 49 MB/s | 48 MB/s | 48 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 49 MB/s | 47 MB/s | 48 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 46 MB/s | 46 MB/s | 48 MB/s | 0 / 1 / 1 |
| 64 B | 2048 | 39 MB/s | 40 MB/s | 44 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 82 MB/s | 79 MB/s | 83 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 693 MB/s | 689 MB/s | 674 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 726 MB/s | 716 MB/s | 718 MB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 733 MB/s | 672 MB/s | 701 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 691 MB/s | 696 MB/s | 617 MB/s | 0 / 1 / 1 |
| 1 KiB | 2048 | 585 MB/s | 602 MB/s | 583 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 1.0 GB/s | 1.0 GB/s | 1.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 7.0 GB/s | 7.0 GB/s | 7.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 7.9 GB/s | 7.7 GB/s | 7.7 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 5.7 GB/s | 5.8 GB/s | 6.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 5.4 GB/s | 5.0 GB/s | 5.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 2048 | 4.9 GB/s | 4.7 GB/s | 4.5 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 3.1 GB/s | 391 MB/s | 355 MB/s | 0 / 6 (593 KB) / 6 (595 KB) |
| 256 KiB | 32 | 14.8 GB/s | 4.6 GB/s | 4.3 GB/s | 0 / 5 (574 KB) / 5 (572 KB) |
| 256 KiB | 128 | 8.5 GB/s | 5.3 GB/s | 5.6 GB/s | 0 / 5 (535 KB) / 5 (535 KB) |
| 256 KiB | 512 | 7.0 GB/s | 5.1 GB/s | 5.0 GB/s | 0 / 5 (536 KB) / 5 (537 KB) |
| 256 KiB | 1024 | 7.2 GB/s | 4.8 GB/s | 4.8 GB/s | 0 / 5 (541 KB) / 5 (540 KB) |
| 256 KiB | 2048 | 6.6 GB/s | 4.8 GB/s | 4.7 GB/s | 0 / 5 (553 KB) / 5 (556 KB) |

## Compressed

| Size | Conns | ews | gws | gws-pull | allocs/op ews / gws / gws-pull |
|---|---|---|---|---|---|
| 64 B | 1 | 3 MB/s | 3 MB/s | 3 MB/s | 0 / 1 / 1 |
| 64 B | 32 | 26 MB/s | 22 MB/s | 22 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 27 MB/s | 22 MB/s | 22 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 16 MB/s | 16 MB/s | 15 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 22 MB/s | 19 MB/s | 20 MB/s | 0 / 1 / 1 |
| 64 B | 2048 | 28 MB/s | 25 MB/s | 26 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 51 MB/s | 45 MB/s | 46 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 393 MB/s | 336 MB/s | 340 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 403 MB/s | 336 MB/s | 318 MB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 170 MB/s | 173 MB/s | 163 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 154 MB/s | 159 MB/s | 150 MB/s | 0 / 1 / 1 |
| 1 KiB | 2048 | 164 MB/s | 157 MB/s | 159 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 535 MB/s | 513 MB/s | 498 MB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 4.2 GB/s | 3.7 GB/s | 3.8 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 3.0 GB/s | 3.1 GB/s | 3.3 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 1.9 GB/s | 1.8 GB/s | 2.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 1.9 GB/s | 1.8 GB/s | 1.9 GB/s | 0 / 1 / 1 |
| 16 KiB | 2048 | 1.7 GB/s | 1.8 GB/s | 1.9 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 1.4 GB/s | 262 MB/s | 335 MB/s | 0 / 18 (1447 KB) / 17 (1398 KB) |
| 256 KiB | 32 | 10.9 GB/s | 3.2 GB/s | 3.2 GB/s | 0 / 17 (1357 KB) / 17 (1360 KB) |
| 256 KiB | 128 | 8.4 GB/s | 3.2 GB/s | 3.3 GB/s | 0 / 16 (1330 KB) / 16 (1331 KB) |
| 256 KiB | 512 | 8.5 GB/s | 3.0 GB/s | 3.3 GB/s | 0 / 17 (1349 KB) / 17 (1341 KB) |
| 256 KiB | 1024 | 7.6 GB/s | 3.2 GB/s | 3.1 GB/s | 0 / 17 (1388 KB) / 17 (1376 KB) |
| 256 KiB | 2048 | 8.1 GB/s | 2.5 GB/s | 2.7 GB/s | 0 / 19 (1481 KB) / 19 (1483 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all three tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews. gws allocates a buffer above its pool threshold on every such message, over half a megabyte uncompressed and over a megabyte compressed.
- With hundreds of connections and 256 KiB messages both libraries are bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- gws's `ReadLoop` and `ReadMessage` share the whole frame path and measure the same within noise.
