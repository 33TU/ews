# Echo benchmark results

Generated 2026-09-16 from `go test -run '^$' -bench Echo -benchtime 1s | go run ../cmd/results` at ews commit `fa0e5e3`.

![echo-plain](echo-plain.svg)

![echo-compressed](echo-compressed.svg)

![echo-nocontext](echo-nocontext.svg)

## Setup

- CPU: AMD Ryzen 9 9950X3D 16-Core Processor
- Kernel: 7.2.2-1-cachyos
- Go: go1.27.1-X:nodwarf5, default build: SWAR masking and the shift-based UTF-8 validator
- GOMAXPROCS: 32
- gws: v1.10.2
- coder/websocket: v1.8.15
- gorilla/websocket: v1.5.3

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer. With compression it keeps a compressor attached per connection.
- `ews-shared`: the same with `CompressionShared`, borrowing a pooled compressor per message as gws and coder do. Takeover table only; it is identical to `ews` otherwise.
- `ews-stream`: `NextMessage` then `WriteFrom` reading the connection itself. Plain messages are not held whole; compressed input is currently inflated whole before being delivered in chunks. This is ews's streaming shape, against `gws-stream` and `coder-stream`.
- `gws`: gws's `ReadMessage` and `WriteMessage` in a loop, the like-for-like shape against ews. Its event-driven `ReadLoop` shares the frame path and measured the same within noise.
- `gws-stream`: gws's `NextReader` piped into `WriteFile`, so no message is held whole.
- `coder`: coder/websocket with `Read` and `Write` in a loop.
- `coder-stream`: coder/websocket piping `Reader` into `Writer` through a reusable buffer, so no message is held whole.
- `gorilla`: gorilla/websocket with `ReadMessage` and `WriteMessage` in a loop. Uncompressed and no-takeover tables only, since gorilla negotiates only `no_context_takeover`.
- `gorilla-stream`: gorilla/websocket piping `NextReader` into `NextWriter` through a reusable buffer.

Compression is permessage-deflate at flate level 1, every message compressed, in two modes that are separate tables because they are different work. With context takeover each direction keeps a 32 KB history that every message extends, so the inflater is primed with a dictionary per message and both ends copy history; it compresses real traffic far better. Without takeover each message is compressed on its own. gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. gorilla/websocket uses the standard library's flate. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are dominated by loopback round-trip latency and vary more between runs than large-message and allocation figures. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | ews-stream | gws | gws-stream | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / gws / gws-stream / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 19 MB/s | 19 MB/s | 19 MB/s | 18 MB/s | 14 MB/s | 13 MB/s | 18 MB/s | 18 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 32 | 115 MB/s | 114 MB/s | 114 MB/s | 111 MB/s | 86 MB/s | 83 MB/s | 109 MB/s | 112 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 128 | 122 MB/s | 121 MB/s | 122 MB/s | 116 MB/s | 89 MB/s | 87 MB/s | 113 MB/s | 120 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 512 | 125 MB/s | 123 MB/s | 124 MB/s | 118 MB/s | 91 MB/s | 87 MB/s | 116 MB/s | 122 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 1024 | 126 MB/s | 124 MB/s | 125 MB/s | 119 MB/s | 85 MB/s | 87 MB/s | 117 MB/s | 123 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 2048 | 109 MB/s | 106 MB/s | 111 MB/s | 104 MB/s | 68 MB/s | 68 MB/s | 94 MB/s | 105 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 1 KiB | 1 | 294 MB/s | 291 MB/s | 287 MB/s | 278 MB/s | 179 MB/s | 198 MB/s | 247 MB/s | 285 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 32 | 1.8 GB/s | 1.7 GB/s | 1.8 GB/s | 1.7 GB/s | 1.0 GB/s | 1.3 GB/s | 1.4 GB/s | 1.7 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 128 | 1.9 GB/s | 1.8 GB/s | 1.9 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 512 | 1.9 GB/s | 1.9 GB/s | 1.9 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.9 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 1024 | 1.9 GB/s | 1.9 GB/s | 1.9 GB/s | 1.8 GB/s | 974 MB/s | 1.2 GB/s | 1.4 GB/s | 1.8 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 2048 | 1.4 GB/s | 1.4 GB/s | 1.4 GB/s | 1.3 GB/s | 733 MB/s | 896 MB/s | 1.0 GB/s | 1.3 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 16 KiB | 1 | 3.1 GB/s | 3.1 GB/s | 3.0 GB/s | 3.0 GB/s | 1.0 GB/s | 1.7 GB/s | 1.6 GB/s | 1.4 GB/s | 0 / 0 / 1 / 6 / 61 (41 KB) / 16 / 16 (38 KB) / 2 |
| 16 KiB | 32 | 18.7 GB/s | 18.9 GB/s | 18.6 GB/s | 18.6 GB/s | 4.1 GB/s | 10.4 GB/s | 5.0 GB/s | 8.8 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 128 | 20.1 GB/s | 20.0 GB/s | 19.8 GB/s | 19.5 GB/s | 5.1 GB/s | 10.9 GB/s | 6.6 GB/s | 9.2 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 512 | 16.9 GB/s | 16.8 GB/s | 16.8 GB/s | 16.3 GB/s | 3.9 GB/s | 7.8 GB/s | 4.9 GB/s | 8.2 GB/s | 0 / 0 / 1 / 6 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 1024 | 9.3 GB/s | 9.2 GB/s | 9.5 GB/s | 9.2 GB/s | 3.1 GB/s | 5.3 GB/s | 3.7 GB/s | 6.0 GB/s | 0 / 0 / 1 / 6 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 2048 | 6.3 GB/s | 6.2 GB/s | 6.2 GB/s | 6.2 GB/s | 2.8 GB/s | 4.1 GB/s | 3.2 GB/s | 4.4 GB/s | 0 / 0 / 1 / 6 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 256 KiB | 1 | 7.0 GB/s | 6.0 GB/s | 4.1 GB/s | 6.4 GB/s | 2.9 GB/s | 3.9 GB/s | 3.0 GB/s | 1.9 GB/s | 0 / 0 / 6 (603 KB) / 12 / 97 (724 KB) / 72 (4 KB) / 24 (719 KB) / 2 |
| 256 KiB | 32 | 42.0 GB/s | 36.3 GB/s | 7.1 GB/s | 37.5 GB/s | 7.3 GB/s | 25.2 GB/s | 7.2 GB/s | 12.2 GB/s | 0 / 0 / 5 (542 KB) / 12 / 96 (638 KB) / 72 (2 KB) / 23 (633 KB) / 2 |
| 256 KiB | 128 | 13.8 GB/s | 13.2 GB/s | 5.5 GB/s | 13.4 GB/s | 5.1 GB/s | 11.4 GB/s | 5.2 GB/s | 9.2 GB/s | 0 / 0 / 5 (536 KB) / 12 / 96 (628 KB) / 72 (4 KB) / 23 (625 KB) / 2 |
| 256 KiB | 512 | 8.6 GB/s | 8.4 GB/s | 5.1 GB/s | 8.7 GB/s | 4.8 GB/s | 7.8 GB/s | 4.8 GB/s | 6.8 GB/s | 0 / 0 / 5 (536 KB) / 12 / 96 (625 KB) / 72 (2 KB) / 23 (623 KB) / 2 |
| 256 KiB | 1024 | 8.0 GB/s | 8.1 GB/s | 4.8 GB/s | 8.1 GB/s | 4.6 GB/s | 7.5 GB/s | 4.7 GB/s | 6.5 GB/s | 0 / 0 / 5 (540 KB) / 12 / 96 (631 KB) / 72 (2 KB) / 23 (628 KB) / 2 |
| 256 KiB | 2048 | 7.9 GB/s | 7.8 GB/s | 4.8 GB/s | 7.9 GB/s | 4.6 GB/s | 7.3 GB/s | 4.6 GB/s | 6.3 GB/s | 0 / 0 / 5 (553 KB) / 12 / 96 (644 KB) / 72 (2 KB) / 23 (641 KB) / 2 |

## Compressed with context takeover

| Size | Conns | ews | ews-shared | ews-stream | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 10 MB/s | 8 MB/s | 10 MB/s | 8 MB/s | 8 MB/s | 8 MB/s | 7 MB/s | 0 / 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 67 MB/s | 55 MB/s | 67 MB/s | 50 MB/s | 51 MB/s | 55 MB/s | 47 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 68 MB/s | 55 MB/s | 67 MB/s | 53 MB/s | 53 MB/s | 56 MB/s | 47 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 52 MB/s | 40 MB/s | 52 MB/s | 32 MB/s | 32 MB/s | 38 MB/s | 35 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 42 MB/s | 35 MB/s | 43 MB/s | 29 MB/s | 29 MB/s | 32 MB/s | 30 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 41 MB/s | 36 MB/s | 41 MB/s | 37 MB/s | 36 MB/s | 33 MB/s | 30 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 161 MB/s | 128 MB/s | 159 MB/s | 125 MB/s | 122 MB/s | 117 MB/s | 122 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 1.0 GB/s | 824 MB/s | 1.0 GB/s | 774 MB/s | 762 MB/s | 773 MB/s | 804 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 1.0 GB/s | 832 MB/s | 1.0 GB/s | 780 MB/s | 789 MB/s | 769 MB/s | 802 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 601 MB/s | 516 MB/s | 584 MB/s | 413 MB/s | 416 MB/s | 451 MB/s | 459 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 480 MB/s | 417 MB/s | 461 MB/s | 329 MB/s | 328 MB/s | 351 MB/s | 360 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 406 MB/s | 375 MB/s | 404 MB/s | 298 MB/s | 294 MB/s | 301 MB/s | 313 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 1.3 GB/s | 1.2 GB/s | 1.3 GB/s | 1.2 GB/s | 1.2 GB/s | 801 MB/s | 1.1 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (42 KB) / 20 |
| 16 KiB | 32 | 8.5 GB/s | 7.9 GB/s | 8.4 GB/s | 7.8 GB/s | 7.9 GB/s | 4.7 GB/s | 7.4 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 4.2 GB/s | 5.3 GB/s | 4.2 GB/s | 5.5 GB/s | 5.9 GB/s | 3.3 GB/s | 4.6 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.4 GB/s | 2.9 GB/s | 2.4 GB/s | 3.0 GB/s | 3.0 GB/s | 2.1 GB/s | 2.5 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 1024 | 2.2 GB/s | 2.6 GB/s | 2.2 GB/s | 2.6 GB/s | 2.6 GB/s | 1.8 GB/s | 2.2 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 2048 | 2.1 GB/s | 2.5 GB/s | 2.1 GB/s | 2.4 GB/s | 2.5 GB/s | 1.7 GB/s | 2.1 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 2.8 GB/s | 2.8 GB/s | 2.4 GB/s | 1.9 GB/s | 1.9 GB/s | 1.5 GB/s | 2.5 GB/s | 0 / 0 / 0 / 18 (1392 KB) / 22 (1152 KB) / 40 (938 KB) / 56 (3 KB) |
| 256 KiB | 32 | 20.4 GB/s | 20.8 GB/s | 17.8 GB/s | 4.0 GB/s | 4.7 GB/s | 6.9 GB/s | 18.2 GB/s | 0 / 0 / 0 / 16 (1322 KB) / 21 (1088 KB) / 32 (639 KB) / 56 (2 KB) |
| 256 KiB | 128 | 10.8 GB/s | 17.2 GB/s | 13.3 GB/s | 4.1 GB/s | 4.2 GB/s | 5.4 GB/s | 10.2 GB/s | 0 / 0 / 0 / 16 (1311 KB) / 20 (1085 KB) / 32 (627 KB) / 56 (2 KB) |
| 256 KiB | 512 | 8.6 GB/s | 13.2 GB/s | 10.5 GB/s | 4.1 GB/s | 4.1 GB/s | 4.9 GB/s | 8.3 GB/s | 0 / 0 / 0 / 16 (1321 KB) / 20 (1094 KB) / 32 (641 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.3 GB/s | 12.6 GB/s | 10.1 GB/s | 3.9 GB/s | 4.0 GB/s | 4.8 GB/s | 8.0 GB/s | 0 / 0 / 0 / 17 (1351 KB) / 20 (1113 KB) / 32 (668 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.2 GB/s | 12.3 GB/s | 9.9 GB/s | 3.7 GB/s | 4.0 GB/s | 4.6 GB/s | 8.0 GB/s | 0 / 0 / 0 / 18 (1426 KB) / 21 (1179 KB) / 33 (728 KB) / 56 (2 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-stream | gws | gws-stream | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / gws / gws-stream / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 12 MB/s | 11 MB/s | 14 MB/s | 11 MB/s | 9 MB/s | 7 MB/s | 11 MB/s | 11 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (2 KB) / 9 (1 KB) / 8 |
| 64 B | 32 | 75 MB/s | 76 MB/s | 90 MB/s | 74 MB/s | 63 MB/s | 52 MB/s | 73 MB/s | 73 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 128 | 78 MB/s | 78 MB/s | 96 MB/s | 77 MB/s | 62 MB/s | 52 MB/s | 73 MB/s | 75 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 512 | 78 MB/s | 79 MB/s | 96 MB/s | 77 MB/s | 61 MB/s | 52 MB/s | 72 MB/s | 76 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 1024 | 79 MB/s | 78 MB/s | 98 MB/s | 77 MB/s | 58 MB/s | 49 MB/s | 71 MB/s | 73 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 2048 | 66 MB/s | 67 MB/s | 85 MB/s | 66 MB/s | 45 MB/s | 40 MB/s | 58 MB/s | 61 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 1 KiB | 1 | 131 MB/s | 129 MB/s | 131 MB/s | 128 MB/s | 102 MB/s | 103 MB/s | 116 MB/s | 125 MB/s | 0 / 0 / 1 / 6 / 16 (4 KB) / 20 (1 KB) / 12 (4 KB) / 8 |
| 1 KiB | 32 | 915 MB/s | 901 MB/s | 915 MB/s | 910 MB/s | 721 MB/s | 725 MB/s | 816 MB/s | 872 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 128 | 917 MB/s | 908 MB/s | 914 MB/s | 901 MB/s | 709 MB/s | 722 MB/s | 807 MB/s | 868 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 512 | 921 MB/s | 917 MB/s | 919 MB/s | 912 MB/s | 690 MB/s | 718 MB/s | 804 MB/s | 880 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 1024 | 920 MB/s | 912 MB/s | 912 MB/s | 909 MB/s | 641 MB/s | 675 MB/s | 757 MB/s | 853 MB/s | 0 / 0 / 1 / 6 / 16 (2 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 2048 | 815 MB/s | 799 MB/s | 796 MB/s | 774 MB/s | 547 MB/s | 575 MB/s | 649 MB/s | 740 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 16 KiB | 1 | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 696 MB/s | 931 MB/s | 763 MB/s | 1.0 GB/s | 0 / 0 / 1 / 6 / 25 (70 KB) / 20 (1 KB) / 21 (69 KB) / 8 |
| 16 KiB | 32 | 7.8 GB/s | 7.8 GB/s | 7.7 GB/s | 7.8 GB/s | 4.5 GB/s | 6.8 GB/s | 4.9 GB/s | 7.6 GB/s | 0 / 0 / 1 / 6 / 25 (47 KB) / 20 / 21 (46 KB) / 8 |
| 16 KiB | 128 | 7.9 GB/s | 7.8 GB/s | 7.7 GB/s | 7.7 GB/s | 4.5 GB/s | 6.7 GB/s | 4.9 GB/s | 7.5 GB/s | 0 / 0 / 1 / 6 / 25 (44 KB) / 20 (1 KB) / 21 (44 KB) / 8 |
| 16 KiB | 512 | 7.9 GB/s | 7.8 GB/s | 7.7 GB/s | 7.8 GB/s | 4.1 GB/s | 6.6 GB/s | 4.5 GB/s | 7.4 GB/s | 0 / 0 / 1 / 6 / 25 (41 KB) / 20 / 21 (41 KB) / 8 |
| 16 KiB | 1024 | 7.9 GB/s | 7.8 GB/s | 7.6 GB/s | 7.7 GB/s | 3.7 GB/s | 6.1 GB/s | 4.0 GB/s | 7.0 GB/s | 0 / 0 / 1 / 6 / 25 (40 KB) / 20 (1 KB) / 21 (40 KB) / 8 |
| 16 KiB | 2048 | 7.3 GB/s | 7.2 GB/s | 7.0 GB/s | 6.9 GB/s | 3.3 GB/s | 5.5 GB/s | 3.6 GB/s | 6.3 GB/s | 0 / 0 / 1 / 6 / 25 (39 KB) / 20 / 21 (39 KB) / 8 |
| 256 KiB | 1 | 3.1 GB/s | 2.6 GB/s | 1.9 GB/s | 1.9 GB/s | 1.4 GB/s | 2.6 GB/s | 1.4 GB/s | 2.9 GB/s | 0 / 0 / 20 (1487 KB) / 23 (1224 KB) / 53 (1548 KB) / 56 (5 KB) / 47 (1518 KB) / 8 |
| 256 KiB | 32 | 23.1 GB/s | 19.5 GB/s | 4.7 GB/s | 5.3 GB/s | 8.0 GB/s | 20.1 GB/s | 8.2 GB/s | 21.5 GB/s | 0 / 0 / 18 (1382 KB) / 22 (1150 KB) / 37 (820 KB) / 56 (3 KB) / 32 (824 KB) / 8 |
| 256 KiB | 128 | 23.3 GB/s | 19.7 GB/s | 4.7 GB/s | 5.1 GB/s | 7.8 GB/s | 20.3 GB/s | 7.9 GB/s | 21.7 GB/s | 0 / 0 / 18 (1379 KB) / 21 (1111 KB) / 35 (751 KB) / 56 (3 KB) / 31 (765 KB) / 8 |
| 256 KiB | 512 | 23.4 GB/s | 19.8 GB/s | 4.8 GB/s | 4.7 GB/s | 7.3 GB/s | 19.9 GB/s | 7.3 GB/s | 21.5 GB/s | 0 / 0 / 17 (1346 KB) / 21 (1151 KB) / 33 (702 KB) / 56 (2 KB) / 30 (721 KB) / 8 |
| 256 KiB | 1024 | 23.1 GB/s | 19.5 GB/s | 4.8 GB/s | 4.7 GB/s | 6.8 GB/s | 19.5 GB/s | 7.1 GB/s | 21.1 GB/s | 0 / 0 / 17 (1363 KB) / 22 (1186 KB) / 34 (726 KB) / 56 (2 KB) / 29 (706 KB) / 8 |
| 256 KiB | 2048 | 22.5 GB/s | 19.0 GB/s | 4.6 GB/s | 4.0 GB/s | 6.3 GB/s | 19.0 GB/s | 6.4 GB/s | 20.6 GB/s | 0 / 0 / 18 (1415 KB) / 25 (1377 KB) / 34 (738 KB) / 56 (2 KB) / 30 (754 KB) / 8 |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed with takeover, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- Without takeover gws leads the 64-byte cells, while ews, gws and the streaming paths converge from 1 KiB through 16 KiB; coder and gorilla's simple APIs trail on allocations. At 256 KiB ews leads the fastest streaming paths and is several times faster than the simple APIs. Comparing the compressed tables shows the cost of takeover: a 32 KB dictionary is primed per message and history is copied on both ends. Takeover buys compression ratio rather than raw speed on traffic that already repeats within each message, and its per-connection history becomes costly at high concurrency.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is several times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

