# Echo benchmark results

Run at ews commit `9186b3e` with `go test -run '^$' -bench Echo -benchtime 1s`; tables and charts generated from the saved output by `go run ../cmd/results` on 2026-09-26.

![echo-sizes-compressed](echo-sizes-compressed.svg)

![echo-sizes-nocontext](echo-sizes-nocontext.svg)

![echo-sizes](echo-sizes.svg)

![echo-plain](echo-plain.svg)

![echo-compressed](echo-compressed.svg)

![echo-nocontext](echo-nocontext.svg)

## Setup

- CPU: AMD Ryzen 9 9950X3D 16-Core Processor
- Kernel: 7.2.2-1-cachyos
- Go: go1.27.1-X:nodwarf5, default build: SWAR masking and the shift-based UTF-8 validator
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
| 64 B | 1 | 19 MB/s | 19 MB/s | 19 MB/s | 19 MB/s | 18 MB/s | 19 MB/s | 14 MB/s | 13 MB/s | 18 MB/s | 18 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 32 | 107 MB/s | 105 MB/s | 107 MB/s | 106 MB/s | 104 MB/s | 107 MB/s | 79 MB/s | 80 MB/s | 101 MB/s | 105 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 128 | 118 MB/s | 117 MB/s | 117 MB/s | 117 MB/s | 111 MB/s | 116 MB/s | 84 MB/s | 85 MB/s | 107 MB/s | 116 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 512 | 119 MB/s | 118 MB/s | 118 MB/s | 120 MB/s | 114 MB/s | 118 MB/s | 88 MB/s | 86 MB/s | 112 MB/s | 117 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 1024 | 120 MB/s | 119 MB/s | 119 MB/s | 120 MB/s | 115 MB/s | 120 MB/s | 89 MB/s | 87 MB/s | 112 MB/s | 118 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 64 B | 2048 | 120 MB/s | 118 MB/s | 119 MB/s | 120 MB/s | 114 MB/s | 119 MB/s | 88 MB/s | 85 MB/s | 113 MB/s | 117 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 / 16 / 2 / 2 |
| 1 KiB | 1 | 290 MB/s | 286 MB/s | 288 MB/s | 284 MB/s | 274 MB/s | 284 MB/s | 178 MB/s | 197 MB/s | 246 MB/s | 279 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 32 | 1.7 GB/s | 1.6 GB/s | 1.6 GB/s | 1.6 GB/s | 1.6 GB/s | 1.6 GB/s | 963 MB/s | 1.2 GB/s | 1.3 GB/s | 1.6 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 128 | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.7 GB/s | 1.8 GB/s | 1.0 GB/s | 1.3 GB/s | 1.4 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 512 | 1.9 GB/s | 1.8 GB/s | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 1024 | 1.9 GB/s | 1.8 GB/s | 1.9 GB/s | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 2048 | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 16 KiB | 1 | 3.0 GB/s | 3.0 GB/s | 3.0 GB/s | 2.9 GB/s | 2.9 GB/s | 3.0 GB/s | 1.0 GB/s | 1.6 GB/s | 1.5 GB/s | 1.3 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (40 KB) / 16 / 16 (38 KB) / 2 |
| 16 KiB | 32 | 18.9 GB/s | 18.6 GB/s | 18.7 GB/s | 18.3 GB/s | 18.2 GB/s | 17.8 GB/s | 4.0 GB/s | 10.2 GB/s | 5.3 GB/s | 8.5 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 128 | 19.4 GB/s | 19.5 GB/s | 19.3 GB/s | 19.3 GB/s | 19.1 GB/s | 19.3 GB/s | 4.8 GB/s | 10.7 GB/s | 6.1 GB/s | 9.0 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 512 | 20.1 GB/s | 19.9 GB/s | 20.0 GB/s | 19.8 GB/s | 19.5 GB/s | 19.7 GB/s | 5.3 GB/s | 10.7 GB/s | 7.0 GB/s | 9.2 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 1024 | 19.5 GB/s | 19.3 GB/s | 19.4 GB/s | 19.3 GB/s | 18.8 GB/s | 19.2 GB/s | 4.8 GB/s | 9.9 GB/s | 6.1 GB/s | 9.0 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 2048 | 14.1 GB/s | 13.7 GB/s | 13.6 GB/s | 14.2 GB/s | 13.5 GB/s | 13.7 GB/s | 3.8 GB/s | 7.0 GB/s | 4.7 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 256 KiB | 1 | 6.8 GB/s | 5.9 GB/s | 6.7 GB/s | 3.6 GB/s | 6.3 GB/s | 3.6 GB/s | 2.6 GB/s | 4.0 GB/s | 2.6 GB/s | 1.8 GB/s | 0 / 0 / 0 / 5 (601 KB) / 12 / 5 (602 KB) / 97 (726 KB) / 72 (3 KB) / 24 (721 KB) / 2 |
| 256 KiB | 32 | 40.5 GB/s | 34.6 GB/s | 40.5 GB/s | 13.5 GB/s | 36.2 GB/s | 13.7 GB/s | 12.3 GB/s | 24.8 GB/s | 12.6 GB/s | 12.9 GB/s | 0 / 0 / 0 / 5 (536 KB) / 12 / 5 (536 KB) / 96 (630 KB) / 72 (2 KB) / 23 (624 KB) / 2 |
| 256 KiB | 128 | 41.3 GB/s | 34.6 GB/s | 41.4 GB/s | 9.9 GB/s | 36.7 GB/s | 10.0 GB/s | 10.0 GB/s | 24.9 GB/s | 9.8 GB/s | 12.7 GB/s | 0 / 0 / 0 / 5 (533 KB) / 12 / 5 (533 KB) / 96 (624 KB) / 72 (3 KB) / 23 (621 KB) / 2 |
| 256 KiB | 512 | 11.9 GB/s | 10.7 GB/s | 12.0 GB/s | 5.9 GB/s | 11.7 GB/s | 6.0 GB/s | 5.5 GB/s | 10.5 GB/s | 5.6 GB/s | 8.3 GB/s | 0 / 0 / 0 / 5 (535 KB) / 12 / 5 (537 KB) / 96 (625 KB) / 72 (2 KB) / 23 (623 KB) / 2 |
| 256 KiB | 1024 | 8.5 GB/s | 8.4 GB/s | 8.5 GB/s | 5.1 GB/s | 8.5 GB/s | 5.1 GB/s | 4.9 GB/s | 7.9 GB/s | 4.9 GB/s | 6.8 GB/s | 0 / 0 / 0 / 5 (540 KB) / 12 / 5 (541 KB) / 96 (631 KB) / 72 (2 KB) / 23 (627 KB) / 2 |
| 256 KiB | 2048 | 8.2 GB/s | 8.2 GB/s | 8.2 GB/s | 5.0 GB/s | 8.3 GB/s | 5.0 GB/s | 4.8 GB/s | 7.7 GB/s | 4.8 GB/s | 6.5 GB/s | 0 / 0 / 0 / 5 (552 KB) / 12 / 5 (551 KB) / 96 (642 KB) / 72 (2 KB) / 23 (639 KB) / 2 |
| 2 MiB | 1 | 9.2 GB/s | 5.8 GB/s | 9.2 GB/s | 4.6 GB/s | 8.0 GB/s | 4.6 GB/s | 4.3 GB/s | 4.5 GB/s | 4.1 GB/s | 1.7 GB/s | 5 (1 KB) / 4 (9 KB) / 5 (1 KB) / 11 (4864 KB) / 58 (4 KB) / 11 (4888 KB) / 129 (5678 KB) / 524 (42 KB) / 36 (5667 KB) / 6 (10 KB) |
| 2 MiB | 32 | 15.0 GB/s | 12.1 GB/s | 15.1 GB/s | 5.0 GB/s | 12.4 GB/s | 4.9 GB/s | 5.2 GB/s | 11.6 GB/s | 5.2 GB/s | 8.5 GB/s | 5 (1 KB) / 4 (3 KB) / 5 (1 KB) / 10 (4308 KB) / 58 (4 KB) / 10 (4316 KB) / 127 (4889 KB) / 524 (24 KB) / 34 (4864 KB) / 6 (1 KB) |
| 2 MiB | 128 | 9.5 GB/s | 7.4 GB/s | 9.1 GB/s | 4.2 GB/s | 7.8 GB/s | 4.5 GB/s | 4.8 GB/s | 7.2 GB/s | 4.7 GB/s | 6.8 GB/s | 5 (1 KB) / 4 / 5 (1 KB) / 9 (4245 KB) / 58 (1 KB) / 9 (4233 KB) / 126 (4744 KB) / 524 (20 KB) / 33 (4751 KB) / 6 |
| 6 MiB | 1 | 9.9 GB/s | 6.6 GB/s | 9.8 GB/s | 4.9 GB/s | 7.2 GB/s | 4.9 GB/s | 4.4 GB/s | 4.4 GB/s | 4.3 GB/s | 1.8 GB/s | 5 (9 KB) / 6 (22 KB) / 5 (8 KB) / 11 (15397 KB) / 156 (69 KB) / 11 (15807 KB) / 144 (19346 KB) / 1550 (184 KB) / 40 (19345 KB) / 8 (185 KB) |
| 6 MiB | 32 | 5.4 GB/s | 6.3 GB/s | 6.0 GB/s | 3.7 GB/s | 6.2 GB/s | 3.8 GB/s | 3.9 GB/s | 6.5 GB/s | 4.0 GB/s | 5.8 GB/s | 5 (103 KB) / 6 (89 KB) / 5 (71 KB) / 9 (12925 KB) / 156 (102 KB) / 9 (13001 KB) / 142 (15411 KB) / 1550 (144 KB) / 37 (15438 KB) / 8 (85 KB) |
| 6 MiB | 128 | 4.4 GB/s | 5.2 GB/s | 4.9 GB/s | 3.4 GB/s | 5.2 GB/s | 3.4 GB/s | 3.6 GB/s | 5.2 GB/s | 3.6 GB/s | 5.1 GB/s | 5 (234 KB) / 6 (298 KB) / 5 (138 KB) / 9 (12987 KB) / 156 (339 KB) / 9 (12966 KB) / 141 (15586 KB) / 1550 (311 KB) / 37 (15593 KB) / 8 (246 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-shared | ews-stream | ews-events | gws | gws-stream | gws-events | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / ews-events / gws / gws-stream / gws-events / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 10 MB/s | 8 MB/s | 10 MB/s | 10 MB/s | 8 MB/s | 7 MB/s | 8 MB/s | 8 MB/s | 7 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 66 MB/s | 54 MB/s | 66 MB/s | 66 MB/s | 49 MB/s | 49 MB/s | 49 MB/s | 54 MB/s | 47 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 128 | 67 MB/s | 54 MB/s | 66 MB/s | 66 MB/s | 51 MB/s | 51 MB/s | 51 MB/s | 54 MB/s | 47 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 512 | 67 MB/s | 55 MB/s | 67 MB/s | 67 MB/s | 52 MB/s | 52 MB/s | 51 MB/s | 55 MB/s | 48 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 1024 | 63 MB/s | 50 MB/s | 62 MB/s | 62 MB/s | 45 MB/s | 45 MB/s | 45 MB/s | 48 MB/s | 42 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 64 B | 2048 | 56 MB/s | 47 MB/s | 56 MB/s | 56 MB/s | 46 MB/s | 46 MB/s | 46 MB/s | 43 MB/s | 40 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 158 MB/s | 126 MB/s | 155 MB/s | 157 MB/s | 121 MB/s | 118 MB/s | 120 MB/s | 115 MB/s | 119 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 1.0 GB/s | 843 MB/s | 1.0 GB/s | 1.0 GB/s | 763 MB/s | 778 MB/s | 766 MB/s | 779 MB/s | 800 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 1.0 GB/s | 844 MB/s | 1.0 GB/s | 1.0 GB/s | 789 MB/s | 789 MB/s | 787 MB/s | 762 MB/s | 807 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 960 MB/s | 784 MB/s | 944 MB/s | 962 MB/s | 746 MB/s | 750 MB/s | 744 MB/s | 722 MB/s | 746 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 722 MB/s | 600 MB/s | 717 MB/s | 724 MB/s | 546 MB/s | 545 MB/s | 544 MB/s | 556 MB/s | 573 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 523 MB/s | 474 MB/s | 528 MB/s | 538 MB/s | 394 MB/s | 391 MB/s | 393 MB/s | 407 MB/s | 417 MB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 1.3 GB/s | 1.2 GB/s | 1.3 GB/s | 1.3 GB/s | 1.2 GB/s | 1.1 GB/s | 1.2 GB/s | 786 MB/s | 1.1 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (41 KB) / 20 |
| 16 KiB | 32 | 8.9 GB/s | 8.0 GB/s | 8.7 GB/s | 8.7 GB/s | 7.8 GB/s | 7.7 GB/s | 7.7 GB/s | 5.6 GB/s | 7.8 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 8.2 GB/s | 8.0 GB/s | 8.1 GB/s | 8.2 GB/s | 7.7 GB/s | 7.6 GB/s | 7.6 GB/s | 4.6 GB/s | 7.2 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 3.7 GB/s | 4.5 GB/s | 3.6 GB/s | 3.7 GB/s | 5.2 GB/s | 5.2 GB/s | 5.2 GB/s | 3.0 GB/s | 4.0 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 1024 | 2.8 GB/s | 3.3 GB/s | 2.8 GB/s | 2.8 GB/s | 3.6 GB/s | 3.6 GB/s | 3.6 GB/s | 2.3 GB/s | 2.9 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.4 GB/s | 2.9 GB/s | 2.4 GB/s | 2.4 GB/s | 2.9 GB/s | 2.9 GB/s | 2.9 GB/s | 1.9 GB/s | 2.4 GB/s | 0 / 0 / 0 / 0 / 1 / 6 / 1 / 25 (38 KB) / 20 |
| 256 KiB | 1 | 2.9 GB/s | 2.9 GB/s | 2.4 GB/s | 2.9 GB/s | 1.9 GB/s | 1.9 GB/s | 1.9 GB/s | 1.6 GB/s | 2.5 GB/s | 0 / 0 / 0 / 0 / 19 (1381 KB) / 22 (1140 KB) / 19 (1381 KB) / 40 (831 KB) / 56 (2 KB) |
| 256 KiB | 32 | 21.8 GB/s | 21.9 GB/s | 18.4 GB/s | 21.8 GB/s | 7.3 GB/s | 8.7 GB/s | 7.0 GB/s | 11.6 GB/s | 19.3 GB/s | 0 / 0 / 0 / 0 / 17 (1312 KB) / 21 (1076 KB) / 17 (1312 KB) / 32 (633 KB) / 56 (2 KB) |
| 256 KiB | 128 | 20.6 GB/s | 21.9 GB/s | 18.2 GB/s | 20.2 GB/s | 5.2 GB/s | 5.5 GB/s | 5.3 GB/s | 7.2 GB/s | 18.0 GB/s | 0 / 0 / 0 / 0 / 16 (1305 KB) / 20 (1071 KB) / 16 (1308 KB) / 32 (623 KB) / 56 (2 KB) |
| 256 KiB | 512 | 10.0 GB/s | 16.9 GB/s | 12.6 GB/s | 10.0 GB/s | 4.5 GB/s | 4.4 GB/s | 4.6 GB/s | 5.3 GB/s | 9.6 GB/s | 0 / 0 / 0 / 0 / 16 (1306 KB) / 20 (1078 KB) / 16 (1306 KB) / 32 (628 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.8 GB/s | 14.3 GB/s | 10.8 GB/s | 8.8 GB/s | 4.3 GB/s | 4.3 GB/s | 4.4 GB/s | 5.0 GB/s | 8.5 GB/s | 0 / 0 / 0 / 0 / 17 (1319 KB) / 21 (1101 KB) / 17 (1318 KB) / 32 (640 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.4 GB/s | 13.1 GB/s | 10.2 GB/s | 8.4 GB/s | 4.2 GB/s | 4.4 GB/s | 4.2 GB/s | 4.9 GB/s | 8.2 GB/s | 0 / 0 / 0 / 0 / 18 (1348 KB) / 21 (1098 KB) / 18 (1349 KB) / 33 (667 KB) / 56 (2 KB) |
| 2 MiB | 1 | 2.7 GB/s | 2.5 GB/s | 2.7 GB/s | 2.6 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.9 GB/s | 2.8 GB/s | 15 (6 KB) / 16 (4 KB) / 8 (12 KB) / 15 (3 KB) / 40 (12681 KB) / 44 (9999 KB) / 40 (12648 KB) / 72 (7651 KB) / 249 (21 KB) |
| 2 MiB | 32 | 10.7 GB/s | 9.3 GB/s | 20.6 GB/s | 10.5 GB/s | 3.9 GB/s | 4.4 GB/s | 4.0 GB/s | 6.8 GB/s | 19.1 GB/s | 15 (2 KB) / 16 (5 KB) / 8 (2 KB) / 15 (3 KB) / 35 (11683 KB) / 41 (9599 KB) / 34 (11525 KB) / 56 (5387 KB) / 249 (13 KB) |
| 2 MiB | 128 | 10.1 GB/s | 9.3 GB/s | 20.6 GB/s | 10.1 GB/s | 4.1 GB/s | 4.5 GB/s | 4.1 GB/s | 7.0 GB/s | 17.9 GB/s | 15 (2 KB) / 16 (8 KB) / 8 (1 KB) / 15 (2 KB) / 32 (11018 KB) / 38 (9022 KB) / 32 (11079 KB) / 53 (4947 KB) / 249 (13 KB) |
| 6 MiB | 1 | 3.4 GB/s | 3.1 GB/s | 2.7 GB/s | 3.4 GB/s | 2.0 GB/s | 2.1 GB/s | 2.0 GB/s | 2.2 GB/s | 3.2 GB/s | 13 (13 KB) / 14 (52 KB) / 8 (17 KB) / 13 (26 KB) / 45 (31621 KB) / 55 (24193 KB) / 44 (30705 KB) / 90 (19879 KB) / 644 (66 KB) |
| 6 MiB | 32 | 26.5 GB/s | 20.3 GB/s | 20.6 GB/s | 26.2 GB/s | 5.0 GB/s | 5.6 GB/s | 5.0 GB/s | 6.8 GB/s | 24.0 GB/s | 13 (7 KB) / 14 (21 KB) / 8 (5 KB) / 13 (8 KB) / 34 (25390 KB) / 43 (19730 KB) / 34 (25367 KB) / 74 (16871 KB) / 644 (32 KB) |
| 6 MiB | 128 | 26.3 GB/s | 19.8 GB/s | 20.8 GB/s | 26.2 GB/s | 5.2 GB/s | 5.9 GB/s | 5.2 GB/s | 7.2 GB/s | 23.8 GB/s | 13 (3 KB) / 14 (12 KB) / 8 (2 KB) / 13 (5 KB) / 33 (24897 KB) / 39 (18592 KB) / 33 (24890 KB) / 69 (15946 KB) / 644 (27 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-stream | ews-events | gws | gws-stream | gws-events | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / ews-events / gws / gws-stream / gws-events / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 11 MB/s | 11 MB/s | 11 MB/s | 11 MB/s | 11 MB/s | 11 MB/s | 9 MB/s | 7 MB/s | 10 MB/s | 11 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (2 KB) / 9 (1 KB) / 8 |
| 64 B | 32 | 76 MB/s | 75 MB/s | 77 MB/s | 76 MB/s | 75 MB/s | 76 MB/s | 61 MB/s | 52 MB/s | 70 MB/s | 73 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 128 | 77 MB/s | 75 MB/s | 77 MB/s | 76 MB/s | 75 MB/s | 77 MB/s | 61 MB/s | 52 MB/s | 71 MB/s | 73 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 512 | 77 MB/s | 76 MB/s | 77 MB/s | 77 MB/s | 74 MB/s | 76 MB/s | 61 MB/s | 52 MB/s | 71 MB/s | 73 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 1024 | 78 MB/s | 77 MB/s | 78 MB/s | 77 MB/s | 75 MB/s | 77 MB/s | 60 MB/s | 52 MB/s | 71 MB/s | 73 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 2048 | 78 MB/s | 77 MB/s | 77 MB/s | 77 MB/s | 75 MB/s | 77 MB/s | 59 MB/s | 51 MB/s | 70 MB/s | 73 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 1 KiB | 1 | 129 MB/s | 126 MB/s | 128 MB/s | 130 MB/s | 126 MB/s | 130 MB/s | 100 MB/s | 101 MB/s | 113 MB/s | 121 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (5 KB) / 20 (1 KB) / 12 (4 KB) / 8 |
| 1 KiB | 32 | 900 MB/s | 904 MB/s | 908 MB/s | 912 MB/s | 898 MB/s | 904 MB/s | 702 MB/s | 720 MB/s | 798 MB/s | 861 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 128 | 908 MB/s | 894 MB/s | 909 MB/s | 906 MB/s | 895 MB/s | 903 MB/s | 704 MB/s | 720 MB/s | 798 MB/s | 864 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 512 | 917 MB/s | 904 MB/s | 918 MB/s | 909 MB/s | 893 MB/s | 904 MB/s | 692 MB/s | 722 MB/s | 798 MB/s | 865 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 1024 | 921 MB/s | 912 MB/s | 920 MB/s | 910 MB/s | 898 MB/s | 909 MB/s | 692 MB/s | 720 MB/s | 793 MB/s | 870 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 2048 | 926 MB/s | 906 MB/s | 916 MB/s | 909 MB/s | 895 MB/s | 910 MB/s | 669 MB/s | 712 MB/s | 781 MB/s | 873 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 16 (3 KB) / 20 (1 KB) / 12 (2 KB) / 8 |
| 16 KiB | 1 | 1.1 GB/s | 1.0 GB/s | 1.1 GB/s | 1.1 GB/s | 1.0 GB/s | 1.1 GB/s | 679 MB/s | 893 MB/s | 739 MB/s | 995 MB/s | 0 / 0 / 0 / 1 / 6 / 1 / 26 (69 KB) / 20 (1 KB) / 21 (66 KB) / 8 |
| 16 KiB | 32 | 7.8 GB/s | 7.6 GB/s | 7.8 GB/s | 7.6 GB/s | 7.6 GB/s | 7.7 GB/s | 4.4 GB/s | 6.7 GB/s | 4.7 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (44 KB) / 20 / 21 (44 KB) / 8 |
| 16 KiB | 128 | 7.8 GB/s | 7.6 GB/s | 7.8 GB/s | 7.6 GB/s | 7.6 GB/s | 7.7 GB/s | 4.4 GB/s | 6.6 GB/s | 4.7 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (43 KB) / 20 (1 KB) / 21 (42 KB) / 8 |
| 16 KiB | 512 | 7.8 GB/s | 7.6 GB/s | 7.8 GB/s | 7.6 GB/s | 7.6 GB/s | 7.6 GB/s | 4.3 GB/s | 6.6 GB/s | 4.8 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (43 KB) / 20 / 21 (41 KB) / 8 |
| 16 KiB | 1024 | 7.9 GB/s | 7.7 GB/s | 7.8 GB/s | 7.6 GB/s | 7.6 GB/s | 7.7 GB/s | 4.0 GB/s | 6.5 GB/s | 4.6 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (43 KB) / 20 (1 KB) / 21 (41 KB) / 8 |
| 16 KiB | 2048 | 7.9 GB/s | 7.7 GB/s | 7.8 GB/s | 7.6 GB/s | 7.6 GB/s | 7.7 GB/s | 3.8 GB/s | 6.4 GB/s | 4.1 GB/s | 7.2 GB/s | 0 / 0 / 0 / 1 / 6 / 1 / 25 (42 KB) / 20 / 21 (41 KB) / 8 |
| 256 KiB | 1 | 3.0 GB/s | 2.5 GB/s | 3.0 GB/s | 1.7 GB/s | 1.8 GB/s | 1.8 GB/s | 1.4 GB/s | 2.6 GB/s | 1.4 GB/s | 2.8 GB/s | 0 / 0 / 0 / 20 (1452 KB) / 23 (1200 KB) / 20 (1450 KB) / 49 (1283 KB) / 56 (4 KB) / 44 (1238 KB) / 8 |
| 256 KiB | 32 | 23.5 GB/s | 19.3 GB/s | 23.4 GB/s | 7.4 GB/s | 8.8 GB/s | 7.5 GB/s | 9.5 GB/s | 20.4 GB/s | 9.7 GB/s | 21.6 GB/s | 0 / 0 / 0 / 18 (1357 KB) / 22 (1129 KB) / 18 (1361 KB) / 36 (765 KB) / 56 (3 KB) / 32 (764 KB) / 8 |
| 256 KiB | 128 | 23.1 GB/s | 19.4 GB/s | 22.9 GB/s | 7.6 GB/s | 7.6 GB/s | 7.5 GB/s | 10.7 GB/s | 20.3 GB/s | 10.6 GB/s | 21.8 GB/s | 0 / 0 / 0 / 18 (1362 KB) / 22 (1135 KB) / 18 (1359 KB) / 35 (714 KB) / 56 (3 KB) / 31 (720 KB) / 8 |
| 256 KiB | 512 | 23.4 GB/s | 19.5 GB/s | 23.2 GB/s | 6.3 GB/s | 5.4 GB/s | 6.4 GB/s | 10.0 GB/s | 20.5 GB/s | 10.5 GB/s | 21.9 GB/s | 0 / 0 / 0 / 18 (1345 KB) / 21 (1123 KB) / 18 (1360 KB) / 34 (680 KB) / 56 (2 KB) / 30 (684 KB) / 8 |
| 256 KiB | 1024 | 23.1 GB/s | 19.4 GB/s | 23.1 GB/s | 5.7 GB/s | 5.1 GB/s | 5.8 GB/s | 9.0 GB/s | 20.2 GB/s | 9.5 GB/s | 21.7 GB/s | 0 / 0 / 0 / 17 (1335 KB) / 22 (1144 KB) / 18 (1342 KB) / 34 (682 KB) / 56 (2 KB) / 29 (677 KB) / 8 |
| 256 KiB | 2048 | 23.1 GB/s | 19.2 GB/s | 23.1 GB/s | 5.2 GB/s | 4.9 GB/s | 5.3 GB/s | 8.1 GB/s | 20.0 GB/s | 8.8 GB/s | 21.4 GB/s | 0 / 0 / 0 / 18 (1355 KB) / 23 (1218 KB) / 18 (1361 KB) / 34 (696 KB) / 56 (2 KB) / 29 (678 KB) / 8 |
| 2 MiB | 1 | 2.7 GB/s | 2.7 GB/s | 2.7 GB/s | 1.8 GB/s | 1.9 GB/s | 1.9 GB/s | 1.8 GB/s | 2.8 GB/s | 1.9 GB/s | 3.0 GB/s | 15 (9 KB) / 8 (3 KB) / 15 (6 KB) / 46 (12768 KB) / 50 (10150 KB) / 45 (12556 KB) / 97 (9149 KB) / 249 (53 KB) / 82 (8754 KB) / 17 (14 KB) |
| 2 MiB | 32 | 10.8 GB/s | 21.0 GB/s | 10.9 GB/s | 4.3 GB/s | 4.8 GB/s | 4.3 GB/s | 7.4 GB/s | 19.7 GB/s | 7.3 GB/s | 20.3 GB/s | 15 (6 KB) / 8 (3 KB) / 15 (4 KB) / 38 (11764 KB) / 46 (9909 KB) / 38 (11697 KB) / 66 (6147 KB) / 249 (15 KB) / 58 (6259 KB) / 17 (5 KB) |
| 2 MiB | 128 | 11.3 GB/s | 21.1 GB/s | 11.4 GB/s | 4.4 GB/s | 4.8 GB/s | 4.4 GB/s | 7.5 GB/s | 19.9 GB/s | 7.5 GB/s | 20.5 GB/s | 15 (7 KB) / 8 (1 KB) / 15 (3 KB) / 35 (11275 KB) / 41 (9277 KB) / 35 (11363 KB) / 57 (5285 KB) / 249 (13 KB) / 49 (5353 KB) / 17 (3 KB) |
| 6 MiB | 1 | 3.4 GB/s | 2.7 GB/s | 3.4 GB/s | 2.1 GB/s | 2.1 GB/s | 2.1 GB/s | 2.3 GB/s | 3.2 GB/s | 2.3 GB/s | 3.4 GB/s | 13 (15 KB) / 8 (16 KB) / 13 (29 KB) / 58 (28947 KB) / 68 (22414 KB) / 57 (28745 KB) / 117 (21413 KB) / 644 (58 KB) / 97 (22314 KB) / 16 (33 KB) |
| 6 MiB | 32 | 26.5 GB/s | 21.1 GB/s | 26.2 GB/s | 5.6 GB/s | 6.5 GB/s | 5.6 GB/s | 6.8 GB/s | 24.7 GB/s | 6.8 GB/s | 26.1 GB/s | 13 (9 KB) / 8 (11 KB) / 13 (9 KB) / 38 (24832 KB) / 49 (19237 KB) / 38 (24959 KB) / 87 (17784 KB) / 644 (40 KB) / 67 (18027 KB) / 16 (6 KB) |
| 6 MiB | 128 | 26.5 GB/s | 21.1 GB/s | 26.4 GB/s | 5.7 GB/s | 6.7 GB/s | 5.9 GB/s | 7.3 GB/s | 25.0 GB/s | 7.3 GB/s | 26.0 GB/s | 13 (4 KB) / 8 (6 KB) / 13 (1 KB) / 35 (24521 KB) / 43 (18337 KB) / 35 (24503 KB) / 75 (16343 KB) / 644 (25 KB) / 54 (16406 KB) / 16 (7 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed with takeover, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- Without takeover ews, gws and the streaming paths tie from 64 bytes through 16 KiB; coder and gorilla's simple APIs trail on allocations. At 256 KiB ews leads the fastest streaming paths and is several times faster than the simple APIs. Comparing the compressed tables shows the cost of takeover: a 32 KB dictionary is primed per message and history is copied on both ends. Takeover buys compression ratio rather than raw speed on traffic that already repeats within each message, and its per-connection history becomes costly at high concurrency.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is several times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

