# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 500ms | go run ../cmd/results` at ews commit `e220206`.

![broadcast-simd](broadcast-simd.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0-X:simd, `GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation
- gws: v1.10.2
- coder/websocket: v1.8.15

One 256-byte message delivered to every connected client, timed until all clients have received it. Servers run behind `httptest` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round.

- `ews`: `Prepare` once, then `SendPrepared` on each connection's `Queue`, returning before the writes complete.
- `ews-sync`: `Prepare` once, then `WritePrepared` on each connection in a loop, waiting for each write.
- `gws`: `NewBroadcaster` once, then `Broadcast` on each connection through its per-connection worker.

Compression is permessage-deflate with context takeover, flate level 1 and 15-bit windows on both libraries; ews servers use `CompressionShared`, the mode meant for many connections. Every server reads through its own `ReadMessage`. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.

## Uncompressed

| Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|
| 128 | 581k msgs/s | 151k msgs/s | 576k msgs/s | 128 (2 KB) / 0 / 259 (9 KB) |
| 512 | 820k msgs/s | 200k msgs/s | 836k msgs/s | 512 (8 KB) / 0 / 1027 (36 KB) |
| 2048 | 852k msgs/s | 229k msgs/s | 853k msgs/s | 2049 (32 KB) / 0 / 4099 (144 KB) |

## Compressed

| Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|
| 128 | 426k msgs/s | 116k msgs/s | 500k msgs/s | 128 (5 KB) / 0 (20 KB) / 259 (9 KB) |
| 512 | 642k msgs/s | 181k msgs/s | 441k msgs/s | 512 (8 KB) / 0 / 1027 (36 KB) |
| 2048 | 664k msgs/s | 212k msgs/s | 466k msgs/s | 2049 (70 KB) / 2 (102 KB) / 4101 (145 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain; ews's amortized window update keeps that cheap.

