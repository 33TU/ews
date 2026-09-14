# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `3a81efb`.

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
| 128 | 614k msgs/s | 159k msgs/s | 639k msgs/s | 0 / 0 / 259 (9 KB) |
| 512 | 826k msgs/s | 199k msgs/s | 714k msgs/s | 2 / 0 / 1028 (36 KB) |
| 2048 | 689k msgs/s | 205k msgs/s | 850k msgs/s | 61 (6 KB) / 0 / 4117 (146 KB) |

## Compressed

| Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|
| 128 | 531k msgs/s | 115k msgs/s | 536k msgs/s | 0 (7 KB) / 0 (12 KB) / 259 (10 KB) |
| 512 | 665k msgs/s | 149k msgs/s | 444k msgs/s | 3 (53 KB) / 3 (181 KB) / 1030 (73 KB) |
| 2048 | 530k msgs/s | 202k msgs/s | 421k msgs/s | 92 (1022 KB) / 41 (2621 KB) / 4139 (646 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain; ews's amortized window update keeps that cheap.

