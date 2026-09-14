# Echo benchmark results

Generated 2026-09-15 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `39a6bc0`.

![echo-plain-simd](echo-plain-simd.svg)

![echo-compressed-simd](echo-compressed-simd.svg)

![echo-nocontext-simd](echo-nocontext-simd.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
- GOMAXPROCS: 20
- gws: v1.10.2
- coder/websocket: v1.8.15
- gorilla/websocket: v1.5.3

Echo servers behind `httptest` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- `ews`: `ws.Conn` with `ReadMessage` and `Write`, default 4 KiB read buffer. With compression it keeps a compressor attached per connection.
- `ews-shared`: the same with `CompressionShared`, borrowing a pooled compressor per message as gws and coder do. Takeover table only; it is identical to `ews` otherwise.
- `ews-stream`: `NextMessage` then `WriteFrom` reading the connection itself, so no message is held whole; ews's streaming shape, against `gws-stream` and `coder-stream`.
- `gws`: gws's `ReadMessage` and `WriteMessage` in a loop, the like-for-like shape against ews. Its event-driven `ReadLoop` shares the frame path and measured the same within noise.
- `gws-stream`: gws's `NextReader` piped into `WriteFile`, so no message is held whole.
- `coder`: coder/websocket with `Read` and `Write` in a loop.
- `coder-stream`: coder/websocket piping `Reader` into `Writer` through a reusable buffer, so no message is held whole.
- `gorilla`: gorilla/websocket with `ReadMessage` and `WriteMessage` in a loop. Uncompressed and no-takeover tables only, since gorilla negotiates only `no_context_takeover`.
- `gorilla-stream`: gorilla/websocket piping `NextReader` into `NextWriter` through a reusable buffer.

Compression is permessage-deflate at flate level 1, every message compressed, in two modes that are separate tables because they are different work. With context takeover each direction keeps a 32 KB history that every message extends, so the inflater is primed with a dictionary per message and both ends copy history; it compresses real traffic far better. Without takeover each message is compressed on its own. gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. gorilla/websocket uses the standard library's flate. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

## Uncompressed

| Size | Conns | ews | ews-stream | gws | gws-stream | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / gws / gws-stream / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 8 MB/s | 8 MB/s | 8 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 7 MB/s | 8 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 32 | 63 MB/s | 62 MB/s | 62 MB/s | 59 MB/s | 42 MB/s | 43 MB/s | 54 MB/s | 61 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 16 / 2 / 2 |
| 64 B | 128 | 66 MB/s | 68 MB/s | 69 MB/s | 67 MB/s | 46 MB/s | 49 MB/s | 58 MB/s | 67 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 16 / 2 / 2 |
| 64 B | 512 | 66 MB/s | 69 MB/s | 66 MB/s | 67 MB/s | 51 MB/s | 49 MB/s | 58 MB/s | 69 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 1024 | 67 MB/s | 61 MB/s | 67 MB/s | 63 MB/s | 49 MB/s | 47 MB/s | 56 MB/s | 63 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 2048 | 54 MB/s | 52 MB/s | 55 MB/s | 50 MB/s | 40 MB/s | 37 MB/s | 50 MB/s | 51 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 1 KiB | 1 | 126 MB/s | 132 MB/s | 134 MB/s | 117 MB/s | 50 MB/s | 86 MB/s | 66 MB/s | 124 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 32 | 981 MB/s | 957 MB/s | 949 MB/s | 933 MB/s | 555 MB/s | 704 MB/s | 699 MB/s | 932 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 128 | 1.0 GB/s | 1.0 GB/s | 1.1 GB/s | 992 MB/s | 587 MB/s | 757 MB/s | 773 MB/s | 972 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 512 | 1.0 GB/s | 984 MB/s | 1.0 GB/s | 983 MB/s | 611 MB/s | 748 MB/s | 807 MB/s | 1.0 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 1024 | 952 MB/s | 890 MB/s | 910 MB/s | 890 MB/s | 569 MB/s | 684 MB/s | 727 MB/s | 922 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 2048 | 732 MB/s | 647 MB/s | 744 MB/s | 656 MB/s | 489 MB/s | 557 MB/s | 599 MB/s | 694 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 16 KiB | 1 | 1.4 GB/s | 1.3 GB/s | 1.3 GB/s | 1.3 GB/s | 112 MB/s | 660 MB/s | 143 MB/s | 554 MB/s | 0 / 0 / 1 / 6 / 61 (41 KB) / 16 / 16 (39 KB) / 2 |
| 16 KiB | 32 | 9.7 GB/s | 9.8 GB/s | 9.4 GB/s | 9.6 GB/s | 1.8 GB/s | 5.5 GB/s | 1.3 GB/s | 4.6 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 128 | 10.0 GB/s | 10.4 GB/s | 9.9 GB/s | 9.5 GB/s | 2.3 GB/s | 5.5 GB/s | 2.3 GB/s | 4.6 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 512 | 7.5 GB/s | 7.5 GB/s | 7.3 GB/s | 7.4 GB/s | 2.8 GB/s | 4.4 GB/s | 3.1 GB/s | 4.3 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 1024 | 5.6 GB/s | 5.5 GB/s | 4.9 GB/s | 5.4 GB/s | 2.7 GB/s | 3.8 GB/s | 3.0 GB/s | 3.8 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 2048 | 4.4 GB/s | 5.0 GB/s | 5.0 GB/s | 4.8 GB/s | 2.4 GB/s | 3.5 GB/s | 2.7 GB/s | 3.6 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 256 KiB | 1 | 3.2 GB/s | 2.7 GB/s | 422 MB/s | 2.7 GB/s | 273 MB/s | 1.5 GB/s | 307 MB/s | 765 MB/s | 0 / 0 / 6 (595 KB) / 12 (1 KB) / 97 (708 KB) / 72 (5 KB) / 24 (702 KB) / 2 |
| 256 KiB | 32 | 16.6 GB/s | 15.1 GB/s | 3.1 GB/s | 14.2 GB/s | 3.4 GB/s | 11.6 GB/s | 3.1 GB/s | 6.7 GB/s | 0 / 0 / 5 (571 KB) / 12 / 96 (667 KB) / 72 (3 KB) / 23 (671 KB) / 2 |
| 256 KiB | 128 | 9.2 GB/s | 8.7 GB/s | 5.2 GB/s | 8.7 GB/s | 4.9 GB/s | 8.0 GB/s | 5.3 GB/s | 5.8 GB/s | 0 / 0 / 5 (536 KB) / 12 / 96 (630 KB) / 72 (3 KB) / 23 (626 KB) / 2 |
| 256 KiB | 512 | 7.6 GB/s | 7.3 GB/s | 5.0 GB/s | 7.4 GB/s | 4.2 GB/s | 6.9 GB/s | 4.8 GB/s | 5.5 GB/s | 0 / 0 / 5 (544 KB) / 12 / 96 (634 KB) / 72 (2 KB) / 23 (631 KB) / 2 |
| 256 KiB | 1024 | 7.2 GB/s | 6.9 GB/s | 5.1 GB/s | 7.0 GB/s | 4.5 GB/s | 6.7 GB/s | 5.2 GB/s | 5.4 GB/s | 0 / 0 / 5 (555 KB) / 12 / 96 (643 KB) / 72 (2 KB) / 23 (642 KB) / 2 |
| 256 KiB | 2048 | 7.0 GB/s | 6.9 GB/s | 5.1 GB/s | 6.8 GB/s | 4.4 GB/s | 6.4 GB/s | 4.5 GB/s | 5.5 GB/s | 0 / 0 / 5 (571 KB) / 12 / 96 (675 KB) / 72 (2 KB) / 23 (668 KB) / 2 |

## Compressed with context takeover

| Size | Conns | ews | ews-shared | ews-stream | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 0 / 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 42 MB/s | 35 MB/s | 39 MB/s | 28 MB/s | 30 MB/s | 33 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 41 MB/s | 34 MB/s | 42 MB/s | 30 MB/s | 31 MB/s | 36 MB/s | 31 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 35 MB/s | 28 MB/s | 35 MB/s | 26 MB/s | 25 MB/s | 29 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 36 MB/s | 32 MB/s | 35 MB/s | 31 MB/s | 33 MB/s | 28 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 34 MB/s | 33 MB/s | 33 MB/s | 32 MB/s | 32 MB/s | 28 MB/s | 26 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 78 MB/s | 64 MB/s | 76 MB/s | 62 MB/s | 63 MB/s | 48 MB/s | 58 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 593 MB/s | 500 MB/s | 587 MB/s | 427 MB/s | 441 MB/s | 462 MB/s | 468 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 577 MB/s | 508 MB/s | 608 MB/s | 436 MB/s | 441 MB/s | 465 MB/s | 487 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 385 MB/s | 328 MB/s | 384 MB/s | 252 MB/s | 249 MB/s | 296 MB/s | 302 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 329 MB/s | 278 MB/s | 328 MB/s | 220 MB/s | 223 MB/s | 256 MB/s | 260 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 309 MB/s | 278 MB/s | 307 MB/s | 222 MB/s | 221 MB/s | 233 MB/s | 241 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 686 MB/s | 617 MB/s | 681 MB/s | 620 MB/s | 563 MB/s | 221 MB/s | 568 MB/s | 0 / 0 / 0 / 1 / 6 / 25 (42 KB) / 20 |
| 16 KiB | 32 | 4.6 GB/s | 4.4 GB/s | 4.7 GB/s | 4.3 GB/s | 4.2 GB/s | 2.7 GB/s | 4.3 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 2.7 GB/s | 3.1 GB/s | 2.7 GB/s | 3.1 GB/s | 3.2 GB/s | 2.2 GB/s | 3.0 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.2 GB/s | 2.4 GB/s | 2.2 GB/s | 2.2 GB/s | 2.2 GB/s | 1.8 GB/s | 2.1 GB/s | 0 / 0 / 0 / 1 / 6 (1 KB) / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.1 GB/s | 2.3 GB/s | 2.0 GB/s | 2.1 GB/s | 2.1 GB/s | 1.7 GB/s | 2.0 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.0 GB/s | 2.2 GB/s | 2.0 GB/s | 2.0 GB/s | 2.0 GB/s | 1.4 GB/s | 1.7 GB/s | 0 / 0 / 0 / 1 / 6 (1 KB) / 25 (38 KB) / 20 |
| 256 KiB | 1 | 1.5 GB/s | 1.5 GB/s | 1.3 GB/s | 387 MB/s | 414 MB/s | 249 MB/s | 1.3 GB/s | 0 / 0 / 0 / 18 (1394 KB) / 22 (1175 KB) / 40 (968 KB) / 56 (4 KB) |
| 256 KiB | 32 | 11.5 GB/s | 12.0 GB/s | 10.8 GB/s | 3.0 GB/s | 3.5 GB/s | 4.8 GB/s | 10.7 GB/s | 0 / 0 (1 KB) / 0 (1 KB) / 17 (1379 KB) / 20 (1090 KB) / 33 (665 KB) / 56 (3 KB) |
| 256 KiB | 128 | 8.9 GB/s | 10.4 GB/s | 9.3 GB/s | 3.2 GB/s | 3.5 GB/s | 4.8 GB/s | 8.6 GB/s | 0 / 0 (4 KB) / 0 / 17 (1347 KB) / 21 (1099 KB) / 32 (644 KB) / 56 (3 KB) |
| 256 KiB | 512 | 8.4 GB/s | 10.1 GB/s | 7.5 GB/s | 3.3 GB/s | 3.2 GB/s | 4.6 GB/s | 7.8 GB/s | 0 / 0 / 0 / 17 (1374 KB) / 21 (1159 KB) / 33 (667 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.4 GB/s | 8.9 GB/s | 8.2 GB/s | 3.3 GB/s | 2.9 GB/s | 4.3 GB/s | 7.4 GB/s | 0 / 0 (7 KB) / 0 / 18 (1431 KB) / 22 (1243 KB) / 33 (711 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.8 GB/s | 10.4 GB/s | 8.2 GB/s | 3.5 GB/s | 2.9 GB/s | 3.0 GB/s | 7.9 GB/s | 0 / 0 / 0 / 19 (1478 KB) / 24 (1338 KB) / 32 (615 KB) / 56 (2 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-stream | gws | gws-stream | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / gws / gws-stream / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 6 MB/s | 6 MB/s | 7 MB/s | 5 MB/s | 5 MB/s | 4 MB/s | 5 MB/s | 6 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (2 KB) / 9 (1 KB) / 8 |
| 64 B | 32 | 49 MB/s | 49 MB/s | 57 MB/s | 47 MB/s | 40 MB/s | 34 MB/s | 46 MB/s | 47 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 128 | 51 MB/s | 50 MB/s | 61 MB/s | 51 MB/s | 42 MB/s | 36 MB/s | 48 MB/s | 49 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 (1 KB) / 8 |
| 64 B | 512 | 54 MB/s | 52 MB/s | 62 MB/s | 51 MB/s | 42 MB/s | 35 MB/s | 48 MB/s | 49 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (2 KB) / 9 (1 KB) / 8 |
| 64 B | 1024 | 52 MB/s | 50 MB/s | 58 MB/s | 48 MB/s | 38 MB/s | 33 MB/s | 44 MB/s | 46 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (2 KB) / 9 (1 KB) / 8 |
| 64 B | 2048 | 46 MB/s | 47 MB/s | 52 MB/s | 43 MB/s | 33 MB/s | 31 MB/s | 42 MB/s | 42 MB/s | 0 / 0 / 1 (1 KB) / 6 (1 KB) / 13 (2 KB) / 32 (1 KB) / 9 / 8 |
| 1 KiB | 1 | 68 MB/s | 67 MB/s | 66 MB/s | 65 MB/s | 49 MB/s | 52 MB/s | 53 MB/s | 64 MB/s | 0 / 0 / 1 / 6 / 16 (4 KB) / 20 (1 KB) / 12 (4 KB) / 8 |
| 1 KiB | 32 | 594 MB/s | 572 MB/s | 591 MB/s | 591 MB/s | 473 MB/s | 479 MB/s | 538 MB/s | 577 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 (1 KB) / 12 (3 KB) / 8 |
| 1 KiB | 128 | 624 MB/s | 597 MB/s | 590 MB/s | 610 MB/s | 474 MB/s | 481 MB/s | 525 MB/s | 587 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 (1 KB) / 12 (3 KB) / 8 |
| 1 KiB | 512 | 628 MB/s | 588 MB/s | 612 MB/s | 600 MB/s | 453 MB/s | 482 MB/s | 507 MB/s | 576 MB/s | 0 / 0 / 1 / 6 (1 KB) / 16 (4 KB) / 20 (1 KB) / 12 (4 KB) / 8 (1 KB) |
| 1 KiB | 1024 | 619 MB/s | 605 MB/s | 596 MB/s | 568 MB/s | 443 MB/s | 477 MB/s | 503 MB/s | 567 MB/s | 0 / 0 / 1 / 6 (2 KB) / 16 (4 KB) / 20 (1 KB) / 12 (3 KB) / 8 |
| 1 KiB | 2048 | 590 MB/s | 570 MB/s | 576 MB/s | 528 MB/s | 415 MB/s | 447 MB/s | 467 MB/s | 527 MB/s | 0 / 0 / 1 / 6 (3 KB) / 16 (4 KB) / 20 / 12 (4 KB) / 8 |
| 16 KiB | 1 | 571 MB/s | 574 MB/s | 565 MB/s | 569 MB/s | 173 MB/s | 491 MB/s | 175 MB/s | 544 MB/s | 0 / 0 / 1 / 6 / 25 (65 KB) / 20 (1 KB) / 21 (63 KB) / 8 |
| 16 KiB | 32 | 5.5 GB/s | 5.6 GB/s | 5.2 GB/s | 5.4 GB/s | 3.1 GB/s | 4.4 GB/s | 3.3 GB/s | 5.3 GB/s | 0 / 0 / 1 / 6 / 25 (46 KB) / 20 (1 KB) / 21 (46 KB) / 8 |
| 16 KiB | 128 | 5.3 GB/s | 5.5 GB/s | 5.4 GB/s | 5.4 GB/s | 2.9 GB/s | 4.5 GB/s | 3.1 GB/s | 5.2 GB/s | 0 / 0 / 1 / 6 / 25 (46 KB) / 20 (1 KB) / 21 (45 KB) / 8 |
| 16 KiB | 512 | 5.6 GB/s | 5.5 GB/s | 5.4 GB/s | 5.4 GB/s | 3.0 GB/s | 4.6 GB/s | 3.1 GB/s | 5.0 GB/s | 0 / 0 / 1 / 6 / 25 (46 KB) / 20 (1 KB) / 21 (45 KB) / 8 (1 KB) |
| 16 KiB | 1024 | 5.6 GB/s | 5.5 GB/s | 5.4 GB/s | 5.2 GB/s | 2.9 GB/s | 4.5 GB/s | 3.0 GB/s | 5.1 GB/s | 0 / 0 / 1 / 6 (2 KB) / 25 (47 KB) / 20 (1 KB) / 21 (44 KB) / 8 |
| 16 KiB | 2048 | 5.5 GB/s | 5.5 GB/s | 5.3 GB/s | 4.8 GB/s | 2.6 GB/s | 4.4 GB/s | 2.9 GB/s | 4.9 GB/s | 0 / 0 / 1 / 6 (5 KB) / 25 (51 KB) / 20 / 21 (46 KB) / 8 |
| 256 KiB | 1 | 1.6 GB/s | 1.3 GB/s | 352 MB/s | 409 MB/s | 242 MB/s | 1.4 GB/s | 260 MB/s | 1.4 GB/s | 0 (1 KB) / 0 / 20 (1476 KB) / 23 (1228 KB) / 46 (1248 KB) / 56 (5 KB) / 40 (1178 KB) / 8 (2 KB) |
| 256 KiB | 32 | 14.6 GB/s | 13.1 GB/s | 3.0 GB/s | 3.5 GB/s | 3.8 GB/s | 13.6 GB/s | 3.8 GB/s | 14.0 GB/s | 0 (1 KB) / 0 (1 KB) / 22 (1623 KB) / 25 (1290 KB) / 41 (1052 KB) / 56 (4 KB) / 37 (1074 KB) / 8 (3 KB) |
| 256 KiB | 128 | 14.9 GB/s | 13.2 GB/s | 3.3 GB/s | 3.8 GB/s | 4.9 GB/s | 13.2 GB/s | 5.0 GB/s | 14.4 GB/s | 0 (3 KB) / 0 (1 KB) / 21 (1558 KB) / 22 (1155 KB) / 36 (805 KB) / 56 (7 KB) / 32 (813 KB) / 8 (4 KB) |
| 256 KiB | 512 | 15.0 GB/s | 12.8 GB/s | 4.0 GB/s | 4.0 GB/s | 5.7 GB/s | 13.6 GB/s | 5.9 GB/s | 14.2 GB/s | 0 / 0 (14 KB) / 19 (1446 KB) / 23 (1213 KB) / 34 (728 KB) / 56 (3 KB) / 30 (736 KB) / 8 (1 KB) |
| 256 KiB | 1024 | 15.0 GB/s | 13.2 GB/s | 4.0 GB/s | 3.8 GB/s | 5.8 GB/s | 13.5 GB/s | 5.8 GB/s | 14.4 GB/s | 0 / 0 / 19 (1468 KB) / 24 (1306 KB) / 34 (755 KB) / 56 (3 KB) / 30 (739 KB) / 8 (1 KB) |
| 256 KiB | 2048 | 15.0 GB/s | 12.9 GB/s | 3.9 GB/s | 3.2 GB/s | 5.1 GB/s | 13.3 GB/s | 5.2 GB/s | 13.7 GB/s | 0 / 0 / 20 (1561 KB) / 30 (1695 KB) / 37 (882 KB) / 56 (3 KB) / 32 (891 KB) / 8 (1 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed with takeover, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- Without takeover the small-message cells tie across ews, gws and gorilla-stream, coder and gorilla's simple API trail on allocations, and at 256 KiB ews reaches 14 GB/s against 13 for gorilla-stream and coder-stream and 3 to 5 for the simple APIs. Comparing the two compressed tables gives each library's cost of takeover: a 32 KB dictionary primed per message and history copied on both ends. For ews that is 5 to 10 percent at 1 KiB and a third at 256 KiB, and the same or more for the others; takeover buys ratio, not speed, on traffic that repeats.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

