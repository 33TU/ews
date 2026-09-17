# Echo benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Echo -benchtime 1s | go run ../cmd/results` at ews commit `622145e`.

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
| 64 B | 32 | 107 MB/s | 107 MB/s | 108 MB/s | 103 MB/s | 79 MB/s | 80 MB/s | 102 MB/s | 105 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 128 | 118 MB/s | 117 MB/s | 119 MB/s | 113 MB/s | 84 MB/s | 85 MB/s | 109 MB/s | 115 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 512 | 121 MB/s | 118 MB/s | 120 MB/s | 115 MB/s | 88 MB/s | 86 MB/s | 113 MB/s | 118 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 1024 | 121 MB/s | 120 MB/s | 120 MB/s | 116 MB/s | 89 MB/s | 86 MB/s | 114 MB/s | 120 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 2048 | 121 MB/s | 119 MB/s | 120 MB/s | 114 MB/s | 88 MB/s | 84 MB/s | 113 MB/s | 118 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 1 KiB | 1 | 291 MB/s | 288 MB/s | 286 MB/s | 275 MB/s | 177 MB/s | 196 MB/s | 244 MB/s | 283 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 32 | 1.7 GB/s | 1.6 GB/s | 1.7 GB/s | 1.6 GB/s | 968 MB/s | 1.2 GB/s | 1.3 GB/s | 1.6 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 128 | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.7 GB/s | 1.0 GB/s | 1.3 GB/s | 1.4 GB/s | 1.8 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 512 | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 1024 | 1.9 GB/s | 1.9 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 2048 | 1.9 GB/s | 1.8 GB/s | 1.9 GB/s | 1.8 GB/s | 1.1 GB/s | 1.3 GB/s | 1.5 GB/s | 1.8 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 16 KiB | 1 | 3.0 GB/s | 3.0 GB/s | 2.9 GB/s | 2.9 GB/s | 989 MB/s | 1.6 GB/s | 1.4 GB/s | 1.3 GB/s | 0 / 0 / 1 / 6 / 61 (41 KB) / 16 / 16 (38 KB) / 2 |
| 16 KiB | 32 | 18.8 GB/s | 18.7 GB/s | 18.3 GB/s | 18.4 GB/s | 3.9 GB/s | 10.2 GB/s | 5.1 GB/s | 8.6 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 128 | 19.6 GB/s | 19.1 GB/s | 19.4 GB/s | 18.9 GB/s | 4.7 GB/s | 10.7 GB/s | 6.1 GB/s | 9.0 GB/s | 0 / 0 / 1 / 6 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 512 | 20.1 GB/s | 19.8 GB/s | 19.8 GB/s | 19.5 GB/s | 5.2 GB/s | 10.8 GB/s | 6.9 GB/s | 9.2 GB/s | 0 / 0 / 1 / 6 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 1024 | 19.4 GB/s | 19.4 GB/s | 19.2 GB/s | 18.9 GB/s | 4.7 GB/s | 9.9 GB/s | 6.2 GB/s | 9.0 GB/s | 0 / 0 / 1 / 6 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 2048 | 13.8 GB/s | 13.5 GB/s | 13.9 GB/s | 13.3 GB/s | 3.7 GB/s | 6.9 GB/s | 4.4 GB/s | 7.3 GB/s | 0 / 0 / 1 / 6 / 61 (38 KB) / 16 / 16 (37 KB) / 2 |
| 256 KiB | 1 | 6.9 GB/s | 5.8 GB/s | 3.6 GB/s | 6.2 GB/s | 2.6 GB/s | 3.8 GB/s | 2.6 GB/s | 1.8 GB/s | 0 / 0 / 6 (604 KB) / 12 / 97 (721 KB) / 72 (4 KB) / 24 (720 KB) / 2 |
| 256 KiB | 32 | 42.2 GB/s | 36.6 GB/s | 13.6 GB/s | 38.8 GB/s | 12.1 GB/s | 25.4 GB/s | 12.6 GB/s | 12.9 GB/s | 0 / 0 / 5 (535 KB) / 12 / 96 (629 KB) / 72 (3 KB) / 23 (625 KB) / 2 |
| 256 KiB | 128 | 41.0 GB/s | 34.9 GB/s | 9.7 GB/s | 37.3 GB/s | 9.7 GB/s | 24.3 GB/s | 9.7 GB/s | 12.5 GB/s | 0 / 0 / 5 (533 KB) / 12 / 96 (624 KB) / 72 (3 KB) / 23 (621 KB) / 2 |
| 256 KiB | 512 | 11.0 GB/s | 10.8 GB/s | 5.8 GB/s | 11.8 GB/s | 5.5 GB/s | 10.0 GB/s | 5.6 GB/s | 8.1 GB/s | 0 / 0 / 5 (535 KB) / 12 / 96 (626 KB) / 72 (2 KB) / 23 (622 KB) / 2 |
| 256 KiB | 1024 | 8.4 GB/s | 8.3 GB/s | 5.1 GB/s | 8.4 GB/s | 4.9 GB/s | 7.9 GB/s | 4.9 GB/s | 6.8 GB/s | 0 / 0 / 5 (540 KB) / 12 / 96 (630 KB) / 72 (2 KB) / 23 (628 KB) / 2 |
| 256 KiB | 2048 | 8.2 GB/s | 8.1 GB/s | 5.0 GB/s | 8.2 GB/s | 4.8 GB/s | 7.6 GB/s | 4.8 GB/s | 6.5 GB/s | 0 / 0 / 5 (553 KB) / 12 / 96 (642 KB) / 72 (2 KB) / 23 (640 KB) / 2 |

## Compressed with context takeover

| Size | Conns | ews | ews-shared | ews-stream | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 10 MB/s | 8 MB/s | 10 MB/s | 8 MB/s | 7 MB/s | 8 MB/s | 7 MB/s | 0 / 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 66 MB/s | 54 MB/s | 66 MB/s | 49 MB/s | 49 MB/s | 54 MB/s | 46 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 67 MB/s | 55 MB/s | 66 MB/s | 51 MB/s | 51 MB/s | 54 MB/s | 46 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 68 MB/s | 55 MB/s | 67 MB/s | 52 MB/s | 52 MB/s | 54 MB/s | 47 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 63 MB/s | 50 MB/s | 62 MB/s | 45 MB/s | 45 MB/s | 49 MB/s | 43 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 55 MB/s | 47 MB/s | 56 MB/s | 46 MB/s | 45 MB/s | 43 MB/s | 39 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 153 MB/s | 124 MB/s | 154 MB/s | 121 MB/s | 117 MB/s | 115 MB/s | 118 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 1.0 GB/s | 834 MB/s | 998 MB/s | 765 MB/s | 774 MB/s | 782 MB/s | 795 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 1.0 GB/s | 837 MB/s | 1.0 GB/s | 785 MB/s | 792 MB/s | 754 MB/s | 802 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 958 MB/s | 779 MB/s | 952 MB/s | 747 MB/s | 748 MB/s | 726 MB/s | 756 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 722 MB/s | 598 MB/s | 718 MB/s | 543 MB/s | 544 MB/s | 556 MB/s | 579 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 546 MB/s | 482 MB/s | 532 MB/s | 394 MB/s | 389 MB/s | 404 MB/s | 422 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 1.3 GB/s | 1.1 GB/s | 1.2 GB/s | 1.1 GB/s | 1.1 GB/s | 758 MB/s | 1.1 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (42 KB) / 20 |
| 16 KiB | 32 | 8.5 GB/s | 7.7 GB/s | 8.4 GB/s | 7.6 GB/s | 7.6 GB/s | 5.6 GB/s | 7.5 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 8.0 GB/s | 7.7 GB/s | 7.9 GB/s | 7.6 GB/s | 7.5 GB/s | 4.6 GB/s | 7.0 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 3.6 GB/s | 4.5 GB/s | 3.6 GB/s | 5.1 GB/s | 5.2 GB/s | 3.0 GB/s | 4.0 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 1024 | 2.8 GB/s | 3.3 GB/s | 2.8 GB/s | 3.6 GB/s | 3.6 GB/s | 2.3 GB/s | 2.9 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 2048 | 2.4 GB/s | 2.8 GB/s | 2.4 GB/s | 2.9 GB/s | 2.9 GB/s | 1.9 GB/s | 2.4 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (39 KB) / 20 |
| 256 KiB | 1 | 2.8 GB/s | 2.7 GB/s | 2.4 GB/s | 1.9 GB/s | 1.9 GB/s | 1.5 GB/s | 2.4 GB/s | 0 / 0 / 0 / 18 (1396 KB) / 22 (1161 KB) / 40 (946 KB) / 56 (3 KB) |
| 256 KiB | 32 | 20.8 GB/s | 20.9 GB/s | 17.9 GB/s | 6.8 GB/s | 8.4 GB/s | 10.9 GB/s | 18.4 GB/s | 0 / 0 / 0 / 16 (1323 KB) / 21 (1092 KB) / 32 (640 KB) / 56 (2 KB) |
| 256 KiB | 128 | 19.6 GB/s | 20.9 GB/s | 17.7 GB/s | 5.3 GB/s | 5.5 GB/s | 7.3 GB/s | 17.3 GB/s | 0 / 0 / 0 / 16 (1313 KB) / 20 (1077 KB) / 32 (626 KB) / 56 (2 KB) |
| 256 KiB | 512 | 9.8 GB/s | 16.0 GB/s | 12.4 GB/s | 4.4 GB/s | 4.4 GB/s | 5.2 GB/s | 9.5 GB/s | 0 / 0 / 0 / 16 (1320 KB) / 20 (1094 KB) / 32 (639 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.6 GB/s | 13.2 GB/s | 10.8 GB/s | 4.3 GB/s | 4.2 GB/s | 4.9 GB/s | 8.5 GB/s | 0 / 0 / 0 / 17 (1349 KB) / 21 (1132 KB) / 32 (666 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.4 GB/s | 12.6 GB/s | 10.0 GB/s | 4.0 GB/s | 4.2 GB/s | 4.7 GB/s | 8.1 GB/s | 0 / 0 / 0 / 18 (1415 KB) / 21 (1175 KB) / 33 (725 KB) / 56 (2 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-stream | gws | gws-stream | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / gws / gws-stream / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 11 MB/s | 11 MB/s | 11 MB/s | 11 MB/s | 9 MB/s | 7 MB/s | 10 MB/s | 11 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (2 KB) / 9 (1 KB) / 8 |
| 64 B | 32 | 77 MB/s | 75 MB/s | 76 MB/s | 74 MB/s | 60 MB/s | 51 MB/s | 69 MB/s | 72 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 128 | 77 MB/s | 76 MB/s | 76 MB/s | 75 MB/s | 60 MB/s | 50 MB/s | 70 MB/s | 72 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 512 | 77 MB/s | 76 MB/s | 77 MB/s | 75 MB/s | 60 MB/s | 51 MB/s | 71 MB/s | 72 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 1024 | 78 MB/s | 77 MB/s | 77 MB/s | 75 MB/s | 59 MB/s | 51 MB/s | 70 MB/s | 72 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 2048 | 78 MB/s | 76 MB/s | 77 MB/s | 75 MB/s | 58 MB/s | 50 MB/s | 70 MB/s | 73 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 1 KiB | 1 | 128 MB/s | 127 MB/s | 129 MB/s | 125 MB/s | 98 MB/s | 99 MB/s | 112 MB/s | 120 MB/s | 0 / 0 / 1 / 6 / 16 (5 KB) / 20 (1 KB) / 12 (4 KB) / 8 |
| 1 KiB | 32 | 891 MB/s | 881 MB/s | 903 MB/s | 893 MB/s | 691 MB/s | 698 MB/s | 786 MB/s | 840 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 128 | 902 MB/s | 892 MB/s | 899 MB/s | 887 MB/s | 690 MB/s | 702 MB/s | 779 MB/s | 852 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 512 | 901 MB/s | 895 MB/s | 900 MB/s | 893 MB/s | 683 MB/s | 704 MB/s | 786 MB/s | 847 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 / 12 (2 KB) / 8 |
| 1 KiB | 1024 | 909 MB/s | 903 MB/s | 906 MB/s | 897 MB/s | 672 MB/s | 700 MB/s | 782 MB/s | 854 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 (1 KB) / 12 (2 KB) / 8 |
| 1 KiB | 2048 | 910 MB/s | 903 MB/s | 906 MB/s | 898 MB/s | 662 MB/s | 697 MB/s | 764 MB/s | 862 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 (1 KB) / 12 (2 KB) / 8 |
| 16 KiB | 1 | 1.0 GB/s | 1.0 GB/s | 1.0 GB/s | 1.0 GB/s | 666 MB/s | 886 MB/s | 717 MB/s | 974 MB/s | 0 / 0 / 1 / 6 / 26 (72 KB) / 20 (1 KB) / 21 (70 KB) / 8 |
| 16 KiB | 32 | 7.6 GB/s | 7.5 GB/s | 7.5 GB/s | 7.4 GB/s | 4.2 GB/s | 6.5 GB/s | 4.6 GB/s | 7.2 GB/s | 0 / 0 / 1 / 6 / 25 (45 KB) / 20 / 21 (43 KB) / 8 |
| 16 KiB | 128 | 7.6 GB/s | 7.5 GB/s | 7.5 GB/s | 7.4 GB/s | 4.3 GB/s | 6.5 GB/s | 4.6 GB/s | 7.3 GB/s | 0 / 0 / 1 / 6 / 25 (43 KB) / 20 / 21 (43 KB) / 8 |
| 16 KiB | 512 | 7.6 GB/s | 7.5 GB/s | 7.5 GB/s | 7.4 GB/s | 4.3 GB/s | 6.5 GB/s | 4.6 GB/s | 7.3 GB/s | 0 / 0 / 1 / 6 / 25 (42 KB) / 20 (1 KB) / 21 (42 KB) / 8 |
| 16 KiB | 1024 | 7.6 GB/s | 7.5 GB/s | 7.5 GB/s | 7.5 GB/s | 4.0 GB/s | 6.5 GB/s | 4.5 GB/s | 7.3 GB/s | 0 / 0 / 1 / 6 / 25 (44 KB) / 20 (1 KB) / 21 (42 KB) / 8 |
| 16 KiB | 2048 | 7.7 GB/s | 7.5 GB/s | 7.5 GB/s | 7.5 GB/s | 3.7 GB/s | 6.3 GB/s | 4.1 GB/s | 7.1 GB/s | 0 / 0 / 1 / 6 / 25 (43 KB) / 20 / 21 (41 KB) / 8 |
| 256 KiB | 1 | 2.9 GB/s | 2.5 GB/s | 1.8 GB/s | 1.8 GB/s | 1.1 GB/s | 2.5 GB/s | 1.2 GB/s | 2.7 GB/s | 0 / 0 / 20 (1474 KB) / 23 (1216 KB) / 55 (1668 KB) / 56 (3 KB) / 49 (1585 KB) / 8 |
| 256 KiB | 32 | 22.2 GB/s | 18.9 GB/s | 7.2 GB/s | 8.4 GB/s | 8.5 GB/s | 19.5 GB/s | 8.8 GB/s | 21.0 GB/s | 0 / 0 / 18 (1381 KB) / 22 (1152 KB) / 37 (836 KB) / 56 (3 KB) / 33 (836 KB) / 8 |
| 256 KiB | 128 | 22.5 GB/s | 19.0 GB/s | 7.1 GB/s | 7.3 GB/s | 9.4 GB/s | 19.6 GB/s | 9.6 GB/s | 21.1 GB/s | 0 (1 KB) / 0 / 18 (1394 KB) / 22 (1163 KB) / 35 (754 KB) / 56 (4 KB) / 31 (766 KB) / 8 |
| 256 KiB | 512 | 22.5 GB/s | 19.2 GB/s | 6.4 GB/s | 5.2 GB/s | 9.5 GB/s | 19.9 GB/s | 9.9 GB/s | 21.1 GB/s | 0 / 0 / 18 (1377 KB) / 21 (1147 KB) / 34 (707 KB) / 56 (2 KB) / 30 (725 KB) / 8 |
| 256 KiB | 1024 | 22.5 GB/s | 19.1 GB/s | 5.4 GB/s | 5.0 GB/s | 8.4 GB/s | 19.7 GB/s | 8.9 GB/s | 21.0 GB/s | 0 / 0 / 18 (1378 KB) / 22 (1175 KB) / 34 (724 KB) / 56 (3 KB) / 30 (720 KB) / 8 |
| 256 KiB | 2048 | 22.4 GB/s | 19.0 GB/s | 4.8 GB/s | 4.5 GB/s | 7.3 GB/s | 19.3 GB/s | 8.2 GB/s | 20.6 GB/s | 0 / 0 / 18 (1439 KB) / 23 (1270 KB) / 34 (757 KB) / 56 (2 KB) / 29 (725 KB) / 8 |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed with takeover, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- Without takeover ews, gws and the streaming paths tie from 64 bytes through 16 KiB; coder and gorilla's simple APIs trail on allocations. At 256 KiB ews leads the fastest streaming paths and is several times faster than the simple APIs. Comparing the compressed tables shows the cost of takeover: a 32 KB dictionary is primed per message and history is copied on both ends. Takeover buys compression ratio rather than raw speed on traffic that already repeats within each message, and its per-connection history becomes costly at high concurrency.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is several times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

