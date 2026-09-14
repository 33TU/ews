# Broadcast benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench Broadcast -benchtime 500ms | go run ../cmd/results` at ews commit `e220206`.

![broadcast](broadcast.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0, default build: SWAR masking and the shift-based UTF-8 validator
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
| 128 | 566k msgs/s | 150k msgs/s | 551k msgs/s | 128 (2 KB) / 0 / 259 (9 KB) |
| 512 | 785k msgs/s | 200k msgs/s | 761k msgs/s | 512 (8 KB) / 0 / 1027 (36 KB) |
| 2048 | 787k msgs/s | 220k msgs/s | 826k msgs/s | 2049 (32 KB) / 0 / 4099 (144 KB) |

## Compressed

| Conns | ews | ews-sync | gws | allocs/op ews / ews-sync / gws |
|---|---|---|---|---|
| 128 | 476k msgs/s | 140k msgs/s | 485k msgs/s | 128 (3 KB) / 0 (14 KB) / 259 (9 KB) |
| 512 | 631k msgs/s | 145k msgs/s | 504k msgs/s | 512 (8 KB) / 0 / 1027 (36 KB) |
| 2048 | 657k msgs/s | 199k msgs/s | 421k msgs/s | 2048 (51 KB) / 2 (115 KB) / 4099 (144 KB) |

## Reading the numbers

- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain; ews's amortized window update keeps that cheap.

