# Benchmarks

Two sets. The `echo`, `broadcast` and `utf8` packages here compare ews with
gws, coder/websocket and gorilla/websocket under `go test -bench`, with the
raw output, generated tables and charts committed:
[echo](echo/RESULTS.md), [broadcast](broadcast/RESULTS.md),
[text validation](utf8/RESULTS.md), each with a `-simd` twin built with
`GOEXPERIMENT=simd`. The second set is a fork of lxzan's
[go-websocket-benchmark](https://github.com/33TU/go-websocket-benchmark),
the harness behind gws's published chart, with current library versions and
an `ews` server added; its
[results](https://github.com/33TU/go-websocket-benchmark/tree/main/results)
cover the whole field at 10k and 30k connections, with and without TLS.

Run the local set with `just bench-echo`, `just bench-broadcast` or
`just bench-utf8` from the repository root. The recipes pin `GOMAXPROCS=8`
so results from different machines measure the same shape, and each results
file records the thread count and library versions. The committed tables
were run pinned to the eight cores of one die (`taskset -c 0-7`), so the
threads share one L3. Nothing else should run during a benchmark; a
concurrent build or test run skews the numbers. The recipes write the Go
version, kernel and commit as `#` lines at the top of the raw output, and the
results tool reads those and the `cpu:` line back, so `just results` on any
machine regenerates the tables and charts with the labels of the run that
produced them.

## Echo

By payload size on 128 connections, one round trip at a time per connection,
each size scaled to its fastest server:

![Echo by payload size, uncompressed](echo/echo-sizes.svg)

![Echo by payload size, compressed](echo/echo-sizes-compressed.svg)

![Echo by payload size, compressed without context takeover](echo/echo-sizes-nocontext.svg)

Uncompressed echo is bounded by the kernel round trip and every well-built
library ties on it up to 16 KiB, within a few percent of a raw TCP echo; at
256 KiB gws, coder and gorilla allocate a frame per message above their
pooled buffer sizes and ews does not. Compressed, ews leads at every size.
The full tables by connection count:

![Echo, compressed](echo/echo-compressed.svg)

![Echo, compressed without context takeover](echo/echo-nocontext.svg)

With context takeover ews keeps a compressor attached per connection,
which is fastest per message; `ews-shared` borrows a pooled one per message
as gws and coder do, which costs more CPU per message and almost no memory.
The no-takeover table is the fair comparison for gorilla, which negotiates
only that mode.

## Broadcast

One message to many connections, each behind its own `Queue`:

![Broadcast, compressed](broadcast/broadcast-compressed.svg)

`Prepare` encodes and compresses the frame once and `SendPrepared` queues it
by reference, so a delivery allocates nothing and the cost per recipient is
the enqueue. A queue per connection means a slow or dead client blocks only
its own writer, and the queue's limit turns a backlog into `ErrQueueFull` on
that connection instead of a stall, so the hub decides what to do with a
client that cannot keep up without waiting for it. Below the machine's
bandwidth ceiling this costs less CPU per delivery than writing in a loop.
At the ceiling, large frames to thousands of clients at once, it costs
more, because thousands of writers wait on memory together; that is the
price of never letting one client slow another. A loop of `WritePrepared`
over the connections is cheaper there only by giving that isolation up: it
stalls on the first slow socket.

## Text validation

`ValidateUTF8` costs one pass over each text message. The SWAR validator is
the default build; `GOEXPERIMENT=simd` enables the vector one:

![Text validation, 128 connections](utf8/utf8-128conn.svg)

![Text validation, 128 connections, built with GOEXPERIMENT=simd](utf8/utf8-128conn-simd.svg)

## The go-websocket-benchmark fork

Echo ties there as it does here. In the rate test, where each connection is
offered 200 messages per second in 16 KiB batches, ews takes the whole
offered load with no drops at 1.8 times gws's echoes per CPU point, because
the Queue coalesces each connection's backlog into one writev; the
synchronous `ews_sync` server, which writes each echo before the next
read, ties with gws, so the lead is the write side.

At 256 KiB payloads the loopback link is the ceiling, about 4.2 GB/s each
way, and the libraries separate on what it costs them to reach it. ews
holds a message only until the next read and returns the buffer to a pool
bounded at 1 MiB per buffer, so its memory follows what is in flight rather
than the connection count. gws allocates a frame per message above its
largest pooled buffer class, which a 256 KiB payload just exceeds; at
128 KiB the two tie completely. gorilla, quickws and nettyws hold gigabytes
at 10k connections, so their memory scales with the connection count times
the message size.

Over TLS the record buffers level the memory column and encryption takes a
quarter off everyone's echo rate; ews and gws then tie within three percent
at 1 KiB, and ews still moves 256 KiB messages on the least CPU. In the rate
test over TLS, ews is the only one of the three that answers the whole
offered load with nothing dropped, on 341 percent CPU against gws's 445,
1.36 times the echoes per CPU point rather than the 1.8 of the plain run:
encryption is a cost none of them avoid, and it narrows the gap while
pushing the other two past what the client will keep sending them. The
fork's results directory has the TLS tables.

Above about 30,000 connections the harness's client, built on nbio, becomes
the bottleneck and the rate test stops measuring the server; the results
quote 10k and 30k and treat 50k as a stress test.
