# Broadcast benchmark results

Run at ews commit `955c34d` with `go test -run '^$' -bench Broadcast -benchtime 1s`; tables and charts generated from the saved output by `go run ../cmd/results` on 2026-09-19.

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
| 256 B | 128 | 1.62M msgs/s | 707k msgs/s | 1.59M msgs/s | 660k msgs/s | 2.5 µs / 3.2 µs / 2.5 µs / 3.3 µs | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 1.98M msgs/s | 737k msgs/s | 1.97M msgs/s | 695k msgs/s | 2.3 µs / 3.1 µs / 2.4 µs / 3.3 µs | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.12M msgs/s | 769k msgs/s | 2.11M msgs/s | 728k msgs/s | 2.4 µs / 3.1 µs / 2.3 µs / 3.2 µs | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 256 B | 8192 | 1.68M msgs/s | 674k msgs/s | 1.68M msgs/s | 623k msgs/s | 3.1 µs / 3.4 µs / 3.1 µs / 3.5 µs | 2 / 2 / 16396 (581 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.43M msgs/s | 646k msgs/s | 1.44M msgs/s | 607k msgs/s | 2.9 µs / 3.5 µs / 2.9 µs / 3.7 µs | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.76M msgs/s | 696k msgs/s | 1.76M msgs/s | 653k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.87M msgs/s | 703k msgs/s | 1.91M msgs/s | 666k msgs/s | 2.8 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 4 KiB | 8192 | 1.11M msgs/s | 549k msgs/s | 1.14M msgs/s | 508k msgs/s | 5.0 µs / 4.2 µs / 4.9 µs / 4.4 µs | 2 (4 KB) / 2 (4 KB) / 16387 (576 KB) / 15 (20 KB) |
| 64 KiB | 128 | 749k msgs/s | 176k msgs/s | 756k msgs/s | 172k msgs/s | 6.1 µs / 7.5 µs / 5.9 µs / 7.9 µs | 2 (72 KB) / 2 (72 KB) / 259 (9 KB) / 21 (165 KB) |
| 64 KiB | 512 | 1.01M msgs/s | 178k msgs/s | 1.00M msgs/s | 175k msgs/s | 5.1 µs / 7.4 µs / 5.1 µs / 7.6 µs | 2 (72 KB) / 2 (72 KB) / 1027 (36 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 416k msgs/s | 192k msgs/s | 417k msgs/s | 179k msgs/s | 14.6 µs / 7.1 µs / 14.6 µs / 7.5 µs | 2 (72 KB) / 2 (72 KB) / 4099 (147 KB) / 21 (165 KB) |
| 64 KiB | 8192 | 265k msgs/s | 174k msgs/s | 265k msgs/s | 172k msgs/s | 24.5 µs / 7.8 µs / 24.8 µs / 7.9 µs | 2 (72 KB) / 2 (74 KB) / 16387 (579 KB) / 21 (164 KB) |
| 256 KiB | 128 | 319k msgs/s | 75k msgs/s | 318k msgs/s | 73k msgs/s | 16.9 µs / 21.2 µs / 17.0 µs / 22.1 µs | 2 (267 KB) / 2 (277 KB) / 261 (277 KB) / 21 (595 KB) |
| 256 KiB | 512 | 197k msgs/s | 71k msgs/s | 198k msgs/s | 74k msgs/s | 32.8 µs / 21.7 µs / 32.6 µs / 21.2 µs | 2 (268 KB) / 2 (264 KB) / 1029 (306 KB) / 21 (557 KB) |
| 256 KiB | 2048 | 77k msgs/s | 69k msgs/s | 77k msgs/s | 71k msgs/s | 92.3 µs / 22.2 µs / 93.4 µs / 21.4 µs | 2 (270 KB) / 2 (264 KB) / 4101 (408 KB) / 21 (548 KB) |
| 256 KiB | 8192 | 71k msgs/s | 68k msgs/s | 71k msgs/s | 70k msgs/s | 99.1 µs / 22.3 µs / 97.1 µs / 21.7 µs | 2 (264 KB) / 2 (264 KB) / 16389 (840 KB) / 21 (548 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | CPU/msg ews / ews-sync / gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.25M msgs/s | 699k msgs/s | 1.17M msgs/s | 3.5 µs / 3.9 µs / 3.9 µs | 4 / 4 (1 KB) / 259 (9 KB) |
| 256 B | 512 | 1.56M msgs/s | 757k msgs/s | 1.43M msgs/s | 3.3 µs / 3.8 µs / 3.7 µs | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.42M msgs/s | 702k msgs/s | 823k msgs/s | 4.1 µs / 4.0 µs / 7.6 µs | 4 / 4 (11 KB) / 4099 (144 KB) |
| 256 B | 8192 | 1.01M msgs/s | 446k msgs/s | 793k msgs/s | 6.0 µs / 5.2 µs / 7.7 µs | 4 (5 KB) / 4 (32 KB) / 16387 (576 KB) |
| 4 KiB | 128 | 1.03M msgs/s | 696k msgs/s | 982k msgs/s | 4.8 µs / 4.9 µs / 5.1 µs | 4 (5 KB) / 4 (4 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.23M msgs/s | 758k msgs/s | 1.16M msgs/s | 4.5 µs / 5.0 µs / 4.9 µs | 4 (4 KB) / 4 (8 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.05M msgs/s | 703k msgs/s | 697k msgs/s | 6.0 µs / 5.6 µs / 9.3 µs | 4 (6 KB) / 4 (14 KB) / 4099 (144 KB) |
| 4 KiB | 8192 | 762k msgs/s | 395k msgs/s | 435k msgs/s | 8.4 µs / 7.3 µs / 14.6 µs | 4 (4 KB) / 4 (18 KB) / 16387 (576 KB) |
| 64 KiB | 128 | 607k msgs/s | 608k msgs/s | 591k msgs/s | 9.6 µs / 9.3 µs / 10.0 µs | 4 (75 KB) / 4 (75 KB) / 259 (9 KB) |
| 64 KiB | 512 | 706k msgs/s | 706k msgs/s | 676k msgs/s | 9.3 µs / 8.9 µs / 9.8 µs | 4 (72 KB) / 4 (75 KB) / 1027 (38 KB) |
| 64 KiB | 2048 | 674k msgs/s | 664k msgs/s | 465k msgs/s | 10.0 µs / 9.2 µs / 14.8 µs | 4 (72 KB) / 4 (78 KB) / 4099 (144 KB) |
| 64 KiB | 8192 | 503k msgs/s | 373k msgs/s | 341k msgs/s | 13.3 µs / 12.3 µs / 19.7 µs | 4 (72 KB) / 4 (72 KB) / 16387 (576 KB) |
| 256 KiB | 128 | 255k msgs/s | 243k msgs/s | 258k msgs/s | 25.8 µs / 25.6 µs / 25.7 µs | 5 (342 KB) / 6 (414 KB) / 261 (286 KB) |
| 256 KiB | 512 | 287k msgs/s | 281k msgs/s | 281k msgs/s | 25.0 µs / 24.7 µs / 25.7 µs | 4 (296 KB) / 4 (325 KB) / 1029 (313 KB) |
| 256 KiB | 2048 | 283k msgs/s | 284k msgs/s | 240k msgs/s | 25.7 µs / 25.3 µs / 30.2 µs | 4 (269 KB) / 4 (265 KB) / 4104 (677 KB) |
| 256 KiB | 8192 | 256k msgs/s | 273k msgs/s | 215k msgs/s | 28.0 µs / 27.1 µs / 34.1 µs | 4 (285 KB) / 4 (285 KB) / 16389 (840 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | CPU/msg ews / ews-sync / gws / gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|---|
| 256 B | 128 | 1.43M msgs/s | 682k msgs/s | 1.40M msgs/s | 641k msgs/s | 2.9 µs / 3.4 µs / 2.9 µs / 3.5 µs | 4 / 4 / 259 (9 KB) / 18 (11 KB) |
| 256 B | 512 | 1.75M msgs/s | 739k msgs/s | 1.74M msgs/s | 700k msgs/s | 2.7 µs / 3.2 µs / 2.8 µs / 3.4 µs | 4 / 4 (3 KB) / 1027 (36 KB) / 18 (11 KB) |
| 256 B | 2048 | 1.93M msgs/s | 781k msgs/s | 1.93M msgs/s | 729k msgs/s | 2.8 µs / 3.0 µs / 2.8 µs / 3.3 µs | 4 (1 KB) / 4 (7 KB) / 4099 (144 KB) / 18 (16 KB) |
| 256 B | 8192 | 1.62M msgs/s | 679k msgs/s | 1.59M msgs/s | 640k msgs/s | 3.5 µs / 3.5 µs / 3.4 µs / 3.7 µs | 4 / 4 / 16387 (576 KB) / 18 (29 KB) |
| 4 KiB | 128 | 1.14M msgs/s | 668k msgs/s | 1.15M msgs/s | 632k msgs/s | 4.1 µs / 4.6 µs / 4.1 µs / 4.7 µs | 4 (4 KB) / 4 (4 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.36M msgs/s | 739k msgs/s | 1.36M msgs/s | 698k msgs/s | 3.9 µs / 4.4 µs / 3.9 µs / 4.5 µs | 4 (4 KB) / 4 (5 KB) / 1027 (36 KB) / 18 (20 KB) |
| 4 KiB | 2048 | 1.49M msgs/s | 774k msgs/s | 1.47M msgs/s | 724k msgs/s | 3.9 µs / 4.2 µs / 4.0 µs / 4.4 µs | 4 (4 KB) / 4 (12 KB) / 4099 (146 KB) / 18 (25 KB) |
| 4 KiB | 8192 | 1.28M msgs/s | 671k msgs/s | 1.27M msgs/s | 634k msgs/s | 4.6 µs / 4.7 µs / 4.6 µs / 4.9 µs | 4 (4 KB) / 4 (13 KB) / 16387 (576 KB) / 18 (33 KB) |
| 64 KiB | 128 | 637k msgs/s | 620k msgs/s | 656k msgs/s | 586k msgs/s | 8.9 µs / 8.7 µs / 8.7 µs / 8.9 µs | 4 (75 KB) / 4 (75 KB) / 259 (9 KB) / 21 (94 KB) |
| 64 KiB | 512 | 745k msgs/s | 706k msgs/s | 749k msgs/s | 675k msgs/s | 8.6 µs / 8.2 µs / 8.6 µs / 8.4 µs | 4 (87 KB) / 4 (76 KB) / 1027 (38 KB) / 21 (96 KB) |
| 64 KiB | 2048 | 789k msgs/s | 750k msgs/s | 787k msgs/s | 712k msgs/s | 8.5 µs / 8.1 µs / 8.6 µs / 8.2 µs | 4 (82 KB) / 4 (90 KB) / 4099 (157 KB) / 21 (91 KB) |
| 64 KiB | 8192 | 713k msgs/s | 605k msgs/s | 696k msgs/s | 548k msgs/s | 9.4 µs / 8.8 µs / 9.3 µs / 9.1 µs | 4 (72 KB) / 4 (92 KB) / 16387 (576 KB) / 21 (88 KB) |
| 256 KiB | 128 | 260k msgs/s | 250k msgs/s | 265k msgs/s | 246k msgs/s | 25.1 µs / 24.8 µs / 24.9 µs / 25.0 µs | 5 (363 KB) / 5 (369 KB) / 262 (342 KB) / 22 (360 KB) |
| 256 KiB | 512 | 296k msgs/s | 291k msgs/s | 298k msgs/s | 285k msgs/s | 24.3 µs / 23.9 µs / 24.2 µs / 24.0 µs | 4 (309 KB) / 5 (333 KB) / 1030 (380 KB) / 23 (442 KB) |
| 256 KiB | 2048 | 301k msgs/s | 306k msgs/s | 305k msgs/s | 308k msgs/s | 24.1 µs / 23.6 µs / 24.0 µs / 23.8 µs | 6 (423 KB) / 4 (269 KB) / 4103 (577 KB) / 22 (416 KB) |
| 256 KiB | 8192 | 290k msgs/s | 308k msgs/s | 292k msgs/s | 307k msgs/s | 25.1 µs / 24.3 µs / 25.1 µs / 24.5 µs | 4 (283 KB) / 4 (301 KB) / 16389 (840 KB) / 21 (280 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.
- The CPU column separates what the throughput tie hides. Below the bandwidth ceiling the asynchronous paths cost a fifth less CPU per delivery than the synchronous ones. At the ceiling, 64 KiB and 256 KiB to 2048 clients and up, they cost two to four times more for the same throughput: thousands of writers copying at once turn memory stalls into CPU time, where one goroutine writing in turn does not. With takeover, gws's per-recipient window copy shows from 2048 clients on: at 8192 gws delivers 60 to 80 percent of ews's messages per second on 1.2 to 1.7 times the CPU per message, widest at 4 KiB.

