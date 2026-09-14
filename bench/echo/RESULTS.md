# Echo benchmark results

Generated 2026-09-15 from `go test -run '^$' -bench Echo -benchtime 500ms | go run ../cmd/results` at ews commit `39a6bc0`.

![echo-plain](echo-plain.svg)

![echo-compressed](echo-compressed.svg)

![echo-nocontext](echo-nocontext.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0, default build: SWAR masking and the shift-based UTF-8 validator
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
| 64 B | 1 | 9 MB/s | 8 MB/s | 9 MB/s | 8 MB/s | 6 MB/s | 6 MB/s | 8 MB/s | 8 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 32 | 66 MB/s | 57 MB/s | 65 MB/s | 63 MB/s | 38 MB/s | 48 MB/s | 56 MB/s | 63 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 16 / 2 / 2 |
| 64 B | 128 | 75 MB/s | 73 MB/s | 71 MB/s | 70 MB/s | 50 MB/s | 53 MB/s | 63 MB/s | 71 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 512 | 75 MB/s | 73 MB/s | 75 MB/s | 71 MB/s | 55 MB/s | 56 MB/s | 66 MB/s | 74 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 1024 | 75 MB/s | 73 MB/s | 73 MB/s | 70 MB/s | 54 MB/s | 54 MB/s | 66 MB/s | 67 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 64 B | 2048 | 62 MB/s | 63 MB/s | 64 MB/s | 61 MB/s | 47 MB/s | 46 MB/s | 60 MB/s | 63 MB/s | 0 / 0 / 1 / 6 / 13 / 16 / 2 / 2 |
| 1 KiB | 1 | 131 MB/s | 134 MB/s | 130 MB/s | 125 MB/s | 49 MB/s | 93 MB/s | 75 MB/s | 123 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 32 | 1.0 GB/s | 1.0 GB/s | 1.0 GB/s | 945 MB/s | 557 MB/s | 743 MB/s | 733 MB/s | 980 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 128 | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 598 MB/s | 799 MB/s | 809 MB/s | 1.1 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 512 | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 1.1 GB/s | 656 MB/s | 814 MB/s | 850 MB/s | 1.1 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 1024 | 1.1 GB/s | 1.1 GB/s | 1.0 GB/s | 1.0 GB/s | 624 MB/s | 766 MB/s | 794 MB/s | 1.0 GB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 1 KiB | 2048 | 897 MB/s | 861 MB/s | 888 MB/s | 845 MB/s | 570 MB/s | 672 MB/s | 694 MB/s | 770 MB/s | 0 / 0 / 1 / 6 / 24 (2 KB) / 16 / 5 (2 KB) / 2 |
| 16 KiB | 1 | 1.5 GB/s | 1.4 GB/s | 1.4 GB/s | 1.4 GB/s | 179 MB/s | 747 MB/s | 215 MB/s | 612 MB/s | 0 / 0 / 1 / 6 / 61 (41 KB) / 16 / 16 (39 KB) / 2 |
| 16 KiB | 32 | 10.3 GB/s | 10.3 GB/s | 9.9 GB/s | 10.0 GB/s | 1.8 GB/s | 5.9 GB/s | 1.5 GB/s | 4.8 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 128 | 11.1 GB/s | 10.5 GB/s | 10.3 GB/s | 10.2 GB/s | 2.5 GB/s | 6.1 GB/s | 2.7 GB/s | 5.3 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 512 | 9.0 GB/s | 8.9 GB/s | 8.7 GB/s | 8.8 GB/s | 2.9 GB/s | 5.1 GB/s | 3.8 GB/s | 4.9 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 1024 | 6.7 GB/s | 6.5 GB/s | 6.5 GB/s | 6.5 GB/s | 2.8 GB/s | 4.3 GB/s | 3.0 GB/s | 4.3 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 16 KiB | 2048 | 5.7 GB/s | 5.6 GB/s | 5.6 GB/s | 5.5 GB/s | 2.5 GB/s | 3.9 GB/s | 2.7 GB/s | 4.0 GB/s | 0 / 0 / 1 / 6 / 61 (39 KB) / 16 / 16 (37 KB) / 2 |
| 256 KiB | 1 | 3.1 GB/s | 2.9 GB/s | 582 MB/s | 3.0 GB/s | 458 MB/s | 1.7 GB/s | 469 MB/s | 810 MB/s | 0 / 0 / 6 (594 KB) / 12 / 97 (699 KB) / 72 (3 KB) / 24 (698 KB) / 2 |
| 256 KiB | 32 | 17.4 GB/s | 16.8 GB/s | 3.9 GB/s | 16.9 GB/s | 3.9 GB/s | 12.9 GB/s | 2.8 GB/s | 7.1 GB/s | 0 / 0 / 5 (574 KB) / 12 / 96 (669 KB) / 72 (4 KB) / 23 (669 KB) / 2 |
| 256 KiB | 128 | 10.4 GB/s | 9.7 GB/s | 6.3 GB/s | 9.7 GB/s | 5.6 GB/s | 8.9 GB/s | 5.8 GB/s | 6.4 GB/s | 0 / 0 / 5 (536 KB) / 12 / 96 (631 KB) / 72 (2 KB) / 23 (627 KB) / 2 |
| 256 KiB | 512 | 9.0 GB/s | 8.7 GB/s | 5.5 GB/s | 8.1 GB/s | 4.5 GB/s | 7.8 GB/s | 4.9 GB/s | 6.0 GB/s | 0 / 0 / 5 (541 KB) / 12 / 96 (636 KB) / 72 (2 KB) / 23 (630 KB) / 2 |
| 256 KiB | 1024 | 8.5 GB/s | 8.3 GB/s | 6.3 GB/s | 8.2 GB/s | 5.3 GB/s | 7.8 GB/s | 5.7 GB/s | 6.2 GB/s | 0 / 0 / 5 (548 KB) / 12 / 96 (640 KB) / 72 (2 KB) / 23 (637 KB) / 2 |
| 256 KiB | 2048 | 7.9 GB/s | 7.8 GB/s | 5.2 GB/s | 7.9 GB/s | 4.9 GB/s | 7.6 GB/s | 5.1 GB/s | 5.9 GB/s | 0 / 0 / 5 (580 KB) / 12 / 96 (664 KB) / 72 (2 KB) / 23 (650 KB) / 2 |

## Compressed with context takeover

| Size | Conns | ews | ews-shared | ews-stream | gws | gws-stream | coder | coder-stream | allocs/op ews / ews-shared / ews-stream / gws / gws-stream / coder / coder-stream |
|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 4 MB/s | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 3 MB/s | 0 / 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) |
| 64 B | 32 | 42 MB/s | 34 MB/s | 41 MB/s | 28 MB/s | 29 MB/s | 33 MB/s | 30 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 128 | 44 MB/s | 36 MB/s | 44 MB/s | 30 MB/s | 31 MB/s | 36 MB/s | 30 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 512 | 38 MB/s | 32 MB/s | 39 MB/s | 26 MB/s | 29 MB/s | 32 MB/s | 28 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 1024 | 37 MB/s | 35 MB/s | 37 MB/s | 33 MB/s | 33 MB/s | 31 MB/s | 29 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 64 B | 2048 | 36 MB/s | 35 MB/s | 36 MB/s | 34 MB/s | 34 MB/s | 30 MB/s | 27 MB/s | 0 / 0 / 0 / 1 / 6 / 13 / 32 (1 KB) |
| 1 KiB | 1 | 76 MB/s | 59 MB/s | 75 MB/s | 58 MB/s | 57 MB/s | 51 MB/s | 54 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 32 | 597 MB/s | 515 MB/s | 599 MB/s | 429 MB/s | 438 MB/s | 476 MB/s | 483 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 128 | 624 MB/s | 502 MB/s | 588 MB/s | 440 MB/s | 457 MB/s | 481 MB/s | 491 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 512 | 427 MB/s | 358 MB/s | 419 MB/s | 269 MB/s | 282 MB/s | 329 MB/s | 337 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 1024 | 348 MB/s | 319 MB/s | 358 MB/s | 248 MB/s | 244 MB/s | 284 MB/s | 289 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 1 KiB | 2048 | 336 MB/s | 301 MB/s | 340 MB/s | 244 MB/s | 244 MB/s | 247 MB/s | 279 MB/s | 0 / 0 / 0 / 1 / 6 / 16 (2 KB) / 20 |
| 16 KiB | 1 | 682 MB/s | 588 MB/s | 660 MB/s | 580 MB/s | 562 MB/s | 178 MB/s | 558 MB/s | 0 / 0 / 0 / 1 / 6 / 25 (42 KB) / 20 |
| 16 KiB | 32 | 4.5 GB/s | 4.7 GB/s | 4.7 GB/s | 4.5 GB/s | 4.5 GB/s | 2.7 GB/s | 4.5 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 128 | 3.3 GB/s | 3.4 GB/s | 3.2 GB/s | 3.5 GB/s | 3.4 GB/s | 2.6 GB/s | 3.4 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 512 | 2.4 GB/s | 2.6 GB/s | 2.3 GB/s | 2.2 GB/s | 2.3 GB/s | 2.0 GB/s | 2.3 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (38 KB) / 20 |
| 16 KiB | 1024 | 2.2 GB/s | 2.4 GB/s | 2.3 GB/s | 2.0 GB/s | 2.2 GB/s | 1.8 GB/s | 2.1 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 16 KiB | 2048 | 2.2 GB/s | 2.3 GB/s | 2.2 GB/s | 2.1 GB/s | 2.2 GB/s | 1.5 GB/s | 2.1 GB/s | 0 / 0 / 0 / 1 / 6 / 25 (37 KB) / 20 |
| 256 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 1.2 GB/s | 395 MB/s | 363 MB/s | 267 MB/s | 1.3 GB/s | 0 / 0 (1 KB) / 0 / 18 (1392 KB) / 22 (1190 KB) / 39 (958 KB) / 56 (3 KB) |
| 256 KiB | 32 | 11.4 GB/s | 12.3 GB/s | 11.1 GB/s | 3.4 GB/s | 3.5 GB/s | 5.0 GB/s | 11.0 GB/s | 0 / 0 (1 KB) / 0 (1 KB) / 17 (1360 KB) / 22 (1157 KB) / 33 (689 KB) / 56 (2 KB) |
| 256 KiB | 128 | 9.6 GB/s | 11.1 GB/s | 9.6 GB/s | 3.6 GB/s | 3.6 GB/s | 5.1 GB/s | 9.1 GB/s | 0 / 0 (1 KB) / 0 / 17 (1357 KB) / 21 (1097 KB) / 32 (642 KB) / 56 (2 KB) |
| 256 KiB | 512 | 8.8 GB/s | 10.1 GB/s | 8.1 GB/s | 3.7 GB/s | 3.6 GB/s | 5.1 GB/s | 7.5 GB/s | 0 / 0 / 0 / 17 (1367 KB) / 21 (1169 KB) / 32 (663 KB) / 56 (2 KB) |
| 256 KiB | 1024 | 8.7 GB/s | 9.0 GB/s | 8.9 GB/s | 3.5 GB/s | 3.3 GB/s | 4.8 GB/s | 8.3 GB/s | 0 / 0 (1 KB) / 0 / 18 (1429 KB) / 23 (1245 KB) / 33 (706 KB) / 56 (2 KB) |
| 256 KiB | 2048 | 8.8 GB/s | 9.9 GB/s | 8.8 GB/s | 3.3 GB/s | 2.4 GB/s | 2.8 GB/s | 8.4 GB/s | 0 / 0 / 0 / 20 (1537 KB) / 25 (1436 KB) / 32 (616 KB) / 56 (2 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-stream | gws | gws-stream | coder | coder-stream | gorilla | gorilla-stream | allocs/op ews / ews-stream / gws / gws-stream / coder / coder-stream / gorilla / gorilla-stream |
|---|---|---|---|---|---|---|---|---|---|---|
| 64 B | 1 | 5 MB/s | 5 MB/s | 6 MB/s | 5 MB/s | 4 MB/s | 4 MB/s | 4 MB/s | 5 MB/s | 0 / 0 / 1 / 6 / 13 (2 KB) / 32 (1 KB) / 9 (1 KB) / 8 |
| 64 B | 32 | 42 MB/s | 40 MB/s | 46 MB/s | 41 MB/s | 35 MB/s | 29 MB/s | 39 MB/s | 39 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 / 8 |
| 64 B | 128 | 45 MB/s | 47 MB/s | 47 MB/s | 48 MB/s | 37 MB/s | 32 MB/s | 45 MB/s | 44 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 (1 KB) / 8 |
| 64 B | 512 | 47 MB/s | 43 MB/s | 51 MB/s | 43 MB/s | 33 MB/s | 29 MB/s | 35 MB/s | 41 MB/s | 0 / 0 / 1 / 6 / 13 (1 KB) / 32 (1 KB) / 9 (1 KB) / 8 |
| 64 B | 1024 | 47 MB/s | 41 MB/s | 47 MB/s | 37 MB/s | 32 MB/s | 26 MB/s | 35 MB/s | 36 MB/s | 0 / 0 / 1 / 6 (1 KB) / 13 (1 KB) / 32 (2 KB) / 9 (2 KB) / 8 (1 KB) |
| 64 B | 2048 | 38 MB/s | 41 MB/s | 47 MB/s | 40 MB/s | 30 MB/s | 26 MB/s | 28 MB/s | 27 MB/s | 0 / 0 / 1 / 6 / 13 (2 KB) / 32 (3 KB) / 9 (3 KB) / 8 |
| 1 KiB | 1 | 64 MB/s | 64 MB/s | 67 MB/s | 61 MB/s | 44 MB/s | 48 MB/s | 54 MB/s | 62 MB/s | 0 / 0 / 1 / 6 / 16 (4 KB) / 20 (1 KB) / 12 (3 KB) / 8 |
| 1 KiB | 32 | 570 MB/s | 578 MB/s | 564 MB/s | 562 MB/s | 456 MB/s | 459 MB/s | 511 MB/s | 560 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 (1 KB) / 12 (2 KB) / 8 |
| 1 KiB | 128 | 591 MB/s | 567 MB/s | 584 MB/s | 586 MB/s | 445 MB/s | 462 MB/s | 503 MB/s | 506 MB/s | 0 / 0 / 1 / 6 / 16 (3 KB) / 20 (1 KB) / 12 (3 KB) / 8 |
| 1 KiB | 512 | 597 MB/s | 610 MB/s | 583 MB/s | 565 MB/s | 442 MB/s | 450 MB/s | 512 MB/s | 549 MB/s | 0 / 0 / 1 / 6 (1 KB) / 16 (3 KB) / 20 (1 KB) / 12 (3 KB) / 8 |
| 1 KiB | 1024 | 563 MB/s | 591 MB/s | 571 MB/s | 518 MB/s | 420 MB/s | 445 MB/s | 483 MB/s | 537 MB/s | 0 (2 KB) / 0 / 1 / 6 (2 KB) / 16 (4 KB) / 20 (1 KB) / 12 (3 KB) / 8 |
| 1 KiB | 2048 | 535 MB/s | 546 MB/s | 536 MB/s | 522 MB/s | 351 MB/s | 414 MB/s | 453 MB/s | 493 MB/s | 0 / 0 / 1 / 6 / 16 (5 KB) / 20 / 12 (3 KB) / 8 |
| 16 KiB | 1 | 519 MB/s | 527 MB/s | 549 MB/s | 538 MB/s | 145 MB/s | 478 MB/s | 142 MB/s | 535 MB/s | 0 / 0 / 1 / 6 / 25 (68 KB) / 20 (1 KB) / 21 (65 KB) / 8 |
| 16 KiB | 32 | 5.4 GB/s | 5.3 GB/s | 5.1 GB/s | 5.2 GB/s | 3.0 GB/s | 4.6 GB/s | 3.1 GB/s | 5.0 GB/s | 0 / 0 / 1 / 6 / 25 (45 KB) / 20 (1 KB) / 21 (43 KB) / 8 |
| 16 KiB | 128 | 5.4 GB/s | 5.4 GB/s | 5.0 GB/s | 5.2 GB/s | 2.8 GB/s | 4.6 GB/s | 3.0 GB/s | 5.2 GB/s | 0 / 0 / 1 / 6 / 25 (47 KB) / 20 (1 KB) / 21 (44 KB) / 8 |
| 16 KiB | 512 | 5.5 GB/s | 5.5 GB/s | 5.2 GB/s | 4.9 GB/s | 2.9 GB/s | 4.4 GB/s | 3.2 GB/s | 5.0 GB/s | 0 / 0 / 1 / 6 (2 KB) / 25 (46 KB) / 20 (3 KB) / 21 (45 KB) / 8 |
| 16 KiB | 1024 | 5.5 GB/s | 5.4 GB/s | 5.3 GB/s | 5.1 GB/s | 2.8 GB/s | 4.3 GB/s | 3.0 GB/s | 5.0 GB/s | 0 / 0 / 1 / 6 (1 KB) / 25 (45 KB) / 20 (1 KB) / 21 (46 KB) / 8 |
| 16 KiB | 2048 | 5.0 GB/s | 5.2 GB/s | 4.4 GB/s | 4.8 GB/s | 2.5 GB/s | 4.1 GB/s | 2.1 GB/s | 4.3 GB/s | 0 (2 KB) / 0 / 1 / 6 (1 KB) / 25 (48 KB) / 20 / 21 (46 KB) / 8 |
| 256 KiB | 1 | 1.6 GB/s | 1.3 GB/s | 338 MB/s | 338 MB/s | 206 MB/s | 1.3 GB/s | 185 MB/s | 1.3 GB/s | 0 / 0 / 19 (1467 KB) / 23 (1242 KB) / 48 (1374 KB) / 56 (5 KB) / 45 (1420 KB) / 8 (1 KB) |
| 256 KiB | 32 | 14.2 GB/s | 11.7 GB/s | 2.6 GB/s | 3.0 GB/s | 3.6 GB/s | 11.1 GB/s | 3.5 GB/s | 12.2 GB/s | 0 (1 KB) / 0 (1 KB) / 19 (1476 KB) / 24 (1258 KB) / 39 (933 KB) / 56 (6 KB) / 35 (970 KB) / 8 (3 KB) |
| 256 KiB | 128 | 14.0 GB/s | 11.8 GB/s | 2.8 GB/s | 3.5 GB/s | 4.3 GB/s | 12.5 GB/s | 4.8 GB/s | 13.4 GB/s | 0 (3 KB) / 0 (1 KB) / 21 (1562 KB) / 22 (1149 KB) / 35 (785 KB) / 56 (8 KB) / 30 (741 KB) / 8 (3 KB) |
| 256 KiB | 512 | 14.4 GB/s | 12.4 GB/s | 3.3 GB/s | 3.2 GB/s | 5.2 GB/s | 12.6 GB/s | 5.2 GB/s | 13.3 GB/s | 0 / 0 / 20 (1495 KB) / 22 (1205 KB) / 34 (732 KB) / 56 (7 KB) / 30 (727 KB) / 8 (1 KB) |
| 256 KiB | 1024 | 14.2 GB/s | 12.4 GB/s | 3.2 GB/s | 3.3 GB/s | 5.0 GB/s | 12.5 GB/s | 5.3 GB/s | 13.4 GB/s | 0 / 0 / 20 (1528 KB) / 25 (1343 KB) / 35 (770 KB) / 56 (3 KB) / 30 (746 KB) / 8 (1 KB) |
| 256 KiB | 2048 | 14.4 GB/s | 12.1 GB/s | 3.3 GB/s | 2.8 GB/s | 4.7 GB/s | 12.4 GB/s | 4.8 GB/s | 13.2 GB/s | 0 / 0 / 21 (1612 KB) / 30 (1653 KB) / 37 (885 KB) / 56 (3 KB) / 32 (848 KB) / 8 (1 KB) |

## Reading the numbers

- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed with takeover, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- Without takeover the small-message cells tie across ews, gws and gorilla-stream, coder and gorilla's simple API trail on allocations, and at 256 KiB ews reaches 14 GB/s against 13 for gorilla-stream and coder-stream and 3 to 5 for the simple APIs. Comparing the two compressed tables gives each library's cost of takeover: a 32 KB dictionary primed per message and history copied on both ends. For ews that is 5 to 10 percent at 1 KiB and a third at 256 KiB, and the same or more for the others; takeover buys ratio, not speed, on traffic that repeats.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented `Read` assembles messages through `io.ReadAll`, which dominates its large-message cells; piping `Reader` into `Writer` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.

