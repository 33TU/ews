# Broadcast benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `3cd8627`.

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

One message of 256 bytes, 4 KiB or 64 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round, so ews's few are the `Prepared` made once per round, not per recipient.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.
- `gorilla`: `NewPreparedMessage` once, then `WritePreparedMessage` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with `ews-sync`. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.62M msgs/s | 706k msgs/s | 1.59M msgs/s | 660k msgs/s | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.02M msgs/s | 737k msgs/s | 2.01M msgs/s | 696k msgs/s | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.19M msgs/s | 763k msgs/s | 2.18M msgs/s | 719k msgs/s | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.46M msgs/s | 646k msgs/s | 1.45M msgs/s | 608k msgs/s | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.79M msgs/s | 690k msgs/s | 1.80M msgs/s | 648k msgs/s | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.90M msgs/s | 702k msgs/s | 1.92M msgs/s | 653k msgs/s | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 756k msgs/s | 176k msgs/s | 766k msgs/s | 175k msgs/s | 2 (72 KB) / 2 (72 KB) / 259 (9 KB) / 21 (166 KB) |
| 64 KiB | 512 | 1.02M msgs/s | 200k msgs/s | 1.02M msgs/s | 193k msgs/s | 2 (72 KB) / 2 (72 KB) / 1027 (36 KB) / 21 (165 KB) |
| 64 KiB | 2048 | 424k msgs/s | 188k msgs/s | 428k msgs/s | 193k msgs/s | 2 (72 KB) / 2 (72 KB) / 4099 (145 KB) / 21 (164 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 1.27M msgs/s | 701k msgs/s | 1.18M msgs/s | 4 / 4 / 259 (9 KB) |
| 256 B | 512 | 1.56M msgs/s | 751k msgs/s | 1.45M msgs/s | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.49M msgs/s | 697k msgs/s | 863k msgs/s | 4 (2 KB) / 4 (9 KB) / 4099 (144 KB) |
| 4 KiB | 128 | 1.05M msgs/s | 703k msgs/s | 1.01M msgs/s | 4 (4 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.25M msgs/s | 757k msgs/s | 1.17M msgs/s | 4 (5 KB) / 4 (10 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.10M msgs/s | 683k msgs/s | 699k msgs/s | 4 (4 KB) / 4 (10 KB) / 4099 (149 KB) |
| 64 KiB | 128 | 596k msgs/s | 607k msgs/s | 585k msgs/s | 4 (75 KB) / 4 (75 KB) / 259 (9 KB) |
| 64 KiB | 512 | 688k msgs/s | 698k msgs/s | 661k msgs/s | 4 (75 KB) / 4 (78 KB) / 1027 (36 KB) |
| 64 KiB | 2048 | 669k msgs/s | 641k msgs/s | 463k msgs/s | 4 (74 KB) / 4 (80 KB) / 4099 (145 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.42M msgs/s | 690k msgs/s | 1.59M msgs/s | 642k msgs/s | 4 / 4 / 258 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.78M msgs/s | 737k msgs/s | 1.99M msgs/s | 694k msgs/s | 4 / 4 (4 KB) / 1027 (36 KB) / 18 (16 KB) |
| 256 B | 2048 | 1.97M msgs/s | 770k msgs/s | 2.18M msgs/s | 719k msgs/s | 4 / 4 (9 KB) / 4099 (144 KB) / 18 (18 KB) |
| 4 KiB | 128 | 1.15M msgs/s | 672k msgs/s | 1.16M msgs/s | 633k msgs/s | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.37M msgs/s | 734k msgs/s | 1.37M msgs/s | 691k msgs/s | 4 (5 KB) / 4 (7 KB) / 1027 (36 KB) / 18 (20 KB) |
| 4 KiB | 2048 | 1.51M msgs/s | 763k msgs/s | 1.49M msgs/s | 715k msgs/s | 4 (4 KB) / 4 (13 KB) / 4099 (145 KB) / 18 (27 KB) |
| 64 KiB | 128 | 647k msgs/s | 628k msgs/s | 665k msgs/s | 591k msgs/s | 4 (76 KB) / 4 (76 KB) / 259 (9 KB) / 21 (96 KB) |
| 64 KiB | 512 | 748k msgs/s | 706k msgs/s | 758k msgs/s | 668k msgs/s | 4 (75 KB) / 4 (76 KB) / 1027 (38 KB) / 21 (91 KB) |
| 64 KiB | 2048 | 801k msgs/s | 746k msgs/s | 795k msgs/s | 704k msgs/s | 4 (86 KB) / 4 (86 KB) / 4099 (151 KB) / 21 (105 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

