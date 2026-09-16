# Broadcast benchmark results

Generated 2026-09-16 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `fa0e5e3`.

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

One message of 256 bytes, 4 KiB or 64 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.
- `gorilla`: `NewPreparedMessage` once, then `WritePreparedMessage` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with `ews-sync`. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.71M msgs/s | 737k msgs/s | 1.68M msgs/s | 693k msgs/s | 0 / 0 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.12M msgs/s | 779k msgs/s | 2.10M msgs/s | 734k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.03M msgs/s | 762k msgs/s | 1.98M msgs/s | 722k msgs/s | 0 / 0 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.52M msgs/s | 680k msgs/s | 1.52M msgs/s | 634k msgs/s | 0 / 0 / 258 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.93M msgs/s | 725k msgs/s | 1.92M msgs/s | 684k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.32M msgs/s | 628k msgs/s | 1.33M msgs/s | 581k msgs/s | 0 / 0 / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 808k msgs/s | 184k msgs/s | 802k msgs/s | 181k msgs/s | 0 / 0 / 259 (9 KB) / 21 (164 KB) |
| 64 KiB | 512 | 603k msgs/s | 193k msgs/s | 603k msgs/s | 194k msgs/s | 0 / 0 / 1027 (36 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 276k msgs/s | 177k msgs/s | 278k msgs/s | 172k msgs/s | 0 / 0 / 4099 (146 KB) / 21 (164 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 1.35M msgs/s | 758k msgs/s | 1.29M msgs/s | 0 / 0 / 259 (9 KB) |
| 256 B | 512 | 1.58M msgs/s | 772k msgs/s | 1.07M msgs/s | 0 / 0 (2 KB) / 1027 (36 KB) |
| 256 B | 2048 | 979k msgs/s | 482k msgs/s | 570k msgs/s | 0 / 0 / 4099 (144 KB) |
| 4 KiB | 128 | 1.11M msgs/s | 748k msgs/s | 1.06M msgs/s | 0 / 0 / 259 (9 KB) |
| 4 KiB | 512 | 1.18M msgs/s | 764k msgs/s | 830k msgs/s | 0 / 0 (3 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 823k msgs/s | 468k msgs/s | 445k msgs/s | 0 / 0 (8 KB) / 4099 (144 KB) |
| 64 KiB | 128 | 644k msgs/s | 670k msgs/s | 629k msgs/s | 0 / 0 / 259 (9 KB) |
| 64 KiB | 512 | 726k msgs/s | 742k msgs/s | 574k msgs/s | 0 / 0 / 1027 (36 KB) |
| 64 KiB | 2048 | 553k msgs/s | 398k msgs/s | 350k msgs/s | 0 / 0 / 4099 (145 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.49M msgs/s | 721k msgs/s | 1.67M msgs/s | 677k msgs/s | 0 / 0 / 258 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.91M msgs/s | 773k msgs/s | 2.13M msgs/s | 732k msgs/s | 0 / 0 / 1027 (36 KB) / 18 (14 KB) |
| 256 B | 2048 | 1.99M msgs/s | 771k msgs/s | 2.07M msgs/s | 719k msgs/s | 0 (1 KB) / 0 (8 KB) / 4099 (144 KB) / 18 (20 KB) |
| 4 KiB | 128 | 1.22M msgs/s | 709k msgs/s | 1.22M msgs/s | 669k msgs/s | 0 / 0 / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.47M msgs/s | 778k msgs/s | 1.48M msgs/s | 731k msgs/s | 0 / 0 (2 KB) / 1027 (36 KB) / 18 (18 KB) |
| 4 KiB | 2048 | 1.51M msgs/s | 772k msgs/s | 1.48M msgs/s | 718k msgs/s | 0 / 0 / 4099 (144 KB) / 18 (25 KB) |
| 64 KiB | 128 | 704k msgs/s | 695k msgs/s | 710k msgs/s | 634k msgs/s | 0 / 0 / 259 (9 KB) / 21 (88 KB) |
| 64 KiB | 512 | 809k msgs/s | 771k msgs/s | 805k msgs/s | 719k msgs/s | 0 / 0 / 1027 (36 KB) / 21 (89 KB) |
| 64 KiB | 2048 | 803k msgs/s | 724k msgs/s | 791k msgs/s | 682k msgs/s | 0 / 0 / 4099 (144 KB) / 21 (98 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

