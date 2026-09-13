# Echo benchmark results

Generated 2026-09-13 from `go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results` at ews commit `88c0fdc`.

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
| 64 B | 1 | 9 MB/s | 9 MB/s | 8 MB/s | 0 / 1 / 1 |
| 64 B | 32 | 63 MB/s | 62 MB/s | 62 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 72 MB/s | 70 MB/s | 71 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 71 MB/s | 67 MB/s | 70 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 70 MB/s | 67 MB/s | 67 MB/s | 0 / 1 / 1 |
| 64 B | 2048 | 53 MB/s | 49 MB/s | 60 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 136 MB/s | 134 MB/s | 137 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 945 MB/s | 938 MB/s | 888 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 953 MB/s | 989 MB/s | 1.0 GB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 995 MB/s | 1.1 GB/s | 985 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 852 MB/s | 967 MB/s | 985 MB/s | 0 / 1 / 1 |
| 1 KiB | 2048 | 620 MB/s | 809 MB/s | 788 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 1.3 GB/s | 1.3 GB/s | 1.3 GB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 9.8 GB/s | 9.3 GB/s | 9.2 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 9.9 GB/s | 9.5 GB/s | 9.6 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 8.0 GB/s | 7.4 GB/s | 7.1 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 5.3 GB/s | 5.8 GB/s | 5.3 GB/s | 0 / 1 / 1 |
| 16 KiB | 2048 | 4.9 GB/s | 4.7 GB/s | 4.4 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 2.9 GB/s | 399 MB/s | 700 MB/s | 0 / 6 (595 KB) / 6 (595 KB) |
| 256 KiB | 32 | 15.8 GB/s | 4.2 GB/s | 4.5 GB/s | 0 / 5 (573 KB) / 5 (575 KB) |
| 256 KiB | 128 | 7.8 GB/s | 5.4 GB/s | 5.9 GB/s | 0 / 5 (535 KB) / 5 (534 KB) |
| 256 KiB | 512 | 7.6 GB/s | 4.9 GB/s | 5.2 GB/s | 0 / 5 (536 KB) / 5 (538 KB) |
| 256 KiB | 1024 | 6.6 GB/s | 5.0 GB/s | 4.4 GB/s | 0 / 5 (541 KB) / 5 (541 KB) |
| 256 KiB | 2048 | 6.8 GB/s | 3.7 GB/s | 4.6 GB/s | 0 / 5 (564 KB) / 5 (553 KB) |

## Compressed

| Size | Conns | ews | gws | gws-pull | allocs/op ews / gws / gws-pull |
|---|---|---|---|---|---|
| 64 B | 1 | 4 MB/s | 4 MB/s | 4 MB/s | 0 / 1 / 1 |
| 64 B | 32 | 32 MB/s | 26 MB/s | 25 MB/s | 0 / 1 / 1 |
| 64 B | 128 | 33 MB/s | 26 MB/s | 27 MB/s | 0 / 1 / 1 |
| 64 B | 512 | 15 MB/s | 14 MB/s | 19 MB/s | 0 / 1 / 1 |
| 64 B | 1024 | 20 MB/s | 18 MB/s | 18 MB/s | 0 / 1 / 1 |
| 64 B | 2048 | 30 MB/s | 29 MB/s | 27 MB/s | 0 / 1 / 1 |
| 1 KiB | 1 | 68 MB/s | 56 MB/s | 55 MB/s | 0 / 1 / 1 |
| 1 KiB | 32 | 478 MB/s | 400 MB/s | 404 MB/s | 0 / 1 / 1 |
| 1 KiB | 128 | 470 MB/s | 378 MB/s | 396 MB/s | 0 / 1 / 1 |
| 1 KiB | 512 | 195 MB/s | 188 MB/s | 175 MB/s | 0 / 1 / 1 |
| 1 KiB | 1024 | 170 MB/s | 146 MB/s | 151 MB/s | 0 / 1 / 1 |
| 1 KiB | 2048 | 171 MB/s | 180 MB/s | 181 MB/s | 0 / 1 / 1 |
| 16 KiB | 1 | 685 MB/s | 614 MB/s | 609 MB/s | 0 / 1 / 1 |
| 16 KiB | 32 | 5.1 GB/s | 4.6 GB/s | 4.6 GB/s | 0 / 1 / 1 |
| 16 KiB | 128 | 3.9 GB/s | 3.9 GB/s | 4.0 GB/s | 0 / 1 / 1 |
| 16 KiB | 512 | 2.3 GB/s | 2.4 GB/s | 2.4 GB/s | 0 / 1 / 1 |
| 16 KiB | 1024 | 2.1 GB/s | 2.2 GB/s | 2.2 GB/s | 0 / 1 / 1 |
| 16 KiB | 2048 | 2.0 GB/s | 2.2 GB/s | 2.2 GB/s | 0 / 1 / 1 |
| 256 KiB | 1 | 1.4 GB/s | 382 MB/s | 403 MB/s | 0 / 18 (1396 KB) / 17 (1380 KB) |
| 256 KiB | 32 | 12.3 GB/s | 3.4 GB/s | 3.5 GB/s | 0 / 18 (1404 KB) / 18 (1410 KB) |
| 256 KiB | 128 | 10.0 GB/s | 3.7 GB/s | 3.7 GB/s | 0 / 17 (1338 KB) / 17 (1338 KB) |
| 256 KiB | 512 | 9.5 GB/s | 3.9 GB/s | 3.9 GB/s | 0 / 17 (1333 KB) / 16 (1330 KB) |
| 256 KiB | 1024 | 9.4 GB/s | 3.7 GB/s | 3.7 GB/s | 0 / 17 (1377 KB) / 17 (1372 KB) |
| 256 KiB | 2048 | 9.3 GB/s | 3.2 GB/s | 3.4 GB/s | 0 / 18 (1457 KB) / 18 (1438 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all three tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews. gws allocates a buffer above its pool threshold on every such message, over half a megabyte uncompressed and over a megabyte compressed.
- With hundreds of connections and 256 KiB messages both libraries are bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- gws's `ReadLoop` and `ReadMessage` share the whole frame path and measure the same within noise.
