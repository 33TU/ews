# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `32177d3`.

![broadcast-plain-simd](broadcast-plain-simd.svg)

![broadcast-compressed-simd](broadcast-compressed-simd.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
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
| 256 B | 128 | 964k msgs/s | 161k msgs/s | 900k msgs/s | 0 / 0 / 259 (9 KB) |
| 256 B | 512 | 1.28M msgs/s | 250k msgs/s | 1.25M msgs/s | 0 / 0 / 1027 (36 KB) |
| 256 B | 2048 | 1.26M msgs/s | 273k msgs/s | 1.26M msgs/s | 1 / 0 / 4100 (145 KB) |
| 4 KiB | 128 | 838k msgs/s | 175k msgs/s | 843k msgs/s | 0 / 0 / 259 (9 KB) |
| 4 KiB | 512 | 1.16M msgs/s | 208k msgs/s | 1.15M msgs/s | 0 / 0 / 1027 (36 KB) |
| 4 KiB | 2048 | 1.06M msgs/s | 244k msgs/s | 1.05M msgs/s | 1 / 0 / 4099 (144 KB) |
| 64 KiB | 128 | 451k msgs/s | 95k msgs/s | 451k msgs/s | 0 / 0 (1 KB) / 259 (9 KB) |
| 64 KiB | 512 | 494k msgs/s | 107k msgs/s | 481k msgs/s | 0 / 0 (2 KB) / 1027 (36 KB) |
| 64 KiB | 2048 | 269k msgs/s | 98k msgs/s | 268k msgs/s | 2 (3 KB) / 0 (1 KB) / 4102 (149 KB) |

## Compressed

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 565k msgs/s | 127k msgs/s | 570k msgs/s | 0 / 0 / 258 (9 KB) |
| 256 B | 512 | 678k msgs/s | 157k msgs/s | 457k msgs/s | 0 / 0 (29 KB) / 1027 (36 KB) |
| 256 B | 2048 | 690k msgs/s | 232k msgs/s | 420k msgs/s | 0 (9 KB) / 0 / 4100 (144 KB) |
| 4 KiB | 128 | 573k msgs/s | 154k msgs/s | 529k msgs/s | 0 (1 KB) / 0 (5 KB) / 259 (9 KB) |
| 4 KiB | 512 | 685k msgs/s | 173k msgs/s | 440k msgs/s | 0 (3 KB) / 0 / 1027 (36 KB) |
| 4 KiB | 2048 | 631k msgs/s | 225k msgs/s | 332k msgs/s | 2 (16 KB) / 0 / 4103 (146 KB) |
| 64 KiB | 128 | 368k msgs/s | 143k msgs/s | 317k msgs/s | 0 (1 KB) / 0 / 259 (9 KB) |
| 64 KiB | 512 | 458k msgs/s | 157k msgs/s | 329k msgs/s | 0 / 0 (21 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 412k msgs/s | 153k msgs/s | 321k msgs/s | 0 (32 KB) / 0 (4 KB) / 4099 (147 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.

