# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `32e2917`.

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
| 256 B | 128 | 996k msgs/s | 150k msgs/s | 933k msgs/s | 0 / 0 / 259 (9 KB) |
| 256 B | 512 | 1.36M msgs/s | 221k msgs/s | 1.31M msgs/s | 0 / 0 / 1027 (36 KB) |
| 256 B | 2048 | 1.34M msgs/s | 278k msgs/s | 1.38M msgs/s | 0 / 0 / 4099 (144 KB) |
| 4 KiB | 128 | 859k msgs/s | 146k msgs/s | 892k msgs/s | 0 / 0 / 259 (9 KB) |
| 4 KiB | 512 | 1.20M msgs/s | 203k msgs/s | 1.21M msgs/s | 0 / 0 / 1027 (36 KB) |
| 4 KiB | 2048 | 1.07M msgs/s | 238k msgs/s | 1.06M msgs/s | 0 / 0 / 4101 (145 KB) |
| 64 KiB | 128 | 453k msgs/s | 90k msgs/s | 461k msgs/s | 0 / 0 (1 KB) / 259 (9 KB) |
| 64 KiB | 512 | 497k msgs/s | 108k msgs/s | 494k msgs/s | 0 / 0 / 1027 (36 KB) |
| 64 KiB | 2048 | 288k msgs/s | 99k msgs/s | 282k msgs/s | 3 (2 KB) / 0 / 4099 (147 KB) |

## Compressed

| Size | Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|---|
| 256 B | 128 | 599k msgs/s | 125k msgs/s | 639k msgs/s | 0 (1 KB) / 0 / 259 (9 KB) |
| 256 B | 512 | 915k msgs/s | 163k msgs/s | 494k msgs/s | 0 / 0 (32 KB) / 1027 (36 KB) |
| 256 B | 2048 | 779k msgs/s | 255k msgs/s | 461k msgs/s | 0 (9 KB) / 0 / 4101 (145 KB) |
| 4 KiB | 128 | 605k msgs/s | 140k msgs/s | 568k msgs/s | 0 (1 KB) / 0 (6 KB) / 259 (9 KB) |
| 4 KiB | 512 | 758k msgs/s | 151k msgs/s | 472k msgs/s | 0 (2 KB) / 0 / 1027 (36 KB) |
| 4 KiB | 2048 | 660k msgs/s | 238k msgs/s | 377k msgs/s | 1 (19 KB) / 0 / 4099 (144 KB) |
| 64 KiB | 128 | 384k msgs/s | 154k msgs/s | 358k msgs/s | 0 (1 KB) / 0 (2 KB) / 259 (9 KB) |
| 64 KiB | 512 | 469k msgs/s | 156k msgs/s | 396k msgs/s | 0 / 0 (16 KB) / 1027 (37 KB) |
| 64 KiB | 2048 | 435k msgs/s | 170k msgs/s | 344k msgs/s | 3 (32 KB) / 0 (3 KB) / 4100 (147 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.

