# Broadcast benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `8494966`.

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
| 256 B | 128 | 1.63M msgs/s | 707k msgs/s | 1.60M msgs/s | 665k msgs/s | 2.5 µs / 3.1 µs / 2.5 µs / 3.3 µs | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.02M msgs/s | 741k msgs/s | 2.00M msgs/s | 698k msgs/s | 2.4 µs / 3.1 µs / 2.4 µs / 3.2 µs | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.16M msgs/s | 768k msgs/s | 2.16M msgs/s | 720k msgs/s | 2.4 µs / 3.0 µs / 2.4 µs / 3.2 µs | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 256 B | 8192 | 1.67M msgs/s | 668k msgs/s | 1.70M msgs/s | 620k msgs/s | 3.1 µs / 3.4 µs / 3.1 µs / 3.5 µs | 5 (1 KB) / 2 / 16391 (579 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.46M msgs/s | 645k msgs/s | 1.44M msgs/s | 605k msgs/s | 2.9 µs / 3.5 µs / 2.9 µs / 3.7 µs | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.76M msgs/s | 690k msgs/s | 1.77M msgs/s | 649k msgs/s | 2.7 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.90M msgs/s | 699k msgs/s | 1.89M msgs/s | 655k msgs/s | 2.8 µs / 3.4 µs / 2.8 µs / 3.6 µs | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 4 KiB | 8192 | 1.12M msgs/s | 546k msgs/s | 1.17M msgs/s | 506k msgs/s | 4.9 µs / 4.2 µs / 4.7 µs / 4.4 µs | 2 (4 KB) / 2 (4 KB) / 16387 (576 KB) / 15 (20 KB) |
| 64 KiB | 128 | 744k msgs/s | 176k msgs/s | 758k msgs/s | 172k msgs/s | 6.1 µs / 7.5 µs / 5.9 µs / 8.0 µs | 2 (72 KB) / 2 (72 KB) / 259 (9 KB) / 21 (165 KB) |
| 64 KiB | 512 | 992k msgs/s | 189k msgs/s | 996k msgs/s | 175k msgs/s | 5.2 µs / 7.1 µs / 5.2 µs / 7.5 µs | 2 (72 KB) / 2 (72 KB) / 1027 (37 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 418k msgs/s | 182k msgs/s | 415k msgs/s | 181k msgs/s | 14.4 µs / 7.3 µs / 14.7 µs / 7.3 µs | 2 (72 KB) / 2 (72 KB) / 4099 (146 KB) / 21 (164 KB) |
| 64 KiB | 8192 | 263k msgs/s | 172k msgs/s | 266k msgs/s | 171k msgs/s | 24.9 µs / 7.8 µs / 24.8 µs / 7.8 µs | 2 (73 KB) / 2 (72 KB) / 16387 (583 KB) / 21 (164 KB) |
| 256 KiB | 128 | 319k msgs/s | 75k msgs/s | 318k msgs/s | 74k msgs/s | 17.0 µs / 21.3 µs / 17.1 µs / 21.9 µs | 2 (268 KB) / 2 (278 KB) / 261 (277 KB) / 21 (580 KB) |
| 256 KiB | 512 | 194k msgs/s | 72k msgs/s | 199k msgs/s | 70k msgs/s | 33.2 µs / 21.4 µs / 32.5 µs / 22.2 µs | 2 (265 KB) / 2 (264 KB) / 1029 (301 KB) / 21 (570 KB) |
| 256 KiB | 2048 | 77k msgs/s | 68k msgs/s | 76k msgs/s | 67k msgs/s | 93.1 µs / 22.4 µs / 92.1 µs / 23.1 µs | 2 (270 KB) / 2 (277 KB) / 4101 (408 KB) / 21 (562 KB) |
| 256 KiB | 8192 | 71k msgs/s | 68k msgs/s | 71k msgs/s | 66k msgs/s | 98.9 µs / 22.4 µs / 96.5 µs / 22.8 µs | 2 (264 KB) / 2 (321 KB) / 16389 (840 KB) / 21 (548 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | CPU/msg ews / ews-sync / gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.25M msgs/s | 699k msgs/s | 1.17M msgs/s | 3.5 µs / 3.9 µs / 3.9 µs | 4 / 4 / 259 (9 KB) |
| 256 B | 512 | 1.56M msgs/s | 753k msgs/s | 1.43M msgs/s | 3.3 µs / 3.8 µs / 3.7 µs | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.42M msgs/s | 692k msgs/s | 845k msgs/s | 4.1 µs / 4.0 µs / 7.4 µs | 4 (2 KB) / 4 (9 KB) / 4099 (144 KB) |
| 256 B | 8192 | 1.02M msgs/s | 441k msgs/s | 790k msgs/s | 5.9 µs / 5.2 µs / 7.8 µs | 4 (15 KB) / 4 (21 KB) / 16387 (576 KB) |
| 4 KiB | 128 | 1.02M msgs/s | 692k msgs/s | 984k msgs/s | 4.8 µs / 5.0 µs / 5.1 µs | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.22M msgs/s | 752k msgs/s | 1.16M msgs/s | 4.6 µs / 5.0 µs / 5.0 µs | 4 (4 KB) / 4 (7 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.07M msgs/s | 689k msgs/s | 695k msgs/s | 5.9 µs / 5.6 µs / 9.3 µs | 4 (6 KB) / 4 (14 KB) / 4099 (144 KB) |
| 4 KiB | 8192 | 749k msgs/s | 396k msgs/s | 437k msgs/s | 8.6 µs / 7.3 µs / 14.7 µs | 4 (4 KB) / 4 (18 KB) / 16387 (576 KB) |
| 64 KiB | 128 | 605k msgs/s | 610k msgs/s | 586k msgs/s | 9.7 µs / 9.3 µs / 10.1 µs | 4 (74 KB) / 4 (74 KB) / 259 (9 KB) |
| 64 KiB | 512 | 695k msgs/s | 706k msgs/s | 674k msgs/s | 9.4 µs / 9.0 µs / 9.8 µs | 4 (73 KB) / 4 (75 KB) / 1027 (36 KB) |
| 64 KiB | 2048 | 673k msgs/s | 651k msgs/s | 465k msgs/s | 10.1 µs / 9.4 µs / 14.7 µs | 4 (74 KB) / 4 (89 KB) / 4099 (144 KB) |
| 64 KiB | 8192 | 502k msgs/s | 367k msgs/s | 345k msgs/s | 13.1 µs / 12.3 µs / 20.1 µs | 4 (107 KB) / 4 (87 KB) / 16387 (578 KB) |
| 256 KiB | 128 | 252k msgs/s | 248k msgs/s | 256k msgs/s | 25.9 µs / 25.4 µs / 25.9 µs | 6 (392 KB) / 5 (324 KB) / 261 (291 KB) |
| 256 KiB | 512 | 282k msgs/s | 282k msgs/s | 282k msgs/s | 25.2 µs / 24.6 µs / 25.5 µs | 7 (524 KB) / 4 (308 KB) / 1029 (342 KB) |
| 256 KiB | 2048 | 279k msgs/s | 288k msgs/s | 238k msgs/s | 25.8 µs / 25.2 µs / 30.3 µs | 4 (265 KB) / 4 (269 KB) / 4104 (640 KB) |
| 256 KiB | 8192 | 256k msgs/s | 272k msgs/s | 213k msgs/s | 27.8 µs / 27.2 µs / 34.2 µs | 4 (308 KB) / 4 (305 KB) / 16389 (840 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | CPU/msg ews / ews-sync / gws / gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|---|
| 256 B | 128 | 1.43M msgs/s | 687k msgs/s | 1.41M msgs/s | 644k msgs/s | 2.9 µs / 3.3 µs / 2.9 µs / 3.5 µs | 4 / 4 / 259 (9 KB) / 18 (11 KB) |
| 256 B | 512 | 1.75M msgs/s | 742k msgs/s | 1.75M msgs/s | 695k msgs/s | 2.8 µs / 3.2 µs / 2.8 µs / 3.4 µs | 4 / 4 (3 KB) / 1027 (36 KB) / 18 (11 KB) |
| 256 B | 2048 | 1.93M msgs/s | 780k msgs/s | 1.91M msgs/s | 724k msgs/s | 2.8 µs / 3.0 µs / 2.8 µs / 3.3 µs | 4 (1 KB) / 4 (9 KB) / 4099 (144 KB) / 18 (18 KB) |
| 256 B | 8192 | 1.61M msgs/s | 672k msgs/s | 1.59M msgs/s | 632k msgs/s | 3.4 µs / 3.5 µs / 3.4 µs / 3.7 µs | 4 (7 KB) / 4 (25 KB) / 16387 (576 KB) / 18 (20 KB) |
| 4 KiB | 128 | 1.14M msgs/s | 671k msgs/s | 1.15M msgs/s | 630k msgs/s | 4.1 µs / 4.6 µs / 4.1 µs / 4.7 µs | 4 (4 KB) / 4 (5 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.37M msgs/s | 740k msgs/s | 1.37M msgs/s | 698k msgs/s | 4.0 µs / 4.4 µs / 4.0 µs / 4.5 µs | 4 (5 KB) / 4 (5 KB) / 1027 (36 KB) / 18 (21 KB) |
| 4 KiB | 2048 | 1.48M msgs/s | 772k msgs/s | 1.46M msgs/s | 721k msgs/s | 4.0 µs / 4.2 µs / 4.0 µs / 4.4 µs | 4 (4 KB) / 4 (13 KB) / 4099 (145 KB) / 18 (25 KB) |
| 4 KiB | 8192 | 1.27M msgs/s | 664k msgs/s | 1.26M msgs/s | 621k msgs/s | 4.7 µs / 4.8 µs / 4.6 µs / 4.9 µs | 4 (4 KB) / 4 (13 KB) / 16387 (576 KB) / 18 (15 KB) |
| 64 KiB | 128 | 634k msgs/s | 618k msgs/s | 657k msgs/s | 586k msgs/s | 8.9 µs / 8.7 µs / 8.7 µs / 8.9 µs | 4 (75 KB) / 4 (75 KB) / 259 (9 KB) / 21 (93 KB) |
| 64 KiB | 512 | 747k msgs/s | 702k msgs/s | 748k msgs/s | 673k msgs/s | 8.6 µs / 8.3 µs / 8.6 µs / 8.4 µs | 4 (74 KB) / 4 (74 KB) / 1027 (37 KB) / 21 (96 KB) |
| 64 KiB | 2048 | 789k msgs/s | 744k msgs/s | 789k msgs/s | 711k msgs/s | 8.6 µs / 8.1 µs / 8.6 µs / 8.2 µs | 4 (80 KB) / 4 (81 KB) / 4099 (151 KB) / 21 (93 KB) |
| 64 KiB | 8192 | 704k msgs/s | 604k msgs/s | 692k msgs/s | 544k msgs/s | 9.4 µs / 8.7 µs / 9.4 µs / 9.1 µs | 4 (80 KB) / 4 (72 KB) / 16387 (576 KB) / 21 (97 KB) |
| 256 KiB | 128 | 260k msgs/s | 248k msgs/s | 264k msgs/s | 247k msgs/s | 25.2 µs / 24.9 µs / 25.0 µs / 25.0 µs | 5 (346 KB) / 5 (378 KB) / 262 (369 KB) / 22 (343 KB) |
| 256 KiB | 512 | 295k msgs/s | 291k msgs/s | 295k msgs/s | 290k msgs/s | 24.4 µs / 23.9 µs / 24.4 µs / 24.0 µs | 5 (337 KB) / 4 (292 KB) / 1032 (542 KB) / 22 (353 KB) |
| 256 KiB | 2048 | 301k msgs/s | 307k msgs/s | 299k msgs/s | 305k msgs/s | 24.1 µs / 23.6 µs / 24.4 µs / 23.7 µs | 4 (284 KB) / 5 (402 KB) / 4133 (2802 KB) / 23 (462 KB) |
| 256 KiB | 8192 | 292k msgs/s | 306k msgs/s | 292k msgs/s | 306k msgs/s | 25.2 µs / 24.3 µs / 25.1 µs / 24.5 µs | 4 (265 KB) / 4 (265 KB) / 16389 (840 KB) / 21 (298 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.
- The CPU column separates what the throughput tie hides. Below the bandwidth ceiling the asynchronous paths cost a fifth less CPU per delivery than the synchronous ones. At the ceiling, 64 KiB and 256 KiB to 2048 clients and up, they cost two to four times more for the same throughput: thousands of writers copying at once turn memory stalls into CPU time, where one goroutine writing in turn does not. With takeover, gws's per-recipient window copy shows from 2048 clients on: at 8192 gws delivers 60 to 80 percent of ews's messages per second on 1.2 to 1.7 times the CPU per message, widest at 4 KiB.

