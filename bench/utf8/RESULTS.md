# Text validation benchmark results

Generated 2026-09-14 from `go test -run '^$' -bench UTF8 -benchtime 500ms | go run ../cmd/results` at ews commit `11f8846`.

## Setup

- CPU: 13th Gen Intel(R) Core(TM) i9-13900H
- Kernel: 6.12.0-211.53.1.el10_2.x86_64
- Go: go1.27.0, default build: SWAR masking and the shift-based UTF-8 validator
- gws: v1.10.2
- coder/websocket: v1.8.15

Text echo across payload kinds, timed like the echo benchmark: one ping-pong at a time per connection, throughput in payload bytes one way. Servers validate UTF-8 where the library offers it, so the difference between columns is the validation pass.

- `ews`: `ReadMessage` and `Write` with `ValidateUTF8` on: one pass over each received text message, a shift-based DFA by default or SIMD lookups under `GOEXPERIMENT=simd`, after skipping the ASCII prefix with word loads.
- `gws`: `ReadMessage` and `WriteMessage` with `CheckUtf8Enabled`, which runs the standard library's `utf8.Valid` on received text and on outgoing text as well, so an echo validates twice.
- `coder`: `Read` and `Write`; coder/websocket has no UTF-8 validation to enable, so this column is the no-validation baseline.

Payloads are JSON-like ASCII, JSON with Japanese values (mixed), and Japanese prose (multibyte), cut on rune boundaries. Clients are ews connections without validation, so the client side costs the same for every server.

## Results

| Kind | Size | Conns | ews | gws | coder | allocs/op ews / gws / coder |
|---|---|---|---|---|---|---|
| ascii | 1 KiB | 1 | 124 MB/s | 126 MB/s | 58 MB/s | 0 / 1 / 24 (2 KB) |
| ascii | 1 KiB | 128 | 867 MB/s | 843 MB/s | 542 MB/s | 0 / 1 / 24 (2 KB) |
| ascii | 16 KiB | 1 | 1.3 GB/s | 1.3 GB/s | 188 MB/s | 0 / 1 / 61 (40 KB) |
| ascii | 16 KiB | 128 | 9.7 GB/s | 9.4 GB/s | 2.2 GB/s | 0 / 1 / 61 (39 KB) |
| mixed | 1 KiB | 1 | 126 MB/s | 112 MB/s | 60 MB/s | 0 / 1 / 24 (2 KB) |
| mixed | 1 KiB | 128 | 842 MB/s | 815 MB/s | 534 MB/s | 0 / 1 / 24 (2 KB) |
| mixed | 16 KiB | 1 | 1.0 GB/s | 603 MB/s | 181 MB/s | 0 / 1 / 61 (40 KB) |
| mixed | 16 KiB | 128 | 8.1 GB/s | 4.8 GB/s | 2.3 GB/s | 0 / 1 / 61 (39 KB) |
| multibyte | 1 KiB | 1 | 111 MB/s | 106 MB/s | 55 MB/s | 0 / 1 / 24 (2 KB) |
| multibyte | 1 KiB | 128 | 825 MB/s | 792 MB/s | 536 MB/s | 0 / 1 / 24 (2 KB) |
| multibyte | 16 KiB | 1 | 895 MB/s | 517 MB/s | 189 MB/s | 0 / 1 / 61 (41 KB) |
| multibyte | 16 KiB | 128 | 7.4 GB/s | 4.7 GB/s | 2.3 GB/s | 0 / 1 / 61 (39 KB) |

## Reading the numbers

- ASCII payloads cost almost nothing to validate in any library: the standard library and ews both skip ASCII in word-sized steps, so these cells match the plain echo results.
- Non-ASCII payloads are where the validators differ. The standard library decodes rune by rune at 1 to 2 GB/s, the ews DFA runs at 2.5 GB/s, and the SIMD kernel at about 10 GB/s; gws also pays the pass twice per echo.
- A validation pass matters most on large messages over few connections, where it is a visible fraction of the round trip; at many connections the syscalls dominate again.

