# Broadcast benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `07f1229`.

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

One message of 256 bytes, 4 KiB, 64 KiB or 256 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round, so ews's few are the `Prepared` made once per round, not per recipient.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.
- `gorilla`: `NewPreparedMessage` once, then `WritePreparedMessage` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with `ews-sync`. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.62M msgs/s | 709k msgs/s | 1.60M msgs/s | 669k msgs/s | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 1.99M msgs/s | 741k msgs/s | 1.99M msgs/s | 699k msgs/s | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.15M msgs/s | 765k msgs/s | 2.15M msgs/s | 722k msgs/s | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.47M msgs/s | 647k msgs/s | 1.44M msgs/s | 610k msgs/s | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.77M msgs/s | 691k msgs/s | 1.77M msgs/s | 652k msgs/s | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.93M msgs/s | 698k msgs/s | 1.93M msgs/s | 658k msgs/s | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 759k msgs/s | 176k msgs/s | 767k msgs/s | 173k msgs/s | 2 (72 KB) / 2 (73 KB) / 259 (9 KB) / 21 (168 KB) |
| 64 KiB | 512 | 993k msgs/s | 189k msgs/s | 988k msgs/s | 191k msgs/s | 2 (72 KB) / 2 (72 KB) / 1027 (36 KB) / 21 (165 KB) |
| 64 KiB | 2048 | 413k msgs/s | 180k msgs/s | 416k msgs/s | 179k msgs/s | 2 (72 KB) / 2 (72 KB) / 4099 (146 KB) / 21 (165 KB) |
| 256 KiB | 128 | 323k msgs/s | 75k msgs/s | 320k msgs/s | 74k msgs/s | 2 (272 KB) / 2 (290 KB) / 261 (280 KB) / 22 (631 KB) |
| 256 KiB | 512 | 191k msgs/s | 71k msgs/s | 191k msgs/s | 72k msgs/s | 2 (267 KB) / 2 (271 KB) / 1029 (308 KB) / 21 (564 KB) |
| 256 KiB | 2048 | 77k msgs/s | 69k msgs/s | 77k msgs/s | 70k msgs/s | 2 (264 KB) / 2 (264 KB) / 4101 (408 KB) / 21 (548 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 1.26M msgs/s | 702k msgs/s | 1.17M msgs/s | 4 / 4 (1 KB) / 259 (9 KB) |
| 256 B | 512 | 1.55M msgs/s | 757k msgs/s | 1.45M msgs/s | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.45M msgs/s | 696k msgs/s | 859k msgs/s | 4 (1 KB) / 4 (9 KB) / 4099 (144 KB) |
| 4 KiB | 128 | 1.04M msgs/s | 706k msgs/s | 989k msgs/s | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.23M msgs/s | 761k msgs/s | 1.16M msgs/s | 4 (4 KB) / 4 (8 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.09M msgs/s | 705k msgs/s | 704k msgs/s | 4 (6 KB) / 4 (10 KB) / 4099 (144 KB) |
| 64 KiB | 128 | 603k msgs/s | 613k msgs/s | 593k msgs/s | 4 (73 KB) / 4 (74 KB) / 259 (9 KB) |
| 64 KiB | 512 | 702k msgs/s | 698k msgs/s | 676k msgs/s | 4 (74 KB) / 4 (74 KB) / 1027 (38 KB) |
| 64 KiB | 2048 | 678k msgs/s | 655k msgs/s | 466k msgs/s | 4 (90 KB) / 4 (78 KB) / 4099 (145 KB) |
| 256 KiB | 128 | 253k msgs/s | 247k msgs/s | 260k msgs/s | 6 (466 KB) / 6 (432 KB) / 261 (306 KB) |
| 256 KiB | 512 | 287k msgs/s | 283k msgs/s | 281k msgs/s | 4 (330 KB) / 5 (393 KB) / 1031 (521 KB) |
| 256 KiB | 2048 | 282k msgs/s | 290k msgs/s | 240k msgs/s | 4 (274 KB) / 4 (269 KB) / 4101 (408 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.40M msgs/s | 690k msgs/s | 1.57M msgs/s | 644k msgs/s | 4 / 4 / 259 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.76M msgs/s | 741k msgs/s | 1.95M msgs/s | 696k msgs/s | 4 / 4 (5 KB) / 1027 (36 KB) / 18 (17 KB) |
| 256 B | 2048 | 1.94M msgs/s | 772k msgs/s | 2.12M msgs/s | 721k msgs/s | 4 (1 KB) / 4 (7 KB) / 4099 (144 KB) / 18 (27 KB) |
| 4 KiB | 128 | 1.14M msgs/s | 664k msgs/s | 1.14M msgs/s | 632k msgs/s | 4 (4 KB) / 4 (4 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.39M msgs/s | 739k msgs/s | 1.37M msgs/s | 692k msgs/s | 4 (5 KB) / 4 (7 KB) / 1027 (36 KB) / 18 (20 KB) |
| 4 KiB | 2048 | 1.49M msgs/s | 767k msgs/s | 1.49M msgs/s | 719k msgs/s | 4 (4 KB) / 4 (13 KB) / 4099 (146 KB) / 18 (23 KB) |
| 64 KiB | 128 | 639k msgs/s | 619k msgs/s | 654k msgs/s | 590k msgs/s | 4 (75 KB) / 4 (76 KB) / 259 (9 KB) / 21 (93 KB) |
| 64 KiB | 512 | 742k msgs/s | 704k msgs/s | 750k msgs/s | 671k msgs/s | 4 (79 KB) / 4 (78 KB) / 1027 (39 KB) / 21 (93 KB) |
| 64 KiB | 2048 | 793k msgs/s | 747k msgs/s | 796k msgs/s | 714k msgs/s | 4 (76 KB) / 4 (84 KB) / 4099 (147 KB) / 21 (95 KB) |
| 256 KiB | 128 | 257k msgs/s | 249k msgs/s | 269k msgs/s | 245k msgs/s | 8 (531 KB) / 7 (523 KB) / 261 (317 KB) / 23 (427 KB) |
| 256 KiB | 512 | 294k msgs/s | 292k msgs/s | 300k msgs/s | 288k msgs/s | 11 (766 KB) / 5 (359 KB) / 1030 (372 KB) / 24 (515 KB) |
| 256 KiB | 2048 | 305k msgs/s | 310k msgs/s | 308k msgs/s | 307k msgs/s | 7 (496 KB) / 6 (431 KB) / 4103 (611 KB) / 21 (280 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

