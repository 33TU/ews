# Broadcast benchmark results

Generated 2026-09-15 from `go test -run '^$' -bench Broadcast -benchtime 500ms | go run ../cmd/results` at ews commit `8db424c`.

![broadcast-plain](broadcast-plain.svg)

![broadcast-compressed](broadcast-compressed.svg)

![broadcast-nocontext](broadcast-nocontext.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0, default build: SWAR masking and the shift-based UTF-8 validator
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
| 256 B | 128 | 902k msgs/s | 146k msgs/s | 919k msgs/s | 138k msgs/s | 0 / 0 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 1.25M msgs/s | 249k msgs/s | 1.24M msgs/s | 231k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 1.26M msgs/s | 262k msgs/s | 1.25M msgs/s | 238k msgs/s | 1 (1 KB) / 0 / 4100 (145 KB) / 15 (11 KB) |
| 4 KiB | 128 | 836k msgs/s | 170k msgs/s | 850k msgs/s | 160k msgs/s | 0 / 0 / 258 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.11M msgs/s | 215k msgs/s | 1.06M msgs/s | 175k msgs/s | 0 / 0 / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 993k msgs/s | 237k msgs/s | 989k msgs/s | 219k msgs/s | 0 / 0 / 4101 (145 KB) / 15 (20 KB) |
| 64 KiB | 128 | 444k msgs/s | 90k msgs/s | 438k msgs/s | 92k msgs/s | 0 / 0 / 259 (9 KB) / 21 (164 KB) |
| 64 KiB | 512 | 464k msgs/s | 100k msgs/s | 458k msgs/s | 99k msgs/s | 0 / 0 / 1027 (36 KB) / 21 (164 KB) |
| 64 KiB | 2048 | 268k msgs/s | 92k msgs/s | 267k msgs/s | 91k msgs/s | 1 (2 KB) / 0 (2 KB) / 4101 (149 KB) / 21 (167 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 623k msgs/s | 130k msgs/s | 622k msgs/s | 0 / 0 / 259 (9 KB) |
| 256 B | 512 | 832k msgs/s | 161k msgs/s | 443k msgs/s | 0 / 0 (22 KB) / 1027 (36 KB) |
| 256 B | 2048 | 711k msgs/s | 224k msgs/s | 395k msgs/s | 1 / 0 / 4100 (144 KB) |
| 4 KiB | 128 | 588k msgs/s | 142k msgs/s | 568k msgs/s | 0 (1 KB) / 0 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 684k msgs/s | 156k msgs/s | 412k msgs/s | 0 / 0 (24 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 615k msgs/s | 213k msgs/s | 354k msgs/s | 0 (18 KB) / 1 (71 KB) / 4100 (144 KB) |
| 64 KiB | 128 | 366k msgs/s | 154k msgs/s | 359k msgs/s | 0 (1 KB) / 0 / 259 (9 KB) |
| 64 KiB | 512 | 453k msgs/s | 158k msgs/s | 380k msgs/s | 0 / 0 (18 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 417k msgs/s | 156k msgs/s | 316k msgs/s | 1 (1 KB) / 0 (2 KB) / 4099 (146 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 757k msgs/s | 136k msgs/s | 954k msgs/s | 152k msgs/s | 0 / 0 (8 KB) / 258 (9 KB) / 18 (17 KB) |
| 256 B | 512 | 1.14M msgs/s | 209k msgs/s | 1.26M msgs/s | 199k msgs/s | 0 (2 KB) / 0 / 1027 (36 KB) / 18 (39 KB) |
| 256 B | 2048 | 947k msgs/s | 249k msgs/s | 1.26M msgs/s | 237k msgs/s | 2 (6 KB) / 1 (64 KB) / 4101 (145 KB) / 19 (80 KB) |
| 4 KiB | 128 | 643k msgs/s | 150k msgs/s | 635k msgs/s | 122k msgs/s | 0 (1 KB) / 0 (6 KB) / 259 (9 KB) / 18 (23 KB) |
| 4 KiB | 512 | 935k msgs/s | 212k msgs/s | 935k msgs/s | 160k msgs/s | 0 / 0 (24 KB) / 1027 (36 KB) / 18 (53 KB) |
| 4 KiB | 2048 | 961k msgs/s | 248k msgs/s | 918k msgs/s | 243k msgs/s | 0 (11 KB) / 0 / 4099 (144 KB) / 18 (73 KB) |
| 64 KiB | 128 | 429k msgs/s | 167k msgs/s | 444k msgs/s | 162k msgs/s | 0 (1 KB) / 0 (4 KB) / 259 (9 KB) / 21 (93 KB) |
| 64 KiB | 512 | 546k msgs/s | 164k msgs/s | 552k msgs/s | 171k msgs/s | 0 (4 KB) / 0 (27 KB) / 1027 (36 KB) / 21 (108 KB) |
| 64 KiB | 2048 | 560k msgs/s | 230k msgs/s | 563k msgs/s | 225k msgs/s | 2 (3 KB) / 0 / 4102 (148 KB) / 22 (154 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

