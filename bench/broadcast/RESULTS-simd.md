# Broadcast benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `5ae144b`.

![broadcast-plain-simd](broadcast-plain-simd.svg)

![broadcast-compressed-simd](broadcast-compressed-simd.svg)

![broadcast-nocontext-simd](broadcast-nocontext-simd.svg)

## Setup

- CPU: AMD Ryzen 9 9950X3D 16-Core Processor
- Kernel: 7.2.2-1-cachyos
- Go: go1.27.1-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
- GOMAXPROCS: 32
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
| 256 B | 128 | 1.62M msgs/s | 705k msgs/s | 1.58M msgs/s | 662k msgs/s | 2.5 µs / 3.2 µs / 2.5 µs / 3.3 µs | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.01M msgs/s | 740k msgs/s | 1.99M msgs/s | 696k msgs/s | 2.3 µs / 3.1 µs / 2.4 µs / 3.2 µs | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.15M msgs/s | 764k msgs/s | 2.13M msgs/s | 721k msgs/s | 2.4 µs / 3.1 µs / 2.3 µs / 3.2 µs | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.45M msgs/s | 643k msgs/s | 1.44M msgs/s | 604k msgs/s | 2.9 µs / 3.5 µs / 2.9 µs / 3.7 µs | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.76M msgs/s | 688k msgs/s | 1.78M msgs/s | 647k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.89M msgs/s | 704k msgs/s | 1.91M msgs/s | 659k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 749k msgs/s | 176k msgs/s | 772k msgs/s | 172k msgs/s | 6.0 µs / 7.5 µs / 5.9 µs / 7.8 µs | 2 (72 KB) / 2 (73 KB) / 259 (9 KB) / 21 (167 KB) |
| 64 KiB | 512 | 996k msgs/s | 179k msgs/s | 1000k msgs/s | 181k msgs/s | 5.2 µs / 7.3 µs / 5.1 µs / 7.4 µs | 2 (72 KB) / 2 (72 KB) / 1027 (37 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 415k msgs/s | 180k msgs/s | 415k msgs/s | 181k msgs/s | 14.7 µs / 7.4 µs / 14.6 µs / 7.4 µs | 2 (72 KB) / 2 (72 KB) / 4099 (146 KB) / 21 (165 KB) |
| 256 KiB | 128 | 322k msgs/s | 75k msgs/s | 320k msgs/s | 74k msgs/s | 16.7 µs / 21.0 µs / 16.8 µs / 21.9 µs | 2 (271 KB) / 2 (285 KB) / 261 (281 KB) / 22 (620 KB) |
| 256 KiB | 512 | 189k msgs/s | 73k msgs/s | 190k msgs/s | 72k msgs/s | 34.4 µs / 21.3 µs / 34.3 µs / 21.7 µs | 2 (266 KB) / 2 (265 KB) / 1029 (304 KB) / 21 (568 KB) |
| 256 KiB | 2048 | 77k msgs/s | 72k msgs/s | 77k msgs/s | 69k msgs/s | 93.4 µs / 21.1 µs / 93.0 µs / 22.3 µs | 2 (264 KB) / 2 (270 KB) / 4101 (408 KB) / 21 (561 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | CPU/msg ews / ews-sync / gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.25M msgs/s | 701k msgs/s | 1.18M msgs/s | 3.5 µs / 3.8 µs / 3.9 µs | 4 / 4 / 259 (9 KB) |
| 256 B | 512 | 1.55M msgs/s | 752k msgs/s | 1.45M msgs/s | 3.3 µs / 3.8 µs / 3.7 µs | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.44M msgs/s | 704k msgs/s | 846k msgs/s | 4.0 µs / 3.9 µs / 7.4 µs | 4 (2 KB) / 4 (11 KB) / 4099 (144 KB) |
| 4 KiB | 128 | 1.03M msgs/s | 691k msgs/s | 983k msgs/s | 4.7 µs / 5.0 µs / 5.1 µs | 4 (5 KB) / 4 (4 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.24M msgs/s | 754k msgs/s | 1.16M msgs/s | 4.5 µs / 4.9 µs / 4.9 µs | 4 (5 KB) / 4 (8 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.08M msgs/s | 697k msgs/s | 706k msgs/s | 5.9 µs / 5.5 µs / 9.2 µs | 4 (4 KB) / 4 (12 KB) / 4099 (144 KB) |
| 64 KiB | 128 | 604k msgs/s | 620k msgs/s | 592k msgs/s | 9.6 µs / 9.3 µs / 10.0 µs | 4 (74 KB) / 4 (75 KB) / 259 (9 KB) |
| 64 KiB | 512 | 701k msgs/s | 705k msgs/s | 680k msgs/s | 9.3 µs / 8.9 µs / 9.8 µs | 4 (74 KB) / 4 (76 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 682k msgs/s | 649k msgs/s | 468k msgs/s | 10.0 µs / 9.4 µs / 14.5 µs | 4 (96 KB) / 4 (80 KB) / 4099 (144 KB) |
| 256 KiB | 128 | 255k msgs/s | 241k msgs/s | 257k msgs/s | 25.7 µs / 25.5 µs / 25.9 µs | 6 (420 KB) / 8 (536 KB) / 261 (319 KB) |
| 256 KiB | 512 | 287k msgs/s | 286k msgs/s | 284k msgs/s | 25.0 µs / 24.5 µs / 25.4 µs | 6 (410 KB) / 4 (323 KB) / 1029 (319 KB) |
| 256 KiB | 2048 | 286k msgs/s | 286k msgs/s | 240k msgs/s | 25.6 µs / 25.2 µs / 30.0 µs | 4 (265 KB) / 4 (265 KB) / 4106 (785 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | CPU/msg ews / ews-sync / gws / gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|---|
| 256 B | 128 | 1.41M msgs/s | 686k msgs/s | 1.57M msgs/s | 643k msgs/s | 2.9 µs / 3.3 µs / 2.5 µs / 3.5 µs | 4 / 4 / 259 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.78M msgs/s | 733k msgs/s | 2.00M msgs/s | 694k msgs/s | 2.7 µs / 3.2 µs / 2.4 µs / 3.4 µs | 4 / 4 (4 KB) / 1027 (36 KB) / 18 (17 KB) |
| 256 B | 2048 | 1.94M msgs/s | 770k msgs/s | 2.15M msgs/s | 726k msgs/s | 2.7 µs / 3.1 µs / 2.4 µs / 3.3 µs | 4 / 4 (9 KB) / 4099 (144 KB) / 18 (22 KB) |
| 4 KiB | 128 | 1.14M msgs/s | 668k msgs/s | 1.14M msgs/s | 629k msgs/s | 4.1 µs / 4.5 µs / 4.1 µs / 4.7 µs | 4 (4 KB) / 4 (5 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.37M msgs/s | 734k msgs/s | 1.37M msgs/s | 694k msgs/s | 3.9 µs / 4.4 µs / 3.9 µs / 4.5 µs | 4 (4 KB) / 4 (7 KB) / 1027 (37 KB) / 18 (19 KB) |
| 4 KiB | 2048 | 1.49M msgs/s | 769k msgs/s | 1.48M msgs/s | 717k msgs/s | 3.9 µs / 4.2 µs / 3.9 µs / 4.4 µs | 4 (5 KB) / 4 (12 KB) / 4099 (145 KB) / 18 (23 KB) |
| 64 KiB | 128 | 640k msgs/s | 620k msgs/s | 652k msgs/s | 582k msgs/s | 8.9 µs / 8.7 µs / 8.8 µs / 8.9 µs | 4 (76 KB) / 4 (75 KB) / 259 (9 KB) / 21 (94 KB) |
| 64 KiB | 512 | 746k msgs/s | 704k msgs/s | 756k msgs/s | 677k msgs/s | 8.6 µs / 8.3 µs / 8.5 µs / 8.4 µs | 4 (85 KB) / 4 (93 KB) / 1027 (38 KB) / 22 (124 KB) |
| 64 KiB | 2048 | 798k msgs/s | 743k msgs/s | 787k msgs/s | 711k msgs/s | 8.5 µs / 8.1 µs / 8.5 µs / 8.2 µs | 4 (79 KB) / 4 (89 KB) / 4099 (149 KB) / 21 (95 KB) |
| 256 KiB | 128 | 260k msgs/s | 248k msgs/s | 266k msgs/s | 238k msgs/s | 25.0 µs / 24.8 µs / 24.8 µs / 25.3 µs | 6 (451 KB) / 8 (537 KB) / 262 (397 KB) / 27 (707 KB) |
| 256 KiB | 512 | 292k msgs/s | 286k msgs/s | 294k msgs/s | 280k msgs/s | 24.4 µs / 24.0 µs / 24.4 µs / 24.2 µs | 11 (805 KB) / 8 (545 KB) / 1038 (981 KB) / 30 (956 KB) |
| 256 KiB | 2048 | 297k msgs/s | 308k msgs/s | 305k msgs/s | 303k msgs/s | 24.5 µs / 23.7 µs / 24.1 µs / 23.8 µs | 44 (3286 KB) / 4 (273 KB) / 4101 (456 KB) / 33 (1225 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.
- The CPU column separates what the throughput tie hides. Below the bandwidth ceiling the asynchronous paths cost a fifth less CPU per delivery than the synchronous ones. At the ceiling, 64 KiB and 256 KiB to 2048 clients, they cost two to four times more for the same throughput: two thousand writers copying at once turn memory stalls into CPU time, where one goroutine writing in turn does not. With takeover, gws's per-recipient window copy shows as up to 1.8 times ews's CPU per message at 2048 clients.

