# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `900cb5b`.

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0
- gws: v1.10.2
- coder/websocket: v1.8.15

One 256-byte message delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.

Compression is permessage-deflate with context takeover, flate level 1 and 15-bit windows on both libraries; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`.

## Uncompressed

| Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|
| 128 | 604k msgs/s | 174k msgs/s | 569k msgs/s | 128 (2 KB) / 0 / 259 (9 KB) |
| 512 | 761k msgs/s | 171k msgs/s | 727k msgs/s | 513 (8 KB) / 0 / 1028 (36 KB) |
| 2048 | 776k msgs/s | 208k msgs/s | 709k msgs/s | 2063 (34 KB) / 0 / 4120 (147 KB) |

## Compressed

| Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|
| 128 | 464k msgs/s | 129k msgs/s | 484k msgs/s | 128 (11 KB) / 0 (11 KB) / 259 (11 KB) |
| 512 | 641k msgs/s | 146k msgs/s | 363k msgs/s | 513 (64 KB) / 2 (165 KB) / 1030 (75 KB) |
| 2048 | 535k msgs/s | 174k msgs/s | 399k msgs/s | 2079 (882 KB) / 39 (2545 KB) / 4139 (650 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain; ews's amortized window update keeps that cheap.

