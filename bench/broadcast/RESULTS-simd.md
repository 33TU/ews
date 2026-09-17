# Broadcast benchmark results

Generated 2026-09-17 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `07f1229`.

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

One message of 256 bytes, 4 KiB, 64 KiB or 256 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round, so ews's few are the `Prepared` made once per round, not per recipient.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.
- `gorilla`: `NewPreparedMessage` once, then `WritePreparedMessage` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with `ews-sync`. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.62M msgs/s | 706k msgs/s | 1.59M msgs/s | 660k msgs/s | 2 / 2 / 259 (9 KB) / 15 (11 KB) |
| 256 B | 512 | 2.01M msgs/s | 738k msgs/s | 1.99M msgs/s | 695k msgs/s | 2 / 2 / 1027 (36 KB) / 15 (11 KB) |
| 256 B | 2048 | 2.16M msgs/s | 763k msgs/s | 2.17M msgs/s | 722k msgs/s | 2 / 2 / 4099 (144 KB) / 15 (11 KB) |
| 4 KiB | 128 | 1.44M msgs/s | 651k msgs/s | 1.46M msgs/s | 609k msgs/s | 2 (4 KB) / 2 (4 KB) / 259 (9 KB) / 15 (20 KB) |
| 4 KiB | 512 | 1.79M msgs/s | 690k msgs/s | 1.78M msgs/s | 653k msgs/s | 2 (4 KB) / 2 (4 KB) / 1027 (36 KB) / 15 (20 KB) |
| 4 KiB | 2048 | 1.90M msgs/s | 700k msgs/s | 1.93M msgs/s | 659k msgs/s | 2 (4 KB) / 2 (4 KB) / 4099 (144 KB) / 15 (20 KB) |
| 64 KiB | 128 | 753k msgs/s | 175k msgs/s | 759k msgs/s | 179k msgs/s | 2 (72 KB) / 2 (73 KB) / 259 (9 KB) / 21 (166 KB) |
| 64 KiB | 512 | 988k msgs/s | 178k msgs/s | 988k msgs/s | 183k msgs/s | 2 (72 KB) / 2 (72 KB) / 1027 (37 KB) / 21 (165 KB) |
| 64 KiB | 2048 | 414k msgs/s | 177k msgs/s | 415k msgs/s | 178k msgs/s | 2 (72 KB) / 2 (72 KB) / 4099 (144 KB) / 21 (164 KB) |
| 256 KiB | 128 | 322k msgs/s | 75k msgs/s | 321k msgs/s | 74k msgs/s | 2 (272 KB) / 2 (283 KB) / 261 (283 KB) / 22 (624 KB) |
| 256 KiB | 512 | 196k msgs/s | 70k msgs/s | 195k msgs/s | 70k msgs/s | 2 (265 KB) / 2 (272 KB) / 1029 (305 KB) / 21 (572 KB) |
| 256 KiB | 2048 | 76k msgs/s | 68k msgs/s | 77k msgs/s | 68k msgs/s | 2 (264 KB) / 2 (264 KB) / 4101 (408 KB) / 21 (568 KB) |

## Compressed with context takeover

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 1.27M msgs/s | 704k msgs/s | 1.18M msgs/s | 4 / 4 / 259 (9 KB) |
| 256 B | 512 | 1.55M msgs/s | 757k msgs/s | 1.45M msgs/s | 4 / 4 (3 KB) / 1027 (36 KB) |
| 256 B | 2048 | 1.45M msgs/s | 699k msgs/s | 857k msgs/s | 4 (2 KB) / 4 (9 KB) / 4099 (144 KB) |
| 4 KiB | 128 | 1.03M msgs/s | 708k msgs/s | 985k msgs/s | 4 (5 KB) / 4 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 1.24M msgs/s | 756k msgs/s | 1.16M msgs/s | 4 (4 KB) / 4 (7 KB) / 1027 (36 KB) |
| 4 KiB | 2048 | 1.10M msgs/s | 704k msgs/s | 691k msgs/s | 4 (4 KB) / 4 (10 KB) / 4099 (144 KB) |
| 64 KiB | 128 | 614k msgs/s | 611k msgs/s | 592k msgs/s | 4 (76 KB) / 4 (75 KB) / 259 (9 KB) |
| 64 KiB | 512 | 699k msgs/s | 705k msgs/s | 670k msgs/s | 4 (75 KB) / 4 (76 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 680k msgs/s | 657k msgs/s | 465k msgs/s | 4 (81 KB) / 4 (82 KB) / 4099 (144 KB) |
| 256 KiB | 128 | 256k msgs/s | 242k msgs/s | 257k msgs/s | 6 (391 KB) / 8 (559 KB) / 261 (312 KB) |
| 256 KiB | 512 | 288k msgs/s | 283k msgs/s | 286k msgs/s | 5 (347 KB) / 4 (273 KB) / 1029 (309 KB) |
| 256 KiB | 2048 | 281k msgs/s | 288k msgs/s | 242k msgs/s | 4 (269 KB) / 4 (274 KB) / 4101 (408 KB) |

## Compressed without context takeover

| Size | Conns | ews | ews-sync | gws | gorilla | allocs/op ews / ews-sync / gws / gorilla |
|---|---|---|---|---|---|---|
| 256 B | 128 | 1.40M msgs/s | 685k msgs/s | 1.57M msgs/s | 641k msgs/s | 4 / 4 / 259 (9 KB) / 18 (12 KB) |
| 256 B | 512 | 1.75M msgs/s | 734k msgs/s | 1.95M msgs/s | 693k msgs/s | 4 / 4 (4 KB) / 1027 (36 KB) / 18 (15 KB) |
| 256 B | 2048 | 1.94M msgs/s | 773k msgs/s | 2.13M msgs/s | 723k msgs/s | 4 (1 KB) / 4 (7 KB) / 4099 (144 KB) / 18 (26 KB) |
| 4 KiB | 128 | 1.14M msgs/s | 671k msgs/s | 1.14M msgs/s | 633k msgs/s | 4 (5 KB) / 4 (4 KB) / 259 (9 KB) / 18 (16 KB) |
| 4 KiB | 512 | 1.37M msgs/s | 736k msgs/s | 1.36M msgs/s | 694k msgs/s | 4 (4 KB) / 4 (7 KB) / 1027 (36 KB) / 18 (20 KB) |
| 4 KiB | 2048 | 1.47M msgs/s | 773k msgs/s | 1.50M msgs/s | 721k msgs/s | 4 (5 KB) / 4 (13 KB) / 4099 (145 KB) / 18 (23 KB) |
| 64 KiB | 128 | 637k msgs/s | 621k msgs/s | 655k msgs/s | 587k msgs/s | 4 (77 KB) / 4 (76 KB) / 259 (9 KB) / 21 (93 KB) |
| 64 KiB | 512 | 746k msgs/s | 703k msgs/s | 750k msgs/s | 681k msgs/s | 4 (76 KB) / 4 (75 KB) / 1027 (37 KB) / 21 (90 KB) |
| 64 KiB | 2048 | 786k msgs/s | 747k msgs/s | 788k msgs/s | 717k msgs/s | 4 (89 KB) / 4 (77 KB) / 4099 (153 KB) / 21 (105 KB) |
| 256 KiB | 128 | 258k msgs/s | 247k msgs/s | 266k msgs/s | 246k msgs/s | 7 (490 KB) / 8 (554 KB) / 262 (382 KB) / 23 (430 KB) |
| 256 KiB | 512 | 291k msgs/s | 286k msgs/s | 298k msgs/s | 279k msgs/s | 15 (1068 KB) / 9 (691 KB) / 1030 (400 KB) / 37 (1492 KB) |
| 256 KiB | 2048 | 308k msgs/s | 309k msgs/s | 301k msgs/s | 306k msgs/s | 5 (378 KB) / 5 (402 KB) / 4102 (523 KB) / 23 (466 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed with takeover, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
- Without takeover there is no history to update, and ews and gws tie at the write floor again; gorilla's synchronous prepared write sits with ews-sync, a little behind it on allocations.

