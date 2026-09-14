# Broadcast benchmark results

Generated 2026-09-15 from `go test -run '^$' -bench Broadcast -benchtime 500ms | go run ../cmd/results` at ews commit `39a6bc0`.

![broadcast-plain-simd](broadcast-plain-simd.svg)

![broadcast-compressed-simd](broadcast-compressed-simd.svg)

![broadcast-nocontext-simd](broadcast-nocontext-simd.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
- GOMAXPROCS: 20
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
| 256 B | 128 | 927k msgs/s | 151k msgs/s | 859k msgs/s | 138k msgs/s | 0 / 0 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 1.28M msgs/s | 237k msgs/s | 1.25M msgs/s | 230k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 1.26M msgs/s | 261k msgs/s | 1.26M msgs/s | 236k msgs/s | 1 (1 KB) / 0 / 4100 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 838k msgs/s | 165k msgs/s | 845k msgs/s | 139k msgs/s | 0 / 0 / 258 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.11M msgs/s | 196k msgs/s | 1.09M msgs/s | 196k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 970k msgs/s | 225k msgs/s | 972k msgs/s | 224k msgs/s | 2 (1 KB) / 0 / 4101 (145 KB) / 15 (20 KB) |
| 64 KiB | 128 | 447k msgs/s | 90k msgs/s | 441k msgs/s | 91k msgs/s | 0 / 0 / 259 (9 KB) / 21 (164 KB) |
| 64 KiB | 512 | 451k msgs/s | 103k msgs/s | 453k msgs/s | 101k msgs/s | 0 / 0 / 1027 (37 KB) / 21 (168 KB) |
| 64 KiB | 2048 | 259k msgs/s | 96k msgs/s | 257k msgs/s | 93k msgs/s | 2 (3 KB) / 0 / 4100 (148 KB) / 21 (165 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 564k msgs/s | 128k msgs/s | 553k msgs/s | 0 / 0 / 258 (9 KB) |
| 256 B | 512 | 817k msgs/s | 159k msgs/s | 452k msgs/s | 0 / 0 (25 KB) / 1027 (36 KB) |
| 256 B | 2048 | 708k msgs/s | 222k msgs/s | 337k msgs/s | 1 / 0 / 4101 (145 KB) |
| 4 KiB | 128 | 569k msgs/s | 145k msgs/s | 552k msgs/s | 0 (1 KB) / 0 (6 KB) / 259 (9 KB) |
| 4 KiB | 512 | 707k msgs/s | 185k msgs/s | 446k msgs/s | 0 / 0 (23 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 603k msgs/s | 221k msgs/s | 323k msgs/s | 1 (12 KB) / 1 (62 KB) / 4101 (145 KB) |
| 64 KiB | 128 | 369k msgs/s | 146k msgs/s | 358k msgs/s | 0 (2 KB) / 0 / 259 (9 KB) |
| 64 KiB | 512 | 453k msgs/s | 148k msgs/s | 372k msgs/s | 0 / 0 (22 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 412k msgs/s | 164k msgs/s | 317k msgs/s | 0 (2 KB) / 0 (5 KB) / 4104 (148 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 786k msgs/s | 151k msgs/s | 954k msgs/s | 160k msgs/s | 0 / 0 (6 KB) / 258 (9 KB) / 18 (17 KB) |
| 256 B | 512 | 1.16M msgs/s | 227k msgs/s | 1.27M msgs/s | 204k msgs/s | 0 (2 KB) / 0 / 1027 (36 KB) / 18 (39 KB) |
| 256 B | 2048 | 1.18M msgs/s | 253k msgs/s | 1.27M msgs/s | 249k msgs/s | 2 (9 KB) / 1 (70 KB) / 4100 (145 KB) / 19 (84 KB) |
| 4 KiB | 128 | 651k msgs/s | 185k msgs/s | 648k msgs/s | 134k msgs/s | 0 / 0 (5 KB) / 259 (9 KB) / 18 (21 KB) |
| 4 KiB | 512 | 929k msgs/s | 184k msgs/s | 932k msgs/s | 175k msgs/s | 0 / 0 (27 KB) / 1027 (36 KB) / 18 (43 KB) |
| 4 KiB | 2048 | 951k msgs/s | 240k msgs/s | 964k msgs/s | 234k msgs/s | 0 (8 KB) / 0 / 4099 (144 KB) / 19 (87 KB) |
| 64 KiB | 128 | 426k msgs/s | 162k msgs/s | 445k msgs/s | 150k msgs/s | 0 (1 KB) / 0 (6 KB) / 259 (9 KB) / 21 (93 KB) |
| 64 KiB | 512 | 538k msgs/s | 182k msgs/s | 551k msgs/s | 173k msgs/s | 0 (6 KB) / 0 (20 KB) / 1027 (37 KB) / 21 (108 KB) |
| 64 KiB | 2048 | 558k msgs/s | 220k msgs/s | 556k msgs/s | 234k msgs/s | 0 (2 KB) / 0 / 4099 (146 KB) / 22 (165 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

