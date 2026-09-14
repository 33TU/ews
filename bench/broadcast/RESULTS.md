# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 1s | go run ../cmd/results` at ews commit `d9b4a55`.

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
| 128 | 140 MB/s | 40 MB/s | 157 MB/s | 0 / 0 / 259 (9 KB) |
| 512 | 190 MB/s | 48 MB/s | 194 MB/s | 2 / 0 / 1028 (36 KB) |
| 2048 | 182 MB/s | 47 MB/s | 180 MB/s | 50 (5 KB) / 0 / 4120 (147 KB) |

## Compressed

| Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|
| 128 | 119 MB/s | 28 MB/s | 117 MB/s | 0 (9 KB) / 0 (10 KB) / 259 (11 KB) |
| 512 | 137 MB/s | 33 MB/s | 78 MB/s | 4 (68 KB) / 3 (203 KB) / 1030 (80 KB) |
| 2048 | 107 MB/s | 46 MB/s | 92 MB/s | 113 (1315 KB) / 46 (2979 KB) / 4138 (644 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain; ews's amortized window update keeps that cheap.

