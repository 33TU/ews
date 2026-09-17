# Broadcast benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `5ae144b`.

![broadcast-plain](broadcast-plain.svg)

![broadcast-compressed](broadcast-compressed.svg)

![broadcast-nocontext](broadcast-nocontext.svg)

## Setup

- CPU: AMD Ryzen 9 9950X3D 16-Core Processor
- Kernel: 7.2.2-1-cachyos
- Go: go1.27.1-X:nodwarf5, default build: SWAR masking and the shift-based UTF-8 validator
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
| 256 B | 128 | 1.62M msgs/s | 706k msgs/s | 1.59M msgs/s | 666k msgs/s | 2.5 µs / 3.1 µs / 2.5 µs / 3.3 µs | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.02M msgs/s | 743k msgs/s | 2.01M msgs/s | 698k msgs/s | 2.4 µs / 3.1 µs / 2.4 µs / 3.2 µs | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.17M msgs/s | 764k msgs/s | 2.16M msgs/s | 723k msgs/s | 2.4 µs / 3.1 µs / 2.3 µs / 3.2 µs | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.47M msgs/s | 649k msgs/s | 1.46M msgs/s | 608k msgs/s | 2.9 µs / 3.5 µs / 2.9 µs / 3.7 µs | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.79M msgs/s | 693k msgs/s | 1.80M msgs/s | 651k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.91M msgs/s | 701k msgs/s | 1.92M msgs/s | 659k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 748k msgs/s | 179k msgs/s | 771k msgs/s | 175k msgs/s | 6.0 µs / 7.4 µs / 5.9 µs / 7.7 µs | 2 (72 KB) / 2 (73 KB) / 259 (9 KB) / 21 (167 KB) |
| 64 KiB | 512 | 991k msgs/s | 191k msgs/s | 981k msgs/s | 194k msgs/s | 5.2 µs / 7.1 µs / 5.2 µs / 7.2 µs | 2 (72 KB) / 2 (73 KB) / 1027 (37 KB) / 21 (165 KB) |
| 64 KiB | 2048 | 415k msgs/s | 177k msgs/s | 416k msgs/s | 178k msgs/s | 14.6 µs / 7.3 µs / 14.7 µs / 7.4 µs | 2 (72 KB) / 2 (72 KB) / 4099 (144 KB) / 21 (164 KB) |
| 256 KiB | 128 | 323k msgs/s | 75k msgs/s | 319k msgs/s | 74k msgs/s | 16.7 µs / 20.9 µs / 16.9 µs / 21.9 µs | 2 (272 KB) / 2 (291 KB) / 261 (283 KB) / 22 (619 KB) |
| 256 KiB | 512 | 192k msgs/s | 71k msgs/s | 192k msgs/s | 71k msgs/s | 33.8 µs / 21.7 µs / 33.9 µs / 22.1 µs | 2 (266 KB) / 2 (276 KB) / 1029 (309 KB) / 21 (572 KB) |
| 256 KiB | 2048 | 77k msgs/s | 68k msgs/s | 77k msgs/s | 69k msgs/s | 94.0 µs / 22.4 µs / 93.8 µs / 22.2 µs | 2 (270 KB) / 2 (270 KB) / 4101 (414 KB) / 21 (548 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | CPU/msg ews / ews-sync / gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.26M msgs/s | 705k msgs/s | 1.17M msgs/s | 3.5 µs / 3.8 µs / 3.9 µs | 4 / 4 / 259 (9 KB) |
| 256 B | 512 | 1.54M msgs/s | 755k msgs/s | 1.44M msgs/s | 3.3 µs / 3.8 µs / 3.7 µs | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.45M msgs/s | 707k msgs/s | 856k msgs/s | 4.0 µs / 3.9 µs / 7.2 µs | 4 (1 KB) / 4 (11 KB) / 4099 (144 KB) |
| 4 KiB | 128 | 1.03M msgs/s | 698k msgs/s | 991k msgs/s | 4.7 µs / 4.8 µs / 5.1 µs | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.23M msgs/s | 756k msgs/s | 1.17M msgs/s | 4.6 µs / 4.9 µs / 4.9 µs | 4 (4 KB) / 4 (7 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.08M msgs/s | 696k msgs/s | 688k msgs/s | 5.8 µs / 5.6 µs / 9.3 µs | 4 (4 KB) / 4 (14 KB) / 4099 (144 KB) |
| 64 KiB | 128 | 603k msgs/s | 619k msgs/s | 592k msgs/s | 9.6 µs / 9.3 µs / 10.0 µs | 4 (75 KB) / 4 (75 KB) / 259 (9 KB) |
| 64 KiB | 512 | 711k msgs/s | 699k msgs/s | 681k msgs/s | 9.3 µs / 8.9 µs / 9.8 µs | 4 (74 KB) / 4 (76 KB) / 1027 (36 KB) |
| 64 KiB | 2048 | 686k msgs/s | 660k msgs/s | 461k msgs/s | 9.9 µs / 9.3 µs / 14.5 µs | 4 (74 KB) / 4 (76 KB) / 4099 (145 KB) |
| 256 KiB | 128 | 254k msgs/s | 245k msgs/s | 256k msgs/s | 25.7 µs / 25.4 µs / 25.9 µs | 5 (375 KB) / 5 (385 KB) / 261 (307 KB) |
| 256 KiB | 512 | 288k msgs/s | 285k msgs/s | 284k msgs/s | 25.1 µs / 24.5 µs / 25.4 µs | 5 (383 KB) / 5 (337 KB) / 1029 (326 KB) |
| 256 KiB | 2048 | 284k msgs/s | 287k msgs/s | 242k msgs/s | 25.6 µs / 25.0 µs / 29.5 µs | 4 (265 KB) / 4 (274 KB) / 4101 (408 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | CPU/msg ews / ews-sync / gws / gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|---|
| 256 B | 128 | 1.40M msgs/s | 685k msgs/s | 1.59M msgs/s | 640k msgs/s | 2.9 µs / 3.3 µs / 2.5 µs / 3.5 µs | 4 / 4 / 259 (9 KB) / 18 (11 KB) |
| 256 B | 512 | 1.78M msgs/s | 740k msgs/s | 1.99M msgs/s | 696k msgs/s | 2.7 µs / 3.2 µs / 2.4 µs / 3.4 µs | 4 / 4 (5 KB) / 1027 (36 KB) / 18 (16 KB) |
| 256 B | 2048 | 1.93M msgs/s | 778k msgs/s | 2.14M msgs/s | 725k msgs/s | 2.7 µs / 3.1 µs / 2.4 µs / 3.3 µs | 4 (1 KB) / 4 (9 KB) / 4099 (144 KB) / 18 (20 KB) |
| 4 KiB | 128 | 1.14M msgs/s | 670k msgs/s | 1.15M msgs/s | 629k msgs/s | 4.1 µs / 4.6 µs / 4.1 µs / 4.7 µs | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.38M msgs/s | 734k msgs/s | 1.35M msgs/s | 695k msgs/s | 3.9 µs / 4.4 µs / 4.0 µs / 4.5 µs | 4 (5 KB) / 4 (7 KB) / 1027 (36 KB) / 18 (19 KB) |
| 4 KiB | 2048 | 1.48M msgs/s | 762k msgs/s | 1.48M msgs/s | 719k msgs/s | 3.9 µs / 4.3 µs / 4.0 µs / 4.4 µs | 4 (6 KB) / 4 (13 KB) / 4099 (145 KB) / 18 (25 KB) |
| 64 KiB | 128 | 638k msgs/s | 621k msgs/s | 654k msgs/s | 593k msgs/s | 8.9 µs / 8.6 µs / 8.7 µs / 8.8 µs | 4 (76 KB) / 4 (78 KB) / 259 (9 KB) / 21 (92 KB) |
| 64 KiB | 512 | 749k msgs/s | 702k msgs/s | 748k msgs/s | 677k msgs/s | 8.6 µs / 8.3 µs / 8.6 µs / 8.4 µs | 4 (77 KB) / 4 (76 KB) / 1027 (37 KB) / 21 (93 KB) |
| 64 KiB | 2048 | 792k msgs/s | 741k msgs/s | 800k msgs/s | 707k msgs/s | 8.5 µs / 8.1 µs / 8.5 µs / 8.2 µs | 4 (75 KB) / 4 (83 KB) / 4099 (147 KB) / 21 (98 KB) |
| 256 KiB | 128 | 259k msgs/s | 249k msgs/s | 264k msgs/s | 238k msgs/s | 25.0 µs / 24.8 µs / 24.8 µs / 25.2 µs | 6 (452 KB) / 8 (526 KB) / 262 (374 KB) / 26 (674 KB) |
| 256 KiB | 512 | 293k msgs/s | 290k msgs/s | 297k msgs/s | 284k msgs/s | 24.5 µs / 23.9 µs / 24.2 µs / 24.1 µs | 8 (600 KB) / 6 (400 KB) / 1030 (402 KB) / 25 (578 KB) |
| 256 KiB | 2048 | 308k msgs/s | 308k msgs/s | 307k msgs/s | 307k msgs/s | 23.9 µs / 23.7 µs / 24.0 µs / 23.7 µs | 4 (265 KB) / 5 (360 KB) / 4103 (593 KB) / 21 (323 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.
- The CPU column separates what the throughput tie hides. Below the bandwidth ceiling the asynchronous paths cost a fifth less CPU per delivery than the synchronous ones. At the ceiling, 64 KiB and 256 KiB to 2048 clients, they cost two to four times more for the same throughput: two thousand writers copying at once turn memory stalls into CPU time, where one goroutine writing in turn does not. With takeover, gws's per-recipient window copy shows as up to 1.8 times ews's CPU per message at 2048 clients.

