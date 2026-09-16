# Broadcast benchmark results

Generated 2026-09-16 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `fa0e5e3`.

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

One message of 256 bytes, 4 KiB or 64 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.
- `gorilla`: `NewPreparedMessage` once, then `WritePreparedMessage` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with `ews-sync`. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.68M msgs/s | 738k msgs/s | 1.67M msgs/s | 690k msgs/s | 0 / 0 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.11M msgs/s | 772k msgs/s | 2.11M msgs/s | 731k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.06M msgs/s | 763k msgs/s | 2.02M msgs/s | 712k msgs/s | 0 / 0 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.56M msgs/s | 678k msgs/s | 1.52M msgs/s | 634k msgs/s | 0 / 0 / 258 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.94M msgs/s | 719k msgs/s | 1.92M msgs/s | 677k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.36M msgs/s | 626k msgs/s | 1.35M msgs/s | 586k msgs/s | 0 / 0 / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 809k msgs/s | 184k msgs/s | 810k msgs/s | 179k msgs/s | 0 / 0 / 259 (9 KB) / 21 (164 KB) |
| 64 KiB | 512 | 588k msgs/s | 198k msgs/s | 596k msgs/s | 192k msgs/s | 0 / 0 / 1027 (36 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 274k msgs/s | 176k msgs/s | 277k msgs/s | 173k msgs/s | 0 (1 KB) / 0 / 4100 (146 KB) / 21 (165 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 1.35M msgs/s | 751k msgs/s | 1.29M msgs/s | 0 / 0 / 259 (9 KB) |
| 256 B | 512 | 1.57M msgs/s | 770k msgs/s | 1.05M msgs/s | 0 / 0 (2 KB) / 1027 (36 KB) |
| 256 B | 2048 | 977k msgs/s | 525k msgs/s | 572k msgs/s | 0 / 0 / 4099 (144 KB) |
| 4 KiB | 128 | 1.11M msgs/s | 740k msgs/s | 1.08M msgs/s | 0 / 0 / 259 (9 KB) |
| 4 KiB | 512 | 1.19M msgs/s | 762k msgs/s | 835k msgs/s | 0 / 0 (2 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 822k msgs/s | 480k msgs/s | 447k msgs/s | 0 / 0 (17 KB) / 4099 (144 KB) |
| 64 KiB | 128 | 641k msgs/s | 665k msgs/s | 630k msgs/s | 0 / 0 / 259 (9 KB) |
| 64 KiB | 512 | 724k msgs/s | 744k msgs/s | 572k msgs/s | 0 / 0 / 1027 (36 KB) |
| 64 KiB | 2048 | 545k msgs/s | 393k msgs/s | 347k msgs/s | 0 / 0 / 4099 (144 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.50M msgs/s | 721k msgs/s | 1.68M msgs/s | 676k msgs/s | 0 / 0 / 258 (9 KB) / 18 (11 KB) |
| 256 B | 512 | 1.91M msgs/s | 769k msgs/s | 2.13M msgs/s | 728k msgs/s | 0 / 0 / 1027 (36 KB) / 18 (14 KB) |
| 256 B | 2048 | 1.94M msgs/s | 762k msgs/s | 2.05M msgs/s | 721k msgs/s | 0 / 0 (9 KB) / 4099 (144 KB) / 18 (20 KB) |
| 4 KiB | 128 | 1.21M msgs/s | 712k msgs/s | 1.21M msgs/s | 667k msgs/s | 0 / 0 / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.45M msgs/s | 774k msgs/s | 1.47M msgs/s | 729k msgs/s | 0 / 0 (3 KB) / 1027 (36 KB) / 18 (18 KB) |
| 4 KiB | 2048 | 1.50M msgs/s | 772k msgs/s | 1.48M msgs/s | 717k msgs/s | 0 / 0 / 4099 (144 KB) / 18 (25 KB) |
| 64 KiB | 128 | 700k msgs/s | 695k msgs/s | 706k msgs/s | 636k msgs/s | 0 / 0 / 259 (9 KB) / 21 (88 KB) |
| 64 KiB | 512 | 805k msgs/s | 775k msgs/s | 806k msgs/s | 726k msgs/s | 0 / 0 (1 KB) / 1027 (36 KB) / 21 (89 KB) |
| 64 KiB | 2048 | 802k msgs/s | 719k msgs/s | 788k msgs/s | 679k msgs/s | 0 / 0 / 4099 (144 KB) / 21 (96 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

