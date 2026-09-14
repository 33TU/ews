# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `32e2917`.

![broadcast-plain](broadcast-plain.svg)

![broadcast-compressed](broadcast-compressed.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0, default build: SWAR masking and the shift-based UTF-8 validator
- gws: v1.10.2
- coder/websocket: v1.8.15

One message of 256 bytes, 4 KiB or 64 KiB delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.

Compression is permessage-deflate with context takeover, flate level 1 and 15-bit windows on both libraries; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 1.00M msgs/s | 169k msgs/s | 907k msgs/s | 0 / 0 / 259 (9 KB) |
| 256 B | 512 | 1.34M msgs/s | 243k msgs/s | 1.32M msgs/s | 0 / 0 / 1027 (36 KB) |
| 256 B | 2048 | 1.34M msgs/s | 281k msgs/s | 1.37M msgs/s | 1 / 0 / 4101 (145 KB) |
| 4 KiB | 128 | 869k msgs/s | 142k msgs/s | 876k msgs/s | 0 / 0 / 259 (9 KB) |
| 4 KiB | 512 | 1.18M msgs/s | 214k msgs/s | 1.17M msgs/s | 0 / 0 / 1027 (36 KB) |
| 4 KiB | 2048 | 1.07M msgs/s | 245k msgs/s | 1.05M msgs/s | 2 (1 KB) / 0 / 4100 (145 KB) |
| 64 KiB | 128 | 458k msgs/s | 93k msgs/s | 457k msgs/s | 0 / 0 (1 KB) / 259 (9 KB) |
| 64 KiB | 512 | 488k msgs/s | 108k msgs/s | 491k msgs/s | 0 / 0 (2 KB) / 1027 (36 KB) |
| 64 KiB | 2048 | 290k msgs/s | 100k msgs/s | 287k msgs/s | 0 (2 KB) / 0 / 4099 (147 KB) |

## Compressed

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 649k msgs/s | 117k msgs/s | 655k msgs/s | 0 / 0 / 259 (9 KB) |
| 256 B | 512 | 889k msgs/s | 178k msgs/s | 533k msgs/s | 0 / 0 (30 KB) / 1027 (36 KB) |
| 256 B | 2048 | 794k msgs/s | 254k msgs/s | 428k msgs/s | 0 (9 KB) / 0 / 4100 (144 KB) |
| 4 KiB | 128 | 617k msgs/s | 139k msgs/s | 571k msgs/s | 0 / 0 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 778k msgs/s | 170k msgs/s | 490k msgs/s | 0 (2 KB) / 0 / 1027 (36 KB) |
| 4 KiB | 2048 | 665k msgs/s | 247k msgs/s | 349k msgs/s | 0 (17 KB) / 0 / 4100 (144 KB) |
| 64 KiB | 128 | 400k msgs/s | 154k msgs/s | 376k msgs/s | 0 (2 KB) / 0 / 259 (9 KB) |
| 64 KiB | 512 | 478k msgs/s | 165k msgs/s | 396k msgs/s | 0 / 0 (17 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 442k msgs/s | 162k msgs/s | 344k msgs/s | 0 (23 KB) / 0 (3 KB) / 4099 (148 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.

