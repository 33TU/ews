# Broadcast benchmark results

Run at ews commit `5387555` with `go test -run '^$' -bench Broadcast -benchtime 1s`; tables and charts generated from the saved output by `go run ../cmd/results` on 2026-09-26.

![broadcast-plain](broadcast-plain.svg)

![broadcast-compressed](broadcast-compressed.svg)

![broadcast-nocontext](broadcast-nocontext.svg)

## Setup

- CPU: AMD Ryzen 9 9950X3D 16-Core Processor
- Kernel: 7.2.2-1-cachyos
- Go: go1.27.1-X:nodwarf5, default build: SWAR masking and the shift-based UTF-8 validator
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
| 256 B | 128 | 1.64M msgs/s | 709k msgs/s | 1.62M msgs/s | 667k msgs/s | 2.5 µs / 3.1 µs / 2.5 µs / 3.2 µs | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.04M msgs/s | 738k msgs/s | 2.01M msgs/s | 697k msgs/s | 2.3 µs / 3.1 µs / 2.4 µs / 3.2 µs | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.12M msgs/s | 763k msgs/s | 2.14M msgs/s | 720k msgs/s | 2.3 µs / 3.1 µs / 2.3 µs / 3.2 µs | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 256 B | 8192 | 1.66M msgs/s | 673k msgs/s | 1.66M msgs/s | 627k msgs/s | 3.2 µs / 3.4 µs / 3.1 µs / 3.5 µs | 2 / 2 / 16391 (579 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.47M msgs/s | 650k msgs/s | 1.47M msgs/s | 608k msgs/s | 2.9 µs / 3.5 µs / 2.9 µs / 3.7 µs | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.79M msgs/s | 688k msgs/s | 1.80M msgs/s | 641k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.87M msgs/s | 693k msgs/s | 1.89M msgs/s | 650k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 4 KiB | 8192 | 1.09M msgs/s | 549k msgs/s | 1.13M msgs/s | 507k msgs/s | 5.1 µs / 4.2 µs / 4.9 µs / 4.4 µs | 2 (4 KB) / 2 (4 KB) / 16387 (576 KB) / 15 (20 KB) |
| 64 KiB | 128 | 767k msgs/s | 174k msgs/s | 778k msgs/s | 171k msgs/s | 6.0 µs / 7.6 µs / 5.9 µs / 8.0 µs | 2 (72 KB) / 2 (72 KB) / 259 (9 KB) / 21 (164 KB) |
| 64 KiB | 512 | 1.02M msgs/s | 181k msgs/s | 1.01M msgs/s | 176k msgs/s | 5.1 µs / 7.3 µs / 5.1 µs / 7.5 µs | 2 (72 KB) / 2 (72 KB) / 1027 (36 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 417k msgs/s | 191k msgs/s | 419k msgs/s | 187k msgs/s | 14.6 µs / 7.1 µs / 14.8 µs / 7.3 µs | 2 (72 KB) / 2 (72 KB) / 4099 (149 KB) / 21 (164 KB) |
| 64 KiB | 8192 | 265k msgs/s | 178k msgs/s | 270k msgs/s | 175k msgs/s | 25.2 µs / 7.7 µs / 25.2 µs / 7.8 µs | 2 (73 KB) / 2 (72 KB) / 16387 (576 KB) / 21 (164 KB) |
| 256 KiB | 128 | 323k msgs/s | 74k msgs/s | 321k msgs/s | 74k msgs/s | 16.9 µs / 21.3 µs / 17.1 µs / 21.9 µs | 2 (268 KB) / 2 (275 KB) / 261 (277 KB) / 21 (603 KB) |
| 256 KiB | 512 | 198k msgs/s | 74k msgs/s | 196k msgs/s | 73k msgs/s | 33.0 µs / 21.0 µs / 33.2 µs / 21.3 µs | 2 (266 KB) / 2 (267 KB) / 1029 (304 KB) / 21 (572 KB) |
| 256 KiB | 2048 | 77k msgs/s | 73k msgs/s | 77k msgs/s | 67k msgs/s | 93.1 µs / 21.0 µs / 93.1 µs / 23.2 µs | 2 (264 KB) / 2 (275 KB) / 4101 (460 KB) / 21 (548 KB) |
| 256 KiB | 8192 | 71k msgs/s | 66k msgs/s | 71k msgs/s | 65k msgs/s | 99.1 µs / 22.6 µs / 98.9 µs / 22.9 µs | 2 (292 KB) / 2 (264 KB) / 16398 (845 KB) / 21 (548 KB) |
| 2 MiB | 128 | 32k msgs/s | 10k msgs/s | 35k msgs/s | 9k msgs/s | 207.6 µs / 160.7 µs / 186.5 µs / 172.4 µs | 3 (2262 KB) / 3 (2492 KB) / 262 (2234 KB) / 24 (4960 KB) |
| 6 MiB | 128 | 3k msgs/s | 3k msgs/s | 3k msgs/s | 3k msgs/s | 2541.5 µs / 469.1 µs / 2832.1 µs / 457.3 µs | 2 (6152 KB) / 4 (8002 KB) / 262 (6162 KB) / 21 (12324 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | CPU/msg ews / ews-sync / gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.30M msgs/s | 713k msgs/s | 1.20M msgs/s | 3.4 µs / 3.6 µs / 3.8 µs | 4 / 4 / 259 (9 KB) |
| 256 B | 512 | 1.58M msgs/s | 758k msgs/s | 1.48M msgs/s | 3.2 µs / 3.6 µs / 3.6 µs | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.44M msgs/s | 700k msgs/s | 848k msgs/s | 4.0 µs / 3.9 µs / 7.4 µs | 4 (2 KB) / 4 (11 KB) / 4099 (144 KB) |
| 256 B | 8192 | 1.02M msgs/s | 468k msgs/s | 789k msgs/s | 5.9 µs / 4.9 µs / 7.9 µs | 4 (10 KB) / 4 (51 KB) / 16387 (576 KB) |
| 4 KiB | 128 | 1.07M msgs/s | 705k msgs/s | 1.02M msgs/s | 4.6 µs / 4.7 µs / 5.0 µs | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.28M msgs/s | 771k msgs/s | 1.20M msgs/s | 4.4 µs / 4.5 µs / 4.8 µs | 4 (5 KB) / 4 (7 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.09M msgs/s | 703k msgs/s | 702k msgs/s | 5.8 µs / 5.4 µs / 9.2 µs | 4 (6 KB) / 4 (12 KB) / 4099 (144 KB) |
| 4 KiB | 8192 | 747k msgs/s | 407k msgs/s | 437k msgs/s | 8.4 µs / 7.1 µs / 15.0 µs | 4 (4 KB) / 4 (4 KB) / 16387 (576 KB) |
| 64 KiB | 128 | 640k msgs/s | 653k msgs/s | 633k msgs/s | 9.1 µs / 8.6 µs / 9.4 µs | 4 (75 KB) / 4 (72 KB) / 259 (9 KB) |
| 64 KiB | 512 | 739k msgs/s | 705k msgs/s | 712k msgs/s | 8.8 µs / 8.4 µs / 9.2 µs | 4 (73 KB) / 4 (75 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 711k msgs/s | 676k msgs/s | 476k msgs/s | 9.6 µs / 8.8 µs / 14.3 µs | 4 (72 KB) / 4 (76 KB) / 4099 (154 KB) |
| 64 KiB | 8192 | 514k msgs/s | 380k msgs/s | 348k msgs/s | 12.8 µs / 11.6 µs / 19.6 µs | 4 (72 KB) / 4 (72 KB) / 16387 (576 KB) |
| 256 KiB | 128 | 285k msgs/s | 271k msgs/s | 277k msgs/s | 23.1 µs / 23.4 µs / 23.9 µs | 4 (265 KB) / 4 (302 KB) / 261 (292 KB) |
| 256 KiB | 512 | 310k msgs/s | 303k msgs/s | 305k msgs/s | 23.0 µs / 22.7 µs / 23.5 µs | 4 (285 KB) / 4 (268 KB) / 1029 (320 KB) |
| 256 KiB | 2048 | 303k msgs/s | 309k msgs/s | 254k msgs/s | 23.7 µs / 23.2 µs / 28.3 µs | 4 (265 KB) / 4 (265 KB) / 4102 (504 KB) |
| 256 KiB | 8192 | 276k msgs/s | 290k msgs/s | 222k msgs/s | 26.1 µs / 25.2 µs / 32.8 µs | 4 (265 KB) / 4 (283 KB) / 16389 (840 KB) |
| 2 MiB | 128 | 12k msgs/s | 12k msgs/s | 12k msgs/s | 623.8 µs / 610.0 µs / 623.9 µs | 8 (2665 KB) / 8 (2935 KB) / 265 (2913 KB) |
| 6 MiB | 128 | 15k msgs/s | 14k msgs/s | 15k msgs/s | 467.7 µs / 467.0 µs / 464.0 µs | 15 (8346 KB) / 13 (9373 KB) / 263 (6640 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | CPU/msg ews / ews-sync / gws / gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|---|
| 256 B | 128 | 1.46M msgs/s | 689k msgs/s | 1.45M msgs/s | 647k msgs/s | 2.8 µs / 3.3 µs / 2.8 µs / 3.5 µs | 4 / 4 (1 KB) / 258 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.81M msgs/s | 738k msgs/s | 1.81M msgs/s | 696k msgs/s | 2.7 µs / 3.2 µs / 2.7 µs / 3.4 µs | 4 / 4 (3 KB) / 1027 (36 KB) / 18 (14 KB) |
| 256 B | 2048 | 1.97M msgs/s | 763k msgs/s | 1.97M msgs/s | 724k msgs/s | 2.6 µs / 3.1 µs / 2.7 µs / 3.3 µs | 4 (1 KB) / 4 (7 KB) / 4099 (144 KB) / 18 (20 KB) |
| 256 B | 8192 | 1.59M msgs/s | 674k msgs/s | 1.60M msgs/s | 634k msgs/s | 3.4 µs / 3.5 µs / 3.4 µs / 3.7 µs | 4 / 4 (8 KB) / 16387 (576 KB) / 18 (20 KB) |
| 4 KiB | 128 | 1.18M msgs/s | 676k msgs/s | 1.20M msgs/s | 640k msgs/s | 4.0 µs / 4.5 µs / 3.9 µs / 4.6 µs | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.42M msgs/s | 737k msgs/s | 1.43M msgs/s | 698k msgs/s | 3.8 µs / 4.3 µs / 3.8 µs / 4.5 µs | 4 (5 KB) / 4 (7 KB) / 1027 (36 KB) / 18 (20 KB) |
| 4 KiB | 2048 | 1.53M msgs/s | 765k msgs/s | 1.52M msgs/s | 723k msgs/s | 3.8 µs / 4.2 µs / 3.8 µs / 4.4 µs | 4 (4 KB) / 4 (12 KB) / 4099 (145 KB) / 18 (25 KB) |
| 4 KiB | 8192 | 1.27M msgs/s | 670k msgs/s | 1.26M msgs/s | 627k msgs/s | 4.5 µs / 4.6 µs / 4.5 µs / 4.8 µs | 4 (4 KB) / 4 (21 KB) / 16387 (576 KB) / 18 (33 KB) |
| 64 KiB | 128 | 683k msgs/s | 636k msgs/s | 701k msgs/s | 585k msgs/s | 8.3 µs / 8.3 µs / 8.2 µs / 8.6 µs | 4 (76 KB) / 4 (78 KB) / 259 (9 KB) / 21 (93 KB) |
| 64 KiB | 512 | 798k msgs/s | 719k msgs/s | 799k msgs/s | 652k msgs/s | 8.1 µs / 7.8 µs / 8.0 µs / 8.0 µs | 4 (75 KB) / 4 (76 KB) / 1027 (37 KB) / 21 (92 KB) |
| 64 KiB | 2048 | 834k msgs/s | 751k msgs/s | 834k msgs/s | 695k msgs/s | 8.0 µs / 7.6 µs / 8.0 µs / 7.7 µs | 4 (84 KB) / 4 (79 KB) / 4099 (145 KB) / 21 (91 KB) |
| 64 KiB | 8192 | 727k msgs/s | 592k msgs/s | 731k msgs/s | 544k msgs/s | 8.9 µs / 8.4 µs / 8.8 µs / 8.8 µs | 4 (72 KB) / 4 (81 KB) / 16387 (578 KB) / 21 (88 KB) |
| 256 KiB | 128 | 284k msgs/s | 276k msgs/s | 289k msgs/s | 267k msgs/s | 23.0 µs / 22.6 µs / 22.8 µs / 22.9 µs | 5 (322 KB) / 5 (319 KB) / 261 (301 KB) / 22 (336 KB) |
| 256 KiB | 512 | 321k msgs/s | 316k msgs/s | 321k msgs/s | 312k msgs/s | 22.4 µs / 21.9 µs / 22.3 µs / 22.0 µs | 5 (330 KB) / 4 (290 KB) / 1029 (322 KB) / 21 (299 KB) |
| 256 KiB | 2048 | 326k msgs/s | 330k msgs/s | 334k msgs/s | 329k msgs/s | 22.3 µs / 21.6 µs / 22.1 µs / 21.8 µs | 4 (269 KB) / 5 (326 KB) / 4102 (487 KB) / 21 (319 KB) |
| 256 KiB | 8192 | 312k msgs/s | 331k msgs/s | 314k msgs/s | 332k msgs/s | 23.4 µs / 22.4 µs / 23.1 µs / 22.4 µs | 14 (738 KB) / 4 (281 KB) / 16389 (840 KB) / 21 (297 KB) |
| 2 MiB | 128 | 13k msgs/s | 13k msgs/s | 13k msgs/s | 13k msgs/s | 564.5 µs / 564.2 µs / 566.5 µs / 557.4 µs | 10 (3000 KB) / 13 (3097 KB) / 266 (3491 KB) / 32 (3209 KB) |
| 6 MiB | 128 | 15k msgs/s | 14k msgs/s | 15k msgs/s | 14k msgs/s | 465.3 µs / 464.4 µs / 458.5 µs / 468.2 µs | 25 (9264 KB) / 14 (8285 KB) / 265 (7095 KB) / 34 (8623 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.
- The CPU column separates what the throughput tie hides. Below the bandwidth ceiling the asynchronous paths cost a fifth less CPU per delivery than the synchronous ones. At the ceiling, 64 KiB and 256 KiB to 2048 clients and up, they cost two to four times more for the same throughput: thousands of writers copying at once turn memory stalls into CPU time, where one goroutine writing in turn does not. With takeover, gws's per-recipient window copy shows from 2048 clients on: at 8192 gws delivers 60 to 80 percent of ews's messages per second on 1.2 to 1.7 times the CPU per message, widest at 4 KiB.

