# Text validation benchmark results

Generated 2026-09-15 from `go test -run '^$' -bench UTF8 -benchtime 500ms | go run ../cmd/results` at ews commit `39a6bc0`.

![utf8-1conn](utf8-1conn.svg)

![utf8-128conn](utf8-128conn.svg)

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0, default build: SWAR masking and the shift-based UTF-8 validator
- GOMAXPROCS: 20
- gws: v1.10.2
- coder/websocket: v1.8.15
- gorilla/websocket: v1.5.3

Text echo across payload kinds, timed like the echo benchmark: one ping-pong at a time per connection, throughput in payload bytes one way. Servers validate UTF-8 where the library offers it, so the difference between columns is the validation pass.

- `ews`: `ReadMessage` and `Write` with `ValidateUTF8` on: one pass over each received text message, a shift-based DFA by default or SIMD lookups under `GOEXPERIMENT=simd`, after skipping the ASCII prefix with word loads.
- `gws`: `ReadMessage` and `WriteMessage` with `CheckUtf8Enabled`, which runs the standard library's `utf8.Valid` on received text and on outgoing text as well, so an echo validates twice.
- `coder`: `Read` and `Write`; coder/websocket has no UTF-8 validation to enable, so this column is the no-validation baseline.

Payloads are JSON-like ASCII, JSON with Japanese values (mixed), and Japanese prose (multibyte), cut on rune boundaries. Clients are ews connections without validation, so the client side costs the same for every server.

## Results

| Kind | Size | Conns | ews | gws | coder | allocs/op ews / gws / coder |
|---|---|---|---|---|---|---|
| ascii | 1 KiB | 1 | 140 MB/s | 137 MB/s | 73 MB/s | 0 / 1 / 24 (2 KB) |
| ascii | 1 KiB | 128 | 1.2 GB/s | 1.1 GB/s | 616 MB/s | 0 / 1 / 24 (2 KB) |
| ascii | 16 KiB | 1 | 1.4 GB/s | 1.4 GB/s | 212 MB/s | 0 / 1 / 61 (40 KB) |
| ascii | 16 KiB | 128 | 11.0 GB/s | 10.6 GB/s | 2.2 GB/s | 0 / 1 / 61 (40 KB) |
| mixed | 1 KiB | 1 | 131 MB/s | 123 MB/s | 66 MB/s | 0 / 1 / 24 (2 KB) |
| mixed | 1 KiB | 128 | 1.1 GB/s | 1.1 GB/s | 611 MB/s | 0 / 1 / 24 (2 KB) |
| mixed | 16 KiB | 1 | 1.1 GB/s | 623 MB/s | 214 MB/s | 0 / 1 / 61 (41 KB) |
| mixed | 16 KiB | 128 | 9.0 GB/s | 5.6 GB/s | 2.3 GB/s | 0 / 1 / 61 (40 KB) |
| multibyte | 1 KiB | 1 | 129 MB/s | 116 MB/s | 60 MB/s | 0 / 1 / 24 (2 KB) |
| multibyte | 1 KiB | 128 | 1.1 GB/s | 1.0 GB/s | 611 MB/s | 0 / 1 / 24 (2 KB) |
| multibyte | 16 KiB | 1 | 919 MB/s | 554 MB/s | 206 MB/s | 0 / 1 / 61 (41 KB) |
| multibyte | 16 KiB | 128 | 8.3 GB/s | 5.0 GB/s | 2.2 GB/s | 0 / 1 / 61 (40 KB) |

## Reading the numbers

- ASCII payloads cost almost nothing to validate in any library: the standard library and ews both skip ASCII in word-sized steps, so these cells match the plain echo results.
- Non-ASCII payloads are where the validators differ. The standard library decodes rune by rune at 1 to 2 GB/s, the ews DFA runs at 2.5 GB/s, and the SIMD kernel at about 10 GB/s; gws also pays the pass twice per echo.
- A validation pass matters most on large messages over few connections, where it is a visible fraction of the round trip; at many connections the syscalls dominate again.

