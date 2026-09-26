# Broadcast benchmark results

Run at ews commit `5387555` with `go test -run '^$' -bench Broadcast -benchtime 1s`; tables and charts generated from the saved output by `go run ../cmd/results` on 2026-09-26.

![broadcast-plain-simd](broadcast-plain-simd.svg)

![broadcast-compressed-simd](broadcast-compressed-simd.svg)

![broadcast-nocontext-simd](broadcast-nocontext-simd.svg)

## Setup

- CPU: AMD Ryzen 9 9950X3D 16-Core Processor
- Kernel: 7.2.2-1-cachyos
- Go: go1.27.1-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
- GOMAXPROCS: 8
- gws: v1.10.2
- coder/websocket: v1.8.15
- gorilla/websocket: v1.5.3

One message of 256 bytes, 4 KiB, 64 KiB or 256 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second. CPU per message is the process's user and system time over the timed rounds divided by messages delivered; it includes the clients' reads, which are the same for every server, so differences between columns are the servers'. Allocations are process-wide per round, so ews's few are the `Prepared` made once per round, not per recipient.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.
- `gorilla`: `NewPreparedMessage` once, then `WritePreparedMessage` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with `ews-sync`. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | gorilla | CPU/msg ews / ews-sync / gws / gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|---|
| 256 B | 128 | 1.65M msgs/s | 710k msgs/s | 1.61M msgs/s | 668k msgs/s | 2.5 µs / 3.1 µs / 2.5 µs / 3.3 µs | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.03M msgs/s | 738k msgs/s | 2.02M msgs/s | 696k msgs/s | 2.4 µs / 3.1 µs / 2.4 µs / 3.2 µs | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.14M msgs/s | 762k msgs/s | 2.14M msgs/s | 723k msgs/s | 2.4 µs / 3.1 µs / 2.3 µs / 3.2 µs | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 256 B | 8192 | 1.67M msgs/s | 675k msgs/s | 1.68M msgs/s | 623k msgs/s | 3.2 µs / 3.4 µs / 3.1 µs / 3.5 µs | 2 / 2 / 16395 (580 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.46M msgs/s | 646k msgs/s | 1.46M msgs/s | 609k msgs/s | 2.9 µs / 3.5 µs / 2.9 µs / 3.7 µs | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.78M msgs/s | 692k msgs/s | 1.78M msgs/s | 650k msgs/s | 2.8 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.83M msgs/s | 693k msgs/s | 1.86M msgs/s | 653k msgs/s | 2.8 µs / 3.5 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 4 KiB | 8192 | 1.09M msgs/s | 545k msgs/s | 1.12M msgs/s | 507k msgs/s | 5.0 µs / 4.2 µs / 5.0 µs / 4.4 µs | 2 (4 KB) / 2 (4 KB) / 16387 (576 KB) / 15 (20 KB) |
| 64 KiB | 128 | 766k msgs/s | 174k msgs/s | 772k msgs/s | 172k msgs/s | 6.0 µs / 7.5 µs / 5.9 µs / 8.0 µs | 2 (72 KB) / 2 (72 KB) / 259 (9 KB) / 21 (165 KB) |
| 64 KiB | 512 | 1.01M msgs/s | 177k msgs/s | 1.01M msgs/s | 179k msgs/s | 5.1 µs / 7.4 µs / 5.1 µs / 7.3 µs | 2 (72 KB) / 2 (72 KB) / 1027 (36 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 415k msgs/s | 179k msgs/s | 418k msgs/s | 182k msgs/s | 14.6 µs / 7.3 µs / 14.8 µs / 7.2 µs | 2 (72 KB) / 2 (72 KB) / 4099 (146 KB) / 21 (165 KB) |
| 64 KiB | 8192 | 264k msgs/s | 173k msgs/s | 268k msgs/s | 174k msgs/s | 24.8 µs / 7.8 µs / 25.1 µs / 7.8 µs | 2 (72 KB) / 2 (72 KB) / 16387 (579 KB) / 21 (164 KB) |
| 256 KiB | 128 | 321k msgs/s | 74k msgs/s | 321k msgs/s | 73k msgs/s | 17.0 µs / 21.4 µs / 17.2 µs / 22.1 µs | 2 (267 KB) / 2 (283 KB) / 261 (277 KB) / 21 (584 KB) |
| 256 KiB | 512 | 196k msgs/s | 72k msgs/s | 193k msgs/s | 70k msgs/s | 33.1 µs / 21.4 µs / 33.8 µs / 22.5 µs | 2 (270 KB) / 2 (264 KB) / 1029 (300 KB) / 21 (559 KB) |
| 256 KiB | 2048 | 77k msgs/s | 69k msgs/s | 76k msgs/s | 71k msgs/s | 94.3 µs / 22.4 µs / 93.7 µs / 21.5 µs | 2 (264 KB) / 2 (264 KB) / 4101 (408 KB) / 21 (560 KB) |
| 256 KiB | 8192 | 71k msgs/s | 66k msgs/s | 71k msgs/s | 67k msgs/s | 99.5 µs / 22.9 µs / 99.0 µs / 22.6 µs | 2 (264 KB) / 2 (264 KB) / 16389 (868 KB) / 21 (548 KB) |
| 2 MiB | 128 | 32k msgs/s | 10k msgs/s | 32k msgs/s | 10k msgs/s | 205.1 µs / 164.8 µs / 207.4 µs / 163.7 µs | 4 (2211 KB) / 4 (2668 KB) / 262 (2222 KB) / 24 (5072 KB) |
| 6 MiB | 128 | 3k msgs/s | 3k msgs/s | 3k msgs/s | 3k msgs/s | 2542.6 µs / 522.4 µs / 2637.1 µs / 489.4 µs | 2 (6152 KB) / 3 (7463 KB) / 262 (9627 KB) / 21 (12324 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | CPU/msg ews / ews-sync / gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.30M msgs/s | 712k msgs/s | 1.20M msgs/s | 3.4 µs / 3.6 µs / 3.8 µs | 4 / 4 / 259 (9 KB) |
| 256 B | 512 | 1.58M msgs/s | 762k msgs/s | 1.48M msgs/s | 3.2 µs / 3.6 µs / 3.6 µs | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.44M msgs/s | 701k msgs/s | 843k msgs/s | 4.0 µs / 3.9 µs / 7.4 µs | 4 (1 KB) / 4 (13 KB) / 4099 (144 KB) |
| 256 B | 8192 | 1.02M msgs/s | 439k msgs/s | 793k msgs/s | 6.0 µs / 5.2 µs / 7.9 µs | 4 (5 KB) / 4 (32 KB) / 16387 (576 KB) |
| 4 KiB | 128 | 1.07M msgs/s | 708k msgs/s | 1.02M msgs/s | 4.6 µs / 4.7 µs / 5.0 µs | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.27M msgs/s | 774k msgs/s | 1.19M msgs/s | 4.5 µs / 4.7 µs / 4.8 µs | 4 (4 KB) / 4 (7 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.09M msgs/s | 696k msgs/s | 704k msgs/s | 5.8 µs / 5.4 µs / 9.1 µs | 4 (6 KB) / 4 (14 KB) / 4099 (144 KB) |
| 4 KiB | 8192 | 727k msgs/s | 429k msgs/s | 433k msgs/s | 8.6 µs / 7.1 µs / 14.9 µs | 4 (20 KB) / 4 (4 KB) / 16389 (589 KB) |
| 64 KiB | 128 | 639k msgs/s | 651k msgs/s | 632k msgs/s | 9.1 µs / 8.7 µs / 9.4 µs | 4 (74 KB) / 4 (72 KB) / 259 (9 KB) |
| 64 KiB | 512 | 741k msgs/s | 704k msgs/s | 712k msgs/s | 8.8 µs / 8.4 µs / 9.2 µs | 4 (74 KB) / 4 (74 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 707k msgs/s | 676k msgs/s | 478k msgs/s | 9.6 µs / 8.8 µs / 14.2 µs | 4 (76 KB) / 4 (82 KB) / 4099 (145 KB) |
| 64 KiB | 8192 | 525k msgs/s | 382k msgs/s | 349k msgs/s | 12.8 µs / 11.7 µs / 19.9 µs | 4 (82 KB) / 4 (86 KB) / 16387 (576 KB) |
| 256 KiB | 128 | 284k msgs/s | 270k msgs/s | 276k msgs/s | 23.2 µs / 23.4 µs / 24.0 µs | 4 (266 KB) / 4 (298 KB) / 261 (291 KB) |
| 256 KiB | 512 | 311k msgs/s | 303k msgs/s | 307k msgs/s | 23.2 µs / 22.8 µs / 23.5 µs | 4 (285 KB) / 4 (278 KB) / 1029 (328 KB) |
| 256 KiB | 2048 | 302k msgs/s | 306k msgs/s | 254k msgs/s | 23.8 µs / 23.3 µs / 28.3 µs | 4 (269 KB) / 4 (269 KB) / 4101 (426 KB) |
| 256 KiB | 8192 | 278k msgs/s | 292k msgs/s | 223k msgs/s | 25.9 µs / 25.2 µs / 32.5 µs | 4 (285 KB) / 4 (283 KB) / 16389 (840 KB) |
| 2 MiB | 128 | 12k msgs/s | 12k msgs/s | 12k msgs/s | 628.9 µs / 606.5 µs / 624.3 µs | 15 (3382 KB) / 4 (2062 KB) / 264 (2648 KB) |
| 6 MiB | 128 | 15k msgs/s | 14k msgs/s | 15k msgs/s | 468.0 µs / 465.6 µs / 465.0 µs | 20 (9960 KB) / 19 (9249 KB) / 269 (9059 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | CPU/msg ews / ews-sync / gws / gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|---|
| 256 B | 128 | 1.47M msgs/s | 690k msgs/s | 1.45M msgs/s | 651k msgs/s | 2.8 µs / 3.3 µs / 2.8 µs / 3.5 µs | 4 / 4 (1 KB) / 259 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.80M msgs/s | 738k msgs/s | 1.80M msgs/s | 695k msgs/s | 2.7 µs / 3.2 µs / 2.7 µs / 3.4 µs | 4 / 4 (3 KB) / 1027 (36 KB) / 18 (14 KB) |
| 256 B | 2048 | 1.96M msgs/s | 765k msgs/s | 1.97M msgs/s | 725k msgs/s | 2.7 µs / 3.1 µs / 2.7 µs / 3.3 µs | 4 (2 KB) / 4 (7 KB) / 4099 (144 KB) / 18 (20 KB) |
| 256 B | 8192 | 1.59M msgs/s | 669k msgs/s | 1.59M msgs/s | 634k msgs/s | 3.4 µs / 3.5 µs / 3.4 µs / 3.7 µs | 4 / 4 (8 KB) / 16387 (576 KB) / 18 (20 KB) |
| 4 KiB | 128 | 1.18M msgs/s | 678k msgs/s | 1.18M msgs/s | 641k msgs/s | 4.0 µs / 4.5 µs / 4.0 µs / 4.6 µs | 4 (4 KB) / 4 (5 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.42M msgs/s | 737k msgs/s | 1.40M msgs/s | 700k msgs/s | 3.8 µs / 4.3 µs / 3.9 µs / 4.4 µs | 4 (5 KB) / 4 (7 KB) / 1027 (36 KB) / 18 (20 KB) |
| 4 KiB | 2048 | 1.51M msgs/s | 762k msgs/s | 1.52M msgs/s | 726k msgs/s | 3.8 µs / 4.2 µs / 3.9 µs / 4.4 µs | 4 (6 KB) / 4 (13 KB) / 4099 (144 KB) / 18 (23 KB) |
| 4 KiB | 8192 | 1.27M msgs/s | 665k msgs/s | 1.26M msgs/s | 625k msgs/s | 4.6 µs / 4.6 µs / 4.6 µs / 4.7 µs | 4 (4 KB) / 4 (13 KB) / 16387 (576 KB) / 18 (24 KB) |
| 64 KiB | 128 | 680k msgs/s | 638k msgs/s | 701k msgs/s | 590k msgs/s | 8.4 µs / 8.3 µs / 8.2 µs / 8.6 µs | 4 (75 KB) / 4 (74 KB) / 259 (9 KB) / 21 (90 KB) |
| 64 KiB | 512 | 798k msgs/s | 715k msgs/s | 795k msgs/s | 655k msgs/s | 8.0 µs / 7.8 µs / 8.1 µs / 8.0 µs | 4 (75 KB) / 4 (76 KB) / 1027 (38 KB) / 21 (90 KB) |
| 64 KiB | 2048 | 834k msgs/s | 747k msgs/s | 834k msgs/s | 689k msgs/s | 8.0 µs / 7.6 µs / 8.0 µs / 7.8 µs | 4 (78 KB) / 4 (79 KB) / 4099 (147 KB) / 21 (96 KB) |
| 64 KiB | 8192 | 723k msgs/s | 598k msgs/s | 732k msgs/s | 553k msgs/s | 8.9 µs / 8.4 µs / 8.9 µs / 8.8 µs | 4 (72 KB) / 4 (81 KB) / 16387 (578 KB) / 21 (88 KB) |
| 256 KiB | 128 | 285k msgs/s | 276k msgs/s | 289k msgs/s | 265k msgs/s | 22.9 µs / 22.6 µs / 22.8 µs / 22.9 µs | 4 (302 KB) / 4 (303 KB) / 261 (303 KB) / 22 (322 KB) |
| 256 KiB | 512 | 321k msgs/s | 315k msgs/s | 322k msgs/s | 311k msgs/s | 22.3 µs / 21.9 µs / 22.3 µs / 22.0 µs | 5 (314 KB) / 5 (347 KB) / 1029 (324 KB) / 21 (321 KB) |
| 256 KiB | 2048 | 325k msgs/s | 332k msgs/s | 332k msgs/s | 334k msgs/s | 22.1 µs / 21.7 µs / 22.1 µs / 21.7 µs | 4 (269 KB) / 5 (327 KB) / 4102 (470 KB) / 21 (292 KB) |
| 256 KiB | 8192 | 306k msgs/s | 332k msgs/s | 315k msgs/s | 332k msgs/s | 23.3 µs / 22.5 µs / 23.3 µs / 22.5 µs | 4 (302 KB) / 4 (281 KB) / 16389 (840 KB) / 21 (297 KB) |
| 2 MiB | 128 | 13k msgs/s | 13k msgs/s | 13k msgs/s | 13k msgs/s | 567.1 µs / 563.9 µs / 566.0 µs / 563.6 µs | 11 (2857 KB) / 16 (3559 KB) / 267 (3099 KB) / 33 (3555 KB) |
| 6 MiB | 128 | 15k msgs/s | 14k msgs/s | 15k msgs/s | 14k msgs/s | 466.9 µs / 464.7 µs / 460.2 µs / 467.9 µs | 24 (10006 KB) / 16 (8411 KB) / 265 (7387 KB) / 38 (9139 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.
- The CPU column separates what the throughput tie hides. Below the bandwidth ceiling the asynchronous paths cost a fifth less CPU per delivery than the synchronous ones. At the ceiling, 64 KiB and 256 KiB to 2048 clients and up, they cost two to four times more for the same throughput: thousands of writers copying at once turn memory stalls into CPU time, where one goroutine writing in turn does not. With takeover, gws's per-recipient window copy shows from 2048 clients on: at 8192 gws delivers 60 to 80 percent of ews's messages per second on 1.2 to 1.7 times the CPU per message, widest at 4 KiB.

