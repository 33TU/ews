# Echo benchmark results

Generated 2026-09-13 from `go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results` at ews commit `8dcf157`.

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
| 64 B | 32 | 44 MB/s | 44 MB/s | 45 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 49 MB/s | 48 MB/s | 48 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 46 MB/s | 48 MB/s | 47 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 48 MB/s | 46 MB/s | 45 MB/s | 0 / 1 / 1 |
| 64 B | 2048 | 40 MB/s | 42 MB/s | 43 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 82 MB/s | 87 MB/s | 83 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 698 MB/s | 683 MB/s | 691 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 733 MB/s | 705 MB/s | 716 MB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 681 MB/s | 696 MB/s | 699 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 682 MB/s | 689 MB/s | 647 MB/s | 0 / 1 / 1 |
| 1 KiB | 2048 | 594 MB/s | 584 MB/s | 568 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 947 MB/s | 1.0 GB/s | 1.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 7.4 GB/s | 7.3 GB/s | 7.1 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 7.6 GB/s | 7.5 GB/s | 7.5 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 5.5 GB/s | 5.8 GB/s | 5.5 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 4.0 GB/s | 4.5 GB/s | 4.6 GB/s | 0 / 1 / 1 |
| 16 KiB | 2048 | 3.7 GB/s | 4.1 GB/s | 4.0 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 3.0 GB/s | 418 MB/s | 393 MB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 32 | 11.2 GB/s | 4.6 GB/s | 4.8 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 128 | 6.3 GB/s | 5.2 GB/s | 5.0 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 512 | 5.5 GB/s | 4.6 GB/s | 4.4 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 1024 | 5.6 GB/s | 4.6 GB/s | 4.4 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |
| 256 KiB | 2048 | 5.4 GB/s | 4.1 GB/s | 3.8 GB/s | 0 / 5 (528 KB) / 5 (528 KB) |

## Compressed

| Size | Conns | ews | gws | gws-pull | allocs/op ews / gws / gws-pull |
|---|---|---|---|---|---|
| 64 B | 1 | 4 MB/s | 3 MB/s | 3 MB/s | 0 / 1 / 1 |
| 64 B | 32 | 26 MB/s | 21 MB/s | 22 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 28 MB/s | 23 MB/s | 22 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 15 MB/s | 15 MB/s | 16 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 25 MB/s | 21 MB/s | 20 MB/s | 0 / 1 / 1 |
| 64 B | 2048 | 28 MB/s | 26 MB/s | 25 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 46 MB/s | 47 MB/s | 47 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 398 MB/s | 339 MB/s | 337 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 401 MB/s | 336 MB/s | 326 MB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 169 MB/s | 165 MB/s | 164 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 153 MB/s | 156 MB/s | 150 MB/s | 0 / 1 / 1 |
| 1 KiB | 2048 | 163 MB/s | 150 MB/s | 156 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 506 MB/s | 512 MB/s | 518 MB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 4.2 GB/s | 4.0 GB/s | 3.9 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 3.1 GB/s | 3.0 GB/s | 3.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 1.9 GB/s | 1.9 GB/s | 1.8 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 1.9 GB/s | 1.9 GB/s | 1.7 GB/s | 0 / 1 / 1 |
| 16 KiB | 2048 | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 1.4 GB/s | 355 MB/s | 319 MB/s | 0 (1 KB) / 18 (1443 KB) / 18 (1412 KB) |
| 256 KiB | 32 | 10.6 GB/s | 3.1 GB/s | 3.2 GB/s | 0 / 17 (1345 KB) / 17 (1356 KB) |
| 256 KiB | 128 | 8.4 GB/s | 3.4 GB/s | 3.1 GB/s | 0 / 16 (1329 KB) / 16 (1329 KB) |
| 256 KiB | 512 | 8.1 GB/s | 3.3 GB/s | 3.1 GB/s | 0 / 17 (1346 KB) / 17 (1359 KB) |
| 256 KiB | 1024 | 8.3 GB/s | 2.9 GB/s | 3.1 GB/s | 0 / 18 (1411 KB) / 18 (1420 KB) |
| 256 KiB | 2048 | 8.0 GB/s | 2.7 GB/s | 2.8 GB/s | 0 / 19 (1473 KB) / 19 (1482 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all three tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews. gws allocates a buffer above its pool threshold on every such message, over half a megabyte uncompressed and over a megabyte compressed.
- With hundreds of connections and 256 KiB messages both libraries are bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- gws's `ReadLoop` and `ReadMessage` share the whole frame path and measure the same within noise.
