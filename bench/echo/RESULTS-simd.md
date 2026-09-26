# Echo benchmark results

Run at ews commit `9186b3e` with `go test -run '^$' -bench Echo -benchtime 1s`; tables and charts generated from the saved output by `go run ../cmd/results` on 2026-09-26.

![echo-sizes-compressed-simd](echo-sizes-compressed-simd.svg)

![echo-sizes-nocontext-simd](echo-sizes-nocontext-simd.svg)

![echo-sizes-simd](echo-sizes-simd.svg)

![echo-plain-simd](echo-plain-simd.svg)

![echo-compressed-simd](echo-compressed-simd.svg)

![echo-nocontext-simd](echo-nocontext-simd.svg)

## Setup

- CPU: AMD Ryzen 9 9950X3D 16-Core Processor
- Kernel: 7.2.2-1-cachyos
- Go: go1.27.1-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
- GOMAXPROCS: 8
- gws: v1.10.2
- coder/websocket: v1.8.15
- gorilla/websocket: v1.5.3

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer. With compression it keeps a compressor attached per connection.
- `ews-shared`: the same with `CompressionShared`, borrowing a pooled compressor per message as gws and coder do. Takeover table only; it is identical to `ews` otherwise.
- `ews-stream`: `NextMessage` then `WriteFrom` reading the connection itself, so no message is held whole and compressed input is inflated as its frames arrive. Messages above `FragmentSize` (64 KiB) go out as several frames, each compressed chunk flushed on its own, and each chunk is copied out of the read buffer, which is what separates it from `ews` at 256 KiB. This is ews's streaming shape, against `gws-stream` and `coder-stream`.
- `ews-events`: the same echo as an `events.Handler` behind `events.HTTP`, one method per event, against `gws-events`.
- `gws`: gws's `ReadMessage` and `WriteMessage` in a loop, the like-for-like shape against ews. Its event-driven `ReadLoop` shares the frame path and measured the same within noise.
- `gws-stream`: gws's `NextReader` piped into `WriteFile`, so no message is held whole.
- `gws-events`: gws's `ReadLoop` driving an `OnMessage` handler that writes the message back, the shape gws documents.
- `coder`: coder/websocket with `Read` and `Write` in a loop.
- `coder-stream`: coder/websocket piping `Reader` into `Writer` through a reusable buffer, so no message is held whole.
- `gorilla`: gorilla/websocket with `ReadMessage` and `WriteMessage` in a loop. Uncompressed and no-takeover tables only, since gorilla negotiates only `no_context_takeover`.
- `gorilla-stream`: gorilla/websocket piping `NextReader` into `NextWriter` through a reusable buffer.

Compression is permessage-deflate at flate level 1, every message compressed, in two modes that are separate tables because they are different work. With context takeover each direction keeps a 32 KB history that every message extends, so the inflater is primed with a dictionary per message and both ends copy history; it compresses real traffic far better. Without takeover each message is compressed on its own. gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. gorilla/websocket uses the standard library's flate. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are dominated by loopback round-trip latency and vary more between runs than large-message and allocation figures. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | ews-stream | ews-events | gws | gws-stream | gws-events | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / ews-events / gws / gws-stream / gws-events / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 19 MB/s | 19 MB/s | 19 MB/s | 19 MB/s | 18 MB/s | 18 MB/s | 14 MB/s | 13 MB/s | 18 MB/s | 18 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 32 | 107 MB/s | 105 MB/s | 106 MB/s | 107 MB/s | 104 MB/s | 106 MB/s | 80 MB/s | 81 MB/s | 100 MB/s | 104 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 128 | 118 MB/s | 117 MB/s | 115 MB/s | 117 MB/s | 113 MB/s | 118 MB/s | 84 MB/s | 85 MB/s | 108 MB/s | 116 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 512 | 120 MB/s | 118 MB/s | 120 MB/s | 120 MB/s | 114 MB/s | 119 MB/s | 89 MB/s | 87 MB/s | 112 MB/s | 117 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 1024 | 119 MB/s | 119 MB/s | 120 MB/s | 120 MB/s | 115 MB/s | 119 MB/s | 90 MB/s | 86 MB/s | 114 MB/s | 118 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 2048 | 120 MB/s | 119 MB/s | 119 MB/s | 120 MB/s | 115 MB/s | 119 MB/s | 88 MB/s | 85 MB/s | 113 MB/s | 118 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 1 KiB | 1 | 289 MB/s | 285 MB/s | 288 MB/s | 284 MB/s | 274 MB/s | 283 MB/s | 178 MB/s | 195 MB/s | 244 MB/s | 280 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 32 | 1.7 GB/s | 1.6 GB/s | 1.7 GB/s | 1.6 GB/s | 1.6 GB/s | 1.6 GB/s | 967 MB/s | 1.3 GB/s | 1.3 GB/s | 1.6 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 128 | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.7 GB/s | 1.8 GB/s | 1.0 GB/s | 1.3 GB/s | 1.4 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 512 | 1.9 GB/s | 1.8 GB/s | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 1024 | 1.9 GB/s | 1.8 GB/s | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 2048 | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 16 KiB | 1 | 3.0 GB/s | 3.0 GB/s | 3.0 GB/s | 2.9 GB/s | 3.0 GB/s | 3.0 GB/s | 989 MB/s | 1.6 GB/s | 1.5 GB/s | 1.3 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (40 KB) / 16 / 16 (38 KB) / 2 |
| 16 KiB | 32 | 18.5 GB/s | 18.6 GB/s | 18.8 GB/s | 18.6 GB/s | 18.2 GB/s | 18.1 GB/s | 4.1 GB/s | 10.3 GB/s | 5.3 GB/s | 8.6 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 128 | 19.8 GB/s | 19.7 GB/s | 19.7 GB/s | 19.3 GB/s | 19.3 GB/s | 19.3 GB/s | 4.8 GB/s | 10.8 GB/s | 6.2 GB/s | 9.1 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 512 | 20.3 GB/s | 20.2 GB/s | 20.1 GB/s | 19.9 GB/s | 19.7 GB/s | 19.8 GB/s | 5.3 GB/s | 11.0 GB/s | 7.0 GB/s | 9.2 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 1024 | 19.7 GB/s | 19.6 GB/s | 19.7 GB/s | 19.4 GB/s | 19.1 GB/s | 19.3 GB/s | 4.7 GB/s | 9.9 GB/s | 6.1 GB/s | 9.1 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 2048 | 14.0 GB/s | 13.7 GB/s | 13.8 GB/s | 14.1 GB/s | 13.6 GB/s | 13.9 GB/s | 3.8 GB/s | 7.0 GB/s | 4.5 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 256 KiB | 1 | 7.1 GB/s | 5.9 GB/s | 6.9 GB/s | 3.7 GB/s | 6.3 GB/s | 3.7 GB/s | 2.8 GB/s | 3.8 GB/s | 2.8 GB/s | 1.8 GB/s | 0 / 0 / 0 / 5 (599 KB) / 12 / 5 (600 KB) / 97 (719 KB) / 72 (3 KB) / 24 (718 KB) / 2 |
| 256 KiB | 32 | 41.3 GB/s | 35.3 GB/s | 41.5 GB/s | 14.1 GB/s | 36.4 GB/s | 13.9 GB/s | 12.3 GB/s | 25.5 GB/s | 12.8 GB/s | 12.9 GB/s | 0 / 0 / 0 / 5 (536 KB) / 12 / 5 (536 KB) / 96 (630 KB) / 72 (2 KB) / 23 (625 KB) / 2 |
| 256 KiB | 128 | 42.5 GB/s | 34.9 GB/s | 42.4 GB/s | 10.0 GB/s | 36.3 GB/s | 10.1 GB/s | 9.8 GB/s | 24.8 GB/s | 9.8 GB/s | 12.6 GB/s | 0 / 0 / 0 / 5 (533 KB) / 12 / 5 (534 KB) / 96 (624 KB) / 72 (3 KB) / 23 (621 KB) / 2 |
| 256 KiB | 512 | 11.9 GB/s | 10.8 GB/s | 12.0 GB/s | 6.0 GB/s | 11.7 GB/s | 6.2 GB/s | 5.5 GB/s | 10.5 GB/s | 5.6 GB/s | 8.2 GB/s | 0 / 0 / 0 / 5 (536 KB) / 12 / 5 (536 KB) / 96 (624 KB) / 72 (2 KB) / 23 (622 KB) / 2 |
| 256 KiB | 1024 | 8.5 GB/s | 8.4 GB/s | 8.5 GB/s | 5.1 GB/s | 8.5 GB/s | 5.1 GB/s | 4.9 GB/s | 7.9 GB/s | 5.0 GB/s | 6.9 GB/s | 0 / 0 / 0 / 5 (542 KB) / 12 / 5 (541 KB) / 96 (630 KB) / 72 (2 KB) / 23 (627 KB) / 2 |
| 256 KiB | 2048 | 8.3 GB/s | 8.2 GB/s | 8.3 GB/s | 5.0 GB/s | 8.3 GB/s | 5.0 GB/s | 4.8 GB/s | 7.7 GB/s | 4.8 GB/s | 6.5 GB/s | 0 / 0 / 0 / 5 (551 KB) / 12 / 5 (552 KB) / 96 (642 KB) / 72 (2 KB) / 23 (639 KB) / 2 |
| 2 MiB | 1 | 9.4 GB/s | 5.7 GB/s | 9.4 GB/s | 4.4 GB/s | 7.2 GB/s | 4.3 GB/s | 4.0 GB/s | 4.1 GB/s | 3.7 GB/s | 1.7 GB/s | 5 (2 KB) / 4 (3 KB) / 5 / 11 (4873 KB) / 58 (9 KB) / 11 (4885 KB) / 129 (5674 KB) / 524 (46 KB) / 36 (5667 KB) / 6 (26 KB) |
| 2 MiB | 32 | 14.6 GB/s | 12.2 GB/s | 14.6 GB/s | 5.0 GB/s | 12.4 GB/s | 4.9 GB/s | 5.2 GB/s | 11.4 GB/s | 5.0 GB/s | 8.6 GB/s | 5 (1 KB) / 4 (1 KB) / 5 / 9 (4309 KB) / 58 (3 KB) / 10 (4303 KB) / 127 (4852 KB) / 524 (20 KB) / 34 (4857 KB) / 6 (2 KB) |
| 2 MiB | 128 | 8.9 GB/s | 7.4 GB/s | 9.5 GB/s | 4.6 GB/s | 7.8 GB/s | 4.1 GB/s | 4.7 GB/s | 7.2 GB/s | 4.7 GB/s | 6.8 GB/s | 5 / 4 / 5 (1 KB) / 9 (4237 KB) / 58 (3 KB) / 9 (4240 KB) / 126 (4760 KB) / 524 (19 KB) / 33 (4741 KB) / 6 |
| 6 MiB | 1 | 9.7 GB/s | 6.5 GB/s | 9.8 GB/s | 4.7 GB/s | 7.6 GB/s | 4.8 GB/s | 4.2 GB/s | 4.7 GB/s | 4.0 GB/s | 1.8 GB/s | 5 (18 KB) / 6 (25 KB) / 5 (9 KB) / 12 (16175 KB) / 156 (43 KB) / 11 (15930 KB) / 144 (19249 KB) / 1550 (113 KB) / 40 (19388 KB) / 8 (48 KB) |
| 6 MiB | 32 | 6.0 GB/s | 6.4 GB/s | 5.4 GB/s | 4.0 GB/s | 6.2 GB/s | 3.7 GB/s | 3.9 GB/s | 6.4 GB/s | 3.9 GB/s | 5.8 GB/s | 5 (78 KB) / 6 (87 KB) / 5 (105 KB) / 9 (12989 KB) / 156 (121 KB) / 9 (12951 KB) / 142 (15307 KB) / 1550 (131 KB) / 37 (15556 KB) / 8 (93 KB) |
| 6 MiB | 128 | 4.9 GB/s | 5.3 GB/s | 4.9 GB/s | 3.4 GB/s | 5.2 GB/s | 3.4 GB/s | 3.6 GB/s | 5.2 GB/s | 3.6 GB/s | 5.0 GB/s | 5 (152 KB) / 6 (251 KB) / 5 (133 KB) / 9 (13459 KB) / 156 (390 KB) / 9 (13086 KB) / 141 (15635 KB) / 1550 (292 KB) / 36 (15550 KB) / 8 (263 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-shared | ews-stream | ews-events | gws | gws-stream | gws-events | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / ews-events / gws / gws-stream / gws-events / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 10 MB/s | 8 MB/s | 10 MB/s | 10 MB/s | 8 MB/s | 7 MB/s | 8 MB/s | 8 MB/s | 7 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 66 MB/s | 54 MB/s | 66 MB/s | 66 MB/s | 49 MB/s | 49 MB/s | 49 MB/s | 54 MB/s | 47 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 128 | 67 MB/s | 54 MB/s | 66 MB/s | 67 MB/s | 51 MB/s | 51 MB/s | 51 MB/s | 54 MB/s | 47 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 512 | 67 MB/s | 55 MB/s | 67 MB/s | 68 MB/s | 52 MB/s | 52 MB/s | 52 MB/s | 55 MB/s | 47 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 1024 | 63 MB/s | 50 MB/s | 62 MB/s | 63 MB/s | 45 MB/s | 45 MB/s | 45 MB/s | 49 MB/s | 43 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 2048 | 56 MB/s | 48 MB/s | 56 MB/s | 56 MB/s | 46 MB/s | 46 MB/s | 46 MB/s | 44 MB/s | 40 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 157 MB/s | 125 MB/s | 156 MB/s | 158 MB/s | 120 MB/s | 118 MB/s | 121 MB/s | 116 MB/s | 120 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 1.0 GB/s | 839 MB/s | 1.0 GB/s | 1.0 GB/s | 773 MB/s | 774 MB/s | 767 MB/s | 793 MB/s | 807 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 1.0 GB/s | 845 MB/s | 1.0 GB/s | 1.0 GB/s | 794 MB/s | 796 MB/s | 784 MB/s | 769 MB/s | 812 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 970 MB/s | 784 MB/s | 962 MB/s | 967 MB/s | 755 MB/s | 755 MB/s | 748 MB/s | 730 MB/s | 743 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 723 MB/s | 603 MB/s | 717 MB/s | 725 MB/s | 548 MB/s | 551 MB/s | 546 MB/s | 554 MB/s | 574 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 543 MB/s | 475 MB/s | 525 MB/s | 540 MB/s | 395 MB/s | 392 MB/s | 394 MB/s | 410 MB/s | 425 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 1.3 GB/s | 1.2 GB/s | 1.3 GB/s | 1.3 GB/s | 1.2 GB/s | 1.1 GB/s | 1.2 GB/s | 779 MB/s | 1.1 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (41 KB) / 20 |
| 16 KiB | 32 | 8.8 GB/s | 8.0 GB/s | 8.6 GB/s | 8.8 GB/s | 7.8 GB/s | 7.7 GB/s | 7.8 GB/s | 5.6 GB/s | 7.8 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 8.2 GB/s | 8.0 GB/s | 8.0 GB/s | 8.2 GB/s | 7.8 GB/s | 7.7 GB/s | 7.7 GB/s | 4.6 GB/s | 7.2 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 3.6 GB/s | 4.5 GB/s | 3.6 GB/s | 3.7 GB/s | 5.2 GB/s | 5.2 GB/s | 5.2 GB/s | 3.1 GB/s | 4.0 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 1024 | 2.8 GB/s | 3.3 GB/s | 2.7 GB/s | 2.8 GB/s | 3.6 GB/s | 3.6 GB/s | 3.6 GB/s | 2.3 GB/s | 2.9 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.4 GB/s | 2.8 GB/s | 2.4 GB/s | 2.4 GB/s | 2.9 GB/s | 2.9 GB/s | 2.9 GB/s | 1.9 GB/s | 2.4 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (38 KB) / 20 |
| 256 KiB | 1 | 2.9 GB/s | 2.8 GB/s | 2.4 GB/s | 2.8 GB/s | 1.9 GB/s | 1.9 GB/s | 1.9 GB/s | 1.5 GB/s | 2.5 GB/s | 0 / 0 / 0 / 0 / 18 (1373 KB) / 22 (1133 KB) / 18 (1372 KB) / 40 (833 KB) / 56 (2 KB) |
| 256 KiB | 32 | 21.8 GB/s | 21.9 GB/s | 18.4 GB/s | 21.8 GB/s | 6.9 GB/s | 8.9 GB/s | 6.8 GB/s | 11.5 GB/s | 19.3 GB/s | 0 / 0 / 0 / 0 / 17 (1311 KB) / 21 (1076 KB) / 17 (1312 KB) / 32 (632 KB) / 56 (2 KB) |
| 256 KiB | 128 | 20.3 GB/s | 21.9 GB/s | 18.1 GB/s | 20.3 GB/s | 5.3 GB/s | 5.6 GB/s | 5.3 GB/s | 7.2 GB/s | 18.0 GB/s | 0 / 0 / 0 / 0 / 16 (1305 KB) / 20 (1069 KB) / 16 (1306 KB) / 32 (623 KB) / 56 (2 KB) |
| 256 KiB | 512 | 10.1 GB/s | 17.0 GB/s | 12.5 GB/s | 10.0 GB/s | 4.5 GB/s | 4.5 GB/s | 4.5 GB/s | 5.3 GB/s | 9.6 GB/s | 0 / 0 / 0 / 0 / 16 (1306 KB) / 20 (1072 KB) / 16 (1306 KB) / 32 (628 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.8 GB/s | 14.3 GB/s | 10.9 GB/s | 8.8 GB/s | 4.3 GB/s | 4.4 GB/s | 4.3 GB/s | 5.0 GB/s | 8.5 GB/s | 0 / 0 / 0 / 0 / 17 (1319 KB) / 20 (1084 KB) / 17 (1323 KB) / 32 (640 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.4 GB/s | 13.0 GB/s | 10.2 GB/s | 8.4 GB/s | 4.2 GB/s | 4.4 GB/s | 4.2 GB/s | 4.9 GB/s | 8.2 GB/s | 0 / 0 / 0 / 0 / 18 (1348 KB) / 21 (1111 KB) / 17 (1349 KB) / 33 (667 KB) / 56 (2 KB) |
| 2 MiB | 1 | 2.6 GB/s | 2.5 GB/s | 2.7 GB/s | 2.6 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.9 GB/s | 2.7 GB/s | 15 (11 KB) / 16 (6 KB) / 8 / 15 (5 KB) / 40 (12623 KB) / 45 (10188 KB) / 40 (12497 KB) / 72 (7431 KB) / 249 (17 KB) |
| 2 MiB | 32 | 10.7 GB/s | 9.3 GB/s | 20.7 GB/s | 10.6 GB/s | 4.0 GB/s | 4.3 GB/s | 4.0 GB/s | 6.9 GB/s | 19.3 GB/s | 15 (2 KB) / 16 (6 KB) / 8 (3 KB) / 15 (3 KB) / 34 (11654 KB) / 41 (9753 KB) / 34 (11597 KB) / 56 (5348 KB) / 249 (12 KB) |
| 2 MiB | 128 | 10.2 GB/s | 9.3 GB/s | 20.6 GB/s | 10.1 GB/s | 4.1 GB/s | 4.5 GB/s | 4.1 GB/s | 6.9 GB/s | 17.9 GB/s | 15 (2 KB) / 16 (4 KB) / 8 (1 KB) / 15 (2 KB) / 33 (11135 KB) / 38 (8984 KB) / 32 (11036 KB) / 53 (4977 KB) / 249 (12 KB) |
| 6 MiB | 1 | 3.3 GB/s | 3.1 GB/s | 2.7 GB/s | 3.4 GB/s | 2.0 GB/s | 2.1 GB/s | 2.1 GB/s | 2.2 GB/s | 3.1 GB/s | 13 (13 KB) / 14 (23 KB) / 8 / 13 (13 KB) / 45 (31499 KB) / 55 (23793 KB) / 44 (30358 KB) / 90 (20035 KB) / 644 (38 KB) |
| 6 MiB | 32 | 26.2 GB/s | 21.2 GB/s | 20.8 GB/s | 26.3 GB/s | 5.0 GB/s | 5.6 GB/s | 5.0 GB/s | 6.8 GB/s | 23.9 GB/s | 13 (8 KB) / 14 (21 KB) / 8 (7 KB) / 13 (7 KB) / 34 (25240 KB) / 42 (19788 KB) / 35 (25465 KB) / 74 (16940 KB) / 644 (32 KB) |
| 6 MiB | 128 | 26.1 GB/s | 20.9 GB/s | 20.7 GB/s | 26.2 GB/s | 5.1 GB/s | 5.9 GB/s | 5.2 GB/s | 7.2 GB/s | 23.9 GB/s | 13 (3 KB) / 14 (6 KB) / 8 (2 KB) / 13 / 33 (24853 KB) / 39 (18581 KB) / 33 (24940 KB) / 69 (15928 KB) / 644 (25 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-stream | ews-events | gws | gws-stream | gws-events | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / ews-events / gws / gws-stream / gws-events / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 11 MB/s | 11 MB/s | 11 MB/s | 11 MB/s | 11 MB/s | 11 MB/s | 9 MB/s | 7 MB/s | 10 MB/s | 11 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (2 KB) / 9 (1 KB) / 8 |
| 64 B | 32 | 76 MB/s | 75 MB/s | 76 MB/s | 76 MB/s | 74 MB/s | 76 MB/s | 61 MB/s | 52 MB/s | 71 MB/s | 72 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 128 | 77 MB/s | 76 MB/s | 76 MB/s | 77 MB/s | 75 MB/s | 76 MB/s | 61 MB/s | 52 MB/s | 71 MB/s | 72 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 512 | 77 MB/s | 77 MB/s | 77 MB/s | 77 MB/s | 75 MB/s | 76 MB/s | 61 MB/s | 52 MB/s | 71 MB/s | 73 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 1024 | 77 MB/s | 77 MB/s | 77 MB/s | 77 MB/s | 75 MB/s | 77 MB/s | 60 MB/s | 52 MB/s | 71 MB/s | 73 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 2048 | 78 MB/s | 77 MB/s | 77 MB/s | 78 MB/s | 75 MB/s | 77 MB/s | 59 MB/s | 51 MB/s | 70 MB/s | 74 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 1 KiB | 1 | 130 MB/s | 128 MB/s | 131 MB/s | 132 MB/s | 129 MB/s | 131 MB/s | 102 MB/s | 101 MB/s | 115 MB/s | 123 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (4 KB) / 20 (1 KB) / 12 (4 KB) / 8 |
| 1 KiB | 32 | 915 MB/s | 915 MB/s | 923 MB/s | 919 MB/s | 907 MB/s | 910 MB/s | 711 MB/s | 726 MB/s | 809 MB/s | 865 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 128 | 926 MB/s | 913 MB/s | 918 MB/s | 908 MB/s | 906 MB/s | 908 MB/s | 712 MB/s | 719 MB/s | 803 MB/s | 869 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 512 | 927 MB/s | 908 MB/s | 921 MB/s | 917 MB/s | 904 MB/s | 912 MB/s | 703 MB/s | 726 MB/s | 801 MB/s | 873 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 1024 | 936 MB/s | 919 MB/s | 930 MB/s | 916 MB/s | 910 MB/s | 915 MB/s | 696 MB/s | 721 MB/s | 799 MB/s | 873 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 2048 | 933 MB/s | 925 MB/s | 930 MB/s | 916 MB/s | 910 MB/s | 914 MB/s | 670 MB/s | 711 MB/s | 793 MB/s | 860 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 (1 KB) / 12 (2 KB) / 8 |
| 16 KiB | 1 | 1.1 GB/s | 1.0 GB/s | 1.1 GB/s | 1.1 GB/s | 1.0 GB/s | 1.1 GB/s | 694 MB/s | 911 MB/s | 755 MB/s | 1.0 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 26 (69 KB) / 20 (1 KB) / 22 (68 KB) / 8 |
| 16 KiB | 32 | 7.7 GB/s | 7.6 GB/s | 7.9 GB/s | 7.7 GB/s | 7.6 GB/s | 7.7 GB/s | 4.5 GB/s | 6.7 GB/s | 4.8 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (44 KB) / 20 / 21 (43 KB) / 8 |
| 16 KiB | 128 | 7.8 GB/s | 7.7 GB/s | 7.8 GB/s | 7.7 GB/s | 7.6 GB/s | 7.6 GB/s | 4.6 GB/s | 6.7 GB/s | 4.8 GB/s | 7.5 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (43 KB) / 20 / 21 (42 KB) / 8 |
| 16 KiB | 512 | 7.8 GB/s | 7.6 GB/s | 7.8 GB/s | 7.7 GB/s | 7.6 GB/s | 7.7 GB/s | 4.4 GB/s | 6.7 GB/s | 4.9 GB/s | 7.5 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (42 KB) / 20 / 21 (41 KB) / 8 |
| 16 KiB | 1024 | 7.9 GB/s | 7.7 GB/s | 7.9 GB/s | 7.7 GB/s | 7.7 GB/s | 7.7 GB/s | 4.0 GB/s | 6.7 GB/s | 4.6 GB/s | 7.5 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (45 KB) / 20 (1 KB) / 21 (41 KB) / 8 |
| 16 KiB | 2048 | 7.9 GB/s | 7.7 GB/s | 7.9 GB/s | 7.7 GB/s | 7.7 GB/s | 7.6 GB/s | 3.8 GB/s | 6.5 GB/s | 4.2 GB/s | 7.3 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (43 KB) / 20 / 21 (41 KB) / 8 |
| 256 KiB | 1 | 3.0 GB/s | 2.5 GB/s | 3.1 GB/s | 1.9 GB/s | 1.9 GB/s | 1.9 GB/s | 1.5 GB/s | 2.6 GB/s | 1.5 GB/s | 2.8 GB/s | 0 / 0 / 0 / 20 (1458 KB) / 24 (1211 KB) / 20 (1462 KB) / 48 (1244 KB) / 56 (4 KB) / 43 (1224 KB) / 8 |
| 256 KiB | 32 | 23.2 GB/s | 19.3 GB/s | 23.2 GB/s | 7.7 GB/s | 8.9 GB/s | 7.4 GB/s | 9.5 GB/s | 20.2 GB/s | 10.0 GB/s | 21.8 GB/s | 0 / 0 / 0 / 18 (1356 KB) / 22 (1127 KB) / 18 (1356 KB) / 36 (769 KB) / 56 (2 KB) / 32 (766 KB) / 8 |
| 256 KiB | 128 | 23.1 GB/s | 19.5 GB/s | 23.2 GB/s | 7.5 GB/s | 7.6 GB/s | 7.6 GB/s | 10.7 GB/s | 20.3 GB/s | 10.7 GB/s | 21.8 GB/s | 0 / 0 / 0 / 18 (1364 KB) / 22 (1138 KB) / 18 (1362 KB) / 35 (714 KB) / 56 (3 KB) / 31 (720 KB) / 8 |
| 256 KiB | 512 | 23.3 GB/s | 19.5 GB/s | 23.3 GB/s | 6.4 GB/s | 5.4 GB/s | 6.6 GB/s | 10.1 GB/s | 20.5 GB/s | 10.5 GB/s | 21.9 GB/s | 0 / 0 / 0 / 18 (1340 KB) / 21 (1129 KB) / 18 (1357 KB) / 34 (675 KB) / 56 (2 KB) / 30 (680 KB) / 8 |
| 256 KiB | 1024 | 23.3 GB/s | 19.6 GB/s | 23.2 GB/s | 5.7 GB/s | 5.2 GB/s | 5.7 GB/s | 9.0 GB/s | 20.5 GB/s | 9.4 GB/s | 21.7 GB/s | 0 / 0 / 0 / 17 (1336 KB) / 21 (1142 KB) / 18 (1340 KB) / 34 (684 KB) / 56 (2 KB) / 30 (692 KB) / 8 |
| 256 KiB | 2048 | 23.1 GB/s | 19.3 GB/s | 23.1 GB/s | 5.2 GB/s | 4.8 GB/s | 5.3 GB/s | 7.9 GB/s | 19.8 GB/s | 8.6 GB/s | 21.3 GB/s | 0 / 0 / 0 / 18 (1350 KB) / 24 (1238 KB) / 18 (1362 KB) / 34 (699 KB) / 56 (2 KB) / 30 (680 KB) / 8 |
| 2 MiB | 1 | 2.6 GB/s | 2.7 GB/s | 2.7 GB/s | 1.8 GB/s | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 2.8 GB/s | 1.9 GB/s | 3.0 GB/s | 15 (10 KB) / 8 (3 KB) / 15 (5 KB) / 45 (12578 KB) / 50 (10222 KB) / 46 (12594 KB) / 93 (8742 KB) / 249 (27 KB) / 81 (8614 KB) / 17 (13 KB) |
| 2 MiB | 32 | 10.9 GB/s | 20.9 GB/s | 10.9 GB/s | 4.3 GB/s | 4.8 GB/s | 4.3 GB/s | 7.3 GB/s | 19.7 GB/s | 7.3 GB/s | 20.4 GB/s | 15 (4 KB) / 8 (2 KB) / 15 (4 KB) / 38 (11734 KB) / 45 (9836 KB) / 38 (11690 KB) / 66 (6132 KB) / 249 (15 KB) / 57 (6239 KB) / 17 (7 KB) |
| 2 MiB | 128 | 11.3 GB/s | 21.1 GB/s | 11.4 GB/s | 4.4 GB/s | 4.9 GB/s | 4.4 GB/s | 7.5 GB/s | 19.8 GB/s | 7.5 GB/s | 20.6 GB/s | 15 (3 KB) / 8 (1 KB) / 15 (8 KB) / 34 (11225 KB) / 40 (9259 KB) / 35 (11253 KB) / 57 (5232 KB) / 249 (17 KB) / 49 (5300 KB) / 17 (3 KB) |
| 6 MiB | 1 | 3.4 GB/s | 2.7 GB/s | 3.4 GB/s | 2.2 GB/s | 2.2 GB/s | 2.2 GB/s | 2.2 GB/s | 3.2 GB/s | 2.3 GB/s | 3.4 GB/s | 13 (27 KB) / 8 (72 KB) / 13 / 55 (28252 KB) / 68 (22374 KB) / 57 (28750 KB) / 117 (21419 KB) / 644 (57 KB) / 96 (22099 KB) / 16 |
| 6 MiB | 32 | 26.4 GB/s | 21.2 GB/s | 25.9 GB/s | 5.6 GB/s | 6.5 GB/s | 5.6 GB/s | 6.8 GB/s | 24.9 GB/s | 6.8 GB/s | 26.2 GB/s | 13 (10 KB) / 8 (7 KB) / 13 (8 KB) / 38 (24631 KB) / 49 (19147 KB) / 38 (24656 KB) / 89 (17792 KB) / 644 (37 KB) / 67 (18126 KB) / 16 (9 KB) |
| 6 MiB | 128 | 26.5 GB/s | 21.2 GB/s | 26.5 GB/s | 5.8 GB/s | 6.8 GB/s | 5.8 GB/s | 7.2 GB/s | 24.9 GB/s | 7.3 GB/s | 26.3 GB/s | 13 (3 KB) / 8 (1 KB) / 13 (3 KB) / 35 (24421 KB) / 42 (18551 KB) / 35 (24386 KB) / 75 (16517 KB) / 644 (29 KB) / 54 (16342 KB) / 16 (3 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed with takeover, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- Without takeover ews, gws and the streaming paths tie from 64 bytes through 16 KiB; coder and gorilla's simple APIs trail on allocations. At 256 KiB ews leads the fastest streaming paths and is several times faster than the simple APIs. Comparing the compressed tables shows the cost of takeover: a 32 KB dictionary is primed per message and history is copied on both ends. Takeover buys compression ratio rather than raw speed on traffic that already repeats within each message, and its per-connection history becomes costly at high concurrency.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is several times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

