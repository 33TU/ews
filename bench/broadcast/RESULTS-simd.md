# Broadcast benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `3cd8627`.

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

One message of 256 bytes, 4 KiB or 64 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round, so ews's few are the `Prepared` made once per round, not per recipient.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.
- `gorilla`: `NewPreparedMessage` once, then `WritePreparedMessage` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with `ews-sync`. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.61M msgs/s | 699k msgs/s | 1.58M msgs/s | 657k msgs/s | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.02M msgs/s | 732k msgs/s | 2.00M msgs/s | 691k msgs/s | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.17M msgs/s | 758k msgs/s | 2.19M msgs/s | 720k msgs/s | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.47M msgs/s | 646k msgs/s | 1.45M msgs/s | 606k msgs/s | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.80M msgs/s | 693k msgs/s | 1.80M msgs/s | 650k msgs/s | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.95M msgs/s | 702k msgs/s | 1.95M msgs/s | 657k msgs/s | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 758k msgs/s | 178k msgs/s | 769k msgs/s | 175k msgs/s | 2 (72 KB) / 2 (72 KB) / 259 (9 KB) / 21 (168 KB) |
| 64 KiB | 512 | 1.02M msgs/s | 188k msgs/s | 1.01M msgs/s | 185k msgs/s | 2 (72 KB) / 2 (72 KB) / 1027 (37 KB) / 21 (166 KB) |
| 64 KiB | 2048 | 424k msgs/s | 196k msgs/s | 429k msgs/s | 189k msgs/s | 2 (72 KB) / 2 (72 KB) / 4099 (146 KB) / 21 (165 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 1.26M msgs/s | 699k msgs/s | 1.19M msgs/s | 4 / 4 (1 KB) / 259 (9 KB) |
| 256 B | 512 | 1.56M msgs/s | 746k msgs/s | 1.46M msgs/s | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.49M msgs/s | 700k msgs/s | 872k msgs/s | 4 (1 KB) / 4 (9 KB) / 4099 (144 KB) |
| 4 KiB | 128 | 1.05M msgs/s | 702k msgs/s | 990k msgs/s | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.25M msgs/s | 755k msgs/s | 1.18M msgs/s | 4 (4 KB) / 4 (8 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.10M msgs/s | 681k msgs/s | 717k msgs/s | 4 (4 KB) / 4 (12 KB) / 4099 (144 KB) |
| 64 KiB | 128 | 593k msgs/s | 603k msgs/s | 583k msgs/s | 4 (75 KB) / 4 (75 KB) / 259 (9 KB) |
| 64 KiB | 512 | 687k msgs/s | 702k msgs/s | 660k msgs/s | 4 (74 KB) / 4 (74 KB) / 1027 (36 KB) |
| 64 KiB | 2048 | 672k msgs/s | 644k msgs/s | 465k msgs/s | 4 (72 KB) / 4 (76 KB) / 4099 (150 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.41M msgs/s | 681k msgs/s | 1.59M msgs/s | 643k msgs/s | 4 / 4 (1 KB) / 258 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.78M msgs/s | 735k msgs/s | 2.00M msgs/s | 691k msgs/s | 4 / 4 (3 KB) / 1027 (36 KB) / 18 (16 KB) |
| 256 B | 2048 | 1.96M msgs/s | 767k msgs/s | 2.16M msgs/s | 721k msgs/s | 4 / 4 (7 KB) / 4099 (144 KB) / 18 (18 KB) |
| 4 KiB | 128 | 1.15M msgs/s | 668k msgs/s | 1.16M msgs/s | 634k msgs/s | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.38M msgs/s | 733k msgs/s | 1.37M msgs/s | 694k msgs/s | 4 (5 KB) / 4 (7 KB) / 1027 (36 KB) / 18 (21 KB) |
| 4 KiB | 2048 | 1.51M msgs/s | 763k msgs/s | 1.51M msgs/s | 720k msgs/s | 4 (5 KB) / 4 (13 KB) / 4099 (145 KB) / 18 (25 KB) |
| 64 KiB | 128 | 645k msgs/s | 629k msgs/s | 663k msgs/s | 595k msgs/s | 4 (76 KB) / 4 (76 KB) / 259 (9 KB) / 21 (93 KB) |
| 64 KiB | 512 | 752k msgs/s | 705k msgs/s | 754k msgs/s | 674k msgs/s | 4 (80 KB) / 4 (74 KB) / 1027 (38 KB) / 21 (99 KB) |
| 64 KiB | 2048 | 798k msgs/s | 746k msgs/s | 792k msgs/s | 717k msgs/s | 4 (84 KB) / 4 (90 KB) / 4099 (149 KB) / 21 (95 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

