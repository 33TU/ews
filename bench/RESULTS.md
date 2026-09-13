# Echo benchmark results

Generated 2026-09-13 from `go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results` at ews commit `c227d98`.

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
| 64 B | 32 | 45 MB/s | 46 MB/s | 46 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 50 MB/s | 46 MB/s | 49 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 48 MB/s | 46 MB/s | 44 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 43 MB/s | 44 MB/s | 46 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 85 MB/s | 78 MB/s | 81 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 680 MB/s | 666 MB/s | 658 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 725 MB/s | 693 MB/s | 683 MB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 702 MB/s | 698 MB/s | 701 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 644 MB/s | 664 MB/s | 671 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 925 MB/s | 978 MB/s | 986 MB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 7.2 GB/s | 7.1 GB/s | 7.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 7.6 GB/s | 7.3 GB/s | 7.2 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 5.3 GB/s | 5.4 GB/s | 5.8 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 3.9 GB/s | 4.6 GB/s | 4.6 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 2.7 GB/s | 377 MB/s | 396 MB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 32 | 12.4 GB/s | 5.4 GB/s | 5.1 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 128 | 6.0 GB/s | 5.2 GB/s | 5.1 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 512 | 5.7 GB/s | 4.5 GB/s | 4.8 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 1024 | 5.2 GB/s | 4.6 GB/s | 4.5 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |

## Compressed

| Size | Conns | ews | gws | gws-pull | allocs/op ews / gws / gws-pull |
|---|---|---|---|---|---|
| 64 B | 1 | 3 MB/s | 3 MB/s | 3 MB/s | 0 / 1 / 1 |
| 64 B | 32 | 26 MB/s | 21 MB/s | 21 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 27 MB/s | 22 MB/s | 22 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 17 MB/s | 17 MB/s | 16 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 18 MB/s | 21 MB/s | 18 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 48 MB/s | 43 MB/s | 44 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 402 MB/s | 336 MB/s | 328 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 397 MB/s | 322 MB/s | 331 MB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 200 MB/s | 169 MB/s | 175 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 173 MB/s | 157 MB/s | 166 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 540 MB/s | 481 MB/s | 492 MB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 4.3 GB/s | 3.9 GB/s | 3.8 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 2.7 GB/s | 2.9 GB/s | 3.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 1.8 GB/s | 1.9 GB/s | 1.9 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 1.7 GB/s | 1.8 GB/s | 1.7 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 1.4 GB/s | 301 MB/s | 325 MB/s | 0 / 16 (1288 KB) / 16 (1288 KB) |
| 256 KiB | 32 | 8.1 GB/s | 3.3 GB/s | 3.0 GB/s | 0 / 16 (1287 KB) / 16 (1287 KB) |
| 256 KiB | 128 | 7.0 GB/s | 3.5 GB/s | 3.2 GB/s | 0 / 16 (1287 KB) / 16 (1287 KB) |
| 256 KiB | 512 | 6.8 GB/s | 3.5 GB/s | 3.1 GB/s | 0 / 16 (1287 KB) / 16 (1287 KB) |
| 256 KiB | 1024 | 6.4 GB/s | 2.9 GB/s | 3.1 GB/s | 0 / 16 (1287 KB) / 16 (1287 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all three tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews. gws allocates a buffer above its pool threshold on every such message, over half a megabyte uncompressed and over a megabyte compressed.
- With hundreds of connections and 256 KiB messages both libraries are bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- gws's `ReadLoop` and `ReadMessage` share the whole frame path and measure the same within noise.
