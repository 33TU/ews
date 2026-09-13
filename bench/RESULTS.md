# Echo benchmark results

Generated 2026-09-13 from `go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results` at ews commit `8d59667`.

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0
- gws: v1.10.2

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer.
- `gws`: event-driven `ReadLoop` with an `OnMessage` echo, gws's documented server shape.
- `gws-pull`: gws's `ReadMessage` in a loop, the like-for-like shape against ews.
- `coder`: coder/websocket with `Read` and `Write` in a loop.

Compression is permessage-deflate with context takeover in both directions. ews and gws run flate level 1; gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits, which ews does not implement. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | gws | gws-pull | coder | allocs/op ews / gws / gws-pull / coder |
|---|---|---|---|---|---|---|
| 64 B | 1 | 8 MB/s | 9 MB/s | 9 MB/s | 6 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 32 | 67 MB/s | 66 MB/s | 66 MB/s | 45 MB/s | 0 / 1 / 1 / 13 (1 KB) |
| 64 B | 128 | 77 MB/s | 77 MB/s | 77 MB/s | 52 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 512 | 79 MB/s | 77 MB/s | 77 MB/s | 57 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 1024 | 76 MB/s | 76 MB/s | 75 MB/s | 55 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 2048 | 67 MB/s | 65 MB/s | 66 MB/s | 49 MB/s | 0 / 1 / 1 / 13 |
| 1 KiB | 1 | 139 MB/s | 138 MB/s | 139 MB/s | 56 MB/s | 0 / 1 / 1 / 24 (2 KB) |
| 1 KiB | 32 | 1.0 GB/s | 1.0 GB/s | 1.0 GB/s | 492 MB/s | 0 / 1 / 1 / 24 (2 KB) |
| 1 KiB | 128 | 1.1 GB/s | 996 MB/s | 1.1 GB/s | 612 MB/s | 0 / 1 / 1 / 24 (2 KB) |
| 1 KiB | 512 | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 630 MB/s | 0 / 1 / 1 / 24 (2 KB) |
| 1 KiB | 1024 | 1.0 GB/s | 1.0 GB/s | 1.0 GB/s | 621 MB/s | 0 / 1 / 1 / 24 (2 KB) |
| 1 KiB | 2048 | 916 MB/s | 902 MB/s | 895 MB/s | 555 MB/s | 0 / 1 / 1 / 24 (2 KB) |
| 16 KiB | 1 | 1.5 GB/s | 1.4 GB/s | 1.4 GB/s | 176 MB/s | 0 / 1 / 1 / 61 (41 KB) |
| 16 KiB | 32 | 10.2 GB/s | 9.8 GB/s | 9.7 GB/s | 1.9 GB/s | 0 / 1 / 1 / 61 (39 KB) |
| 16 KiB | 128 | 10.8 GB/s | 10.1 GB/s | 10.3 GB/s | 2.5 GB/s | 0 / 1 / 1 / 61 (39 KB) |
| 16 KiB | 512 | 9.0 GB/s | 8.4 GB/s | 8.8 GB/s | 2.9 GB/s | 0 / 1 / 1 / 61 (39 KB) |
| 16 KiB | 1024 | 7.0 GB/s | 6.6 GB/s | 6.8 GB/s | 2.9 GB/s | 0 / 1 / 1 / 61 (39 KB) |
| 16 KiB | 2048 | 5.9 GB/s | 5.7 GB/s | 5.9 GB/s | 2.6 GB/s | 0 / 1 / 1 / 61 (39 KB) |
| 256 KiB | 1 | 3.3 GB/s | 578 MB/s | 773 MB/s | 438 MB/s | 0 / 6 (601 KB) / 6 (597 KB) / 97 (701 KB) |
| 256 KiB | 32 | 18.1 GB/s | 4.0 GB/s | 4.6 GB/s | 3.6 GB/s | 0 / 5 (573 KB) / 5 (573 KB) / 96 (672 KB) |
| 256 KiB | 128 | 10.2 GB/s | 6.8 GB/s | 6.8 GB/s | 6.1 GB/s | 0 / 5 (536 KB) / 5 (534 KB) / 96 (629 KB) |
| 256 KiB | 512 | 8.7 GB/s | 5.8 GB/s | 5.8 GB/s | 5.1 GB/s | 0 / 5 (535 KB) / 5 (536 KB) / 96 (627 KB) |
| 256 KiB | 1024 | 8.1 GB/s | 5.3 GB/s | 5.2 GB/s | 4.9 GB/s | 0 / 5 (542 KB) / 5 (540 KB) / 96 (632 KB) |
| 256 KiB | 2048 | 7.5 GB/s | 4.8 GB/s | 5.3 GB/s | 4.4 GB/s | 0 / 5 (551 KB) / 5 (553 KB) / 96 (648 KB) |

## Compressed

| Size | Conns | ews | gws | gws-pull | coder | allocs/op ews / gws / gws-pull / coder |
|---|---|---|---|---|---|---|
| 64 B | 1 | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 0 / 1 / 1 / 13 (1 KB) |
| 64 B | 32 | 32 MB/s | 27 MB/s | 27 MB/s | 32 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 128 | 36 MB/s | 29 MB/s | 29 MB/s | 35 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 512 | 18 MB/s | 17 MB/s | 17 MB/s | 22 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 1024 | 21 MB/s | 19 MB/s | 19 MB/s | 24 MB/s | 0 / 1 / 1 / 13 |
| 64 B | 2048 | 36 MB/s | 31 MB/s | 31 MB/s | 29 MB/s | 0 / 1 / 1 / 13 |
| 1 KiB | 1 | 72 MB/s | 60 MB/s | 60 MB/s | 53 MB/s | 0 / 1 / 1 / 16 (2 KB) |
| 1 KiB | 32 | 505 MB/s | 417 MB/s | 419 MB/s | 460 MB/s | 0 / 1 / 1 / 16 (2 KB) |
| 1 KiB | 128 | 525 MB/s | 423 MB/s | 420 MB/s | 474 MB/s | 0 / 1 / 1 / 16 (2 KB) |
| 1 KiB | 512 | 216 MB/s | 207 MB/s | 206 MB/s | 249 MB/s | 0 / 1 / 1 / 16 (2 KB) |
| 1 KiB | 1024 | 184 MB/s | 182 MB/s | 183 MB/s | 208 MB/s | 0 / 1 / 1 / 16 (2 KB) |
| 1 KiB | 2048 | 163 MB/s | 181 MB/s | 171 MB/s | 203 MB/s | 0 / 1 / 1 / 16 (2 KB) |
| 16 KiB | 1 | 684 MB/s | 615 MB/s | 614 MB/s | 188 MB/s | 0 / 1 / 1 / 25 (42 KB) |
| 16 KiB | 32 | 5.2 GB/s | 4.8 GB/s | 4.7 GB/s | 3.1 GB/s | 0 / 1 / 1 / 25 (38 KB) |
| 16 KiB | 128 | 4.0 GB/s | 3.7 GB/s | 3.9 GB/s | 2.8 GB/s | 0 / 1 / 1 / 25 (37 KB) |
| 16 KiB | 512 | 2.3 GB/s | 2.4 GB/s | 2.4 GB/s | 2.0 GB/s | 0 / 1 / 1 / 25 (37 KB) |
| 16 KiB | 1024 | 2.1 GB/s | 2.2 GB/s | 2.2 GB/s | 1.9 GB/s | 0 / 1 / 1 / 25 (38 KB) |
| 16 KiB | 2048 | 2.1 GB/s | 2.2 GB/s | 2.2 GB/s | 1.7 GB/s | 0 / 1 / 1 / 25 (37 KB) |
| 256 KiB | 1 | 1.5 GB/s | 361 MB/s | 416 MB/s | 286 MB/s | 0 / 18 (1443 KB) / 17 (1382 KB) / 39 (916 KB) |
| 256 KiB | 32 | 10.5 GB/s | 3.4 GB/s | 3.4 GB/s | 5.1 GB/s | 0 / 18 (1416 KB) / 18 (1409 KB) / 33 (686 KB) |
| 256 KiB | 128 | 10.1 GB/s | 3.7 GB/s | 3.7 GB/s | 5.4 GB/s | 0 / 17 (1335 KB) / 17 (1337 KB) / 32 (640 KB) |
| 256 KiB | 512 | 9.5 GB/s | 4.0 GB/s | 3.9 GB/s | 5.1 GB/s | 0 / 17 (1334 KB) / 16 (1332 KB) / 32 (650 KB) |
| 256 KiB | 1024 | 9.5 GB/s | 3.8 GB/s | 3.8 GB/s | 4.1 GB/s | 0 / 17 (1375 KB) / 17 (1368 KB) / 33 (690 KB) |
| 256 KiB | 2048 | 9.3 GB/s | 3.3 GB/s | 2.9 GB/s | 2.5 GB/s | 0 / 18 (1439 KB) / 18 (1442 KB) / 33 (736 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all three tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews. gws allocates a buffer above its pool threshold on every such message, over half a megabyte uncompressed and over a megabyte compressed.
- With hundreds of connections and 256 KiB messages both libraries are bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- gws's `ReadLoop` and `ReadMessage` share the whole frame path and measure the same within noise.
- coder/websocket allocates on every message and, with context takeover, resets a pooled flate writer with the 32 KB history per message, which is the priming cost ews avoids by keeping a compressor attached.
