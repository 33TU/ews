// Command results turns benchmark output into a Markdown results file.
//
//	go test -run '^$' -bench . -benchtime 1s | go run ../cmd/results > RESULTS.md
//
// It understands names of the form Benchmark<Family>/key=value/.../<server>,
// renders one table per compression mode with servers as columns, and adds
// the notes registered for the family.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type key struct {
	family string
	dims   string // Row dimensions joined, excluding compress.
	comp   string // compress value, or "".
	lib    string
}

type result struct {
	ns, mbs, msgs float64
	bytes, allocs int
}

type family struct {
	title, setup, reading string
}

var families = map[string]family{
	"Echo": {
		title: "Echo benchmark results",
		setup: `Echo servers behind ` + "`httptest`" + ` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- ` + "`ews`" + `: ` + "`ws.Conn`" + ` with ` + "`ReadMessage`" + ` and ` + "`Write`" + `, default 4 KiB read buffer. With compression it keeps a compressor attached per connection.
- ` + "`ews-shared`" + `: the same with ` + "`CompressionShared`" + `, borrowing a pooled compressor per message as gws and coder do. Takeover table only; it is identical to ` + "`ews`" + ` otherwise.
- ` + "`ews-stream`" + `: ` + "`NextMessage`" + ` then ` + "`WriteFrom`" + ` reading the connection itself, so no message is held whole; ews's streaming shape, against ` + "`gws-stream`" + ` and ` + "`coder-stream`" + `.
- ` + "`gws`" + `: gws's ` + "`ReadMessage`" + ` and ` + "`WriteMessage`" + ` in a loop, the like-for-like shape against ews. Its event-driven ` + "`ReadLoop`" + ` shares the frame path and measured the same within noise.
- ` + "`gws-stream`" + `: gws's ` + "`NextReader`" + ` piped into ` + "`WriteFile`" + `, so no message is held whole.
- ` + "`coder`" + `: coder/websocket with ` + "`Read`" + ` and ` + "`Write`" + ` in a loop.
- ` + "`coder-stream`" + `: coder/websocket piping ` + "`Reader`" + ` into ` + "`Writer`" + ` through a reusable buffer, so no message is held whole.
- ` + "`gorilla`" + `: gorilla/websocket with ` + "`ReadMessage`" + ` and ` + "`WriteMessage`" + ` in a loop. Uncompressed and no-takeover tables only, since gorilla negotiates only ` + "`no_context_takeover`" + `.
- ` + "`gorilla-stream`" + `: gorilla/websocket piping ` + "`NextReader`" + ` into ` + "`NextWriter`" + ` through a reusable buffer.

Compression is permessage-deflate at flate level 1, every message compressed, in two modes that are separate tables because they are different work. With context takeover each direction keeps a 32 KB history that every message extends, so the inflater is primed with a dictionary per message and both ends copy history; it compresses real traffic far better. Without takeover each message is compressed on its own. gws is configured for 15-bit windows to match the 32 KB window ews uses, since its default is 12 bits. coder/websocket uses its fixed level and pooled flate readers and writers, with its compression threshold lowered so that, like the others, it compresses every message. gorilla/websocket uses the standard library's flate. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.
`,
		reading: `- Small messages are bound by loopback round trips, so all servers tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews and the streaming variants. gws's and coder's simple read APIs allocate a buffer above their pool thresholds on every such message.
- With hundreds of connections and 256 KiB messages every library is bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- coder's documented ` + "`Read`" + ` assembles messages through ` + "`io.ReadAll`" + `, which dominates its large-message cells; piping ` + "`Reader`" + ` into ` + "`Writer`" + ` is 2 to 4 times faster there and is the fairer comparison for large messages, though slightly slower on small ones.
`,
	},
	"UTF8": {
		title: "Text validation benchmark results",
		setup: `Text echo across payload kinds, timed like the echo benchmark: one ping-pong at a time per connection, throughput in payload bytes one way. Servers validate UTF-8 where the library offers it, so the difference between columns is the validation pass.

- ` + "`ews`" + `: ` + "`ReadMessage`" + ` and ` + "`Write`" + ` with ` + "`ValidateUTF8`" + ` on: one pass over each received text message, a shift-based DFA by default or SIMD lookups under ` + "`GOEXPERIMENT=simd`" + `, after skipping the ASCII prefix with word loads.
- ` + "`gws`" + `: ` + "`ReadMessage`" + ` and ` + "`WriteMessage`" + ` with ` + "`CheckUtf8Enabled`" + `, which runs the standard library's ` + "`utf8.Valid`" + ` on received text and on outgoing text as well, so an echo validates twice.
- ` + "`coder`" + `: ` + "`Read`" + ` and ` + "`Write`" + `; coder/websocket has no UTF-8 validation to enable, so this column is the no-validation baseline.

Payloads are JSON-like ASCII, JSON with Japanese values (mixed), and Japanese prose (multibyte), cut on rune boundaries. Clients are ews connections without validation, so the client side costs the same for every server.
`,
		reading: `- ASCII payloads cost almost nothing to validate in any library: the standard library and ews both skip ASCII in word-sized steps, so these cells match the plain echo results.
- Non-ASCII payloads are where the validators differ. The standard library decodes rune by rune at 1 to 2 GB/s, the ews DFA runs at 2.5 GB/s, and the SIMD kernel at about 10 GB/s; gws also pays the pass twice per echo.
- A validation pass matters most on large messages over few connections, where it is a visible fraction of the round trip; at many connections the syscalls dominate again.
`,
	},
	"Broadcast": {
		title: "Broadcast benchmark results",
		setup: `One message of 256 bytes, 4 KiB or 64 KiB delivered to every connected client, timed until all clients have received it. Servers run behind ` + "`httptest`" + ` on loopback TCP and every client is the same ews reader, so the read side costs the same for all servers and differences come from the broadcast path. Throughput is in messages delivered per second; allocations are process-wide per round.

- ` + "`ews`" + `: ` + "`Prepare`" + ` once, then ` + "`SendPrepared`" + ` on each connection's ` + "`Queue`" + `, returning before the writes complete.
- ` + "`ews-sync`" + `: ` + "`Prepare`" + ` once, then ` + "`WritePrepared`" + ` on each connection in a loop, waiting for each write.
- ` + "`gws`" + `: ` + "`NewBroadcaster`" + ` once, then ` + "`Broadcast`" + ` on each connection through its per-connection worker.
- ` + "`gorilla`" + `: ` + "`NewPreparedMessage`" + ` once, then ` + "`WritePreparedMessage`" + ` on each connection in a loop, waiting for each write; gorilla has no asynchronous send, so compare it with ` + "`ews-sync`" + `. Uncompressed and no-takeover tables only, the modes gorilla supports.

Compression is permessage-deflate at flate level 1 with 15-bit windows, with and without context takeover as separate tables since they are different work; ews servers use ` + "`CompressionShared`" + `, the mode meant for many connections. Every server reads through its own ` + "`ReadMessage`" + `. Servers run in a seeded shuffled order within each cell and get twenty warm-up rounds before timing, since a server measured right after connecting thousands of clients read 10 to 20 percent low.
`,
		reading: `- Uncompressed, a round is one write per connection and one read per client, and the kernel's cost for those dominates; the asynchronous paths tie at that floor, and only the allocation counts differ.
- A synchronous loop serializes every write on one goroutine, so it trails the queued paths by several times as connections grow.
- Compressed, the message is compressed once in both libraries and only the per-connection history update and the clients' decompression remain. gws copies the payload into each connection's window in its worker; ews defers that copy until the connection next compresses a message of its own, which a broadcast-only recipient never does, so the sender's loop stays short at every payload size.
`,
	},
}

var line = regexp.MustCompile(`^Benchmark(\w+)/(\S+?)-\d+\s+\d+\s+([\d.]+) ns/op(.*)$`)

func main() {
	benchtime := flag.String("benchtime", "1s", "the -benchtime the results were produced with, for the header")
	svgDir := flag.String("svg", "", "directory to write SVG charts into, embedded in the Markdown when set")
	flag.Parse()
	rows := map[key]result{}
	var records []record
	var famOrder []string
	dimOrder := map[string][]string{}  // family -> row dims in first-seen order
	libOrder := map[string][]string{}  // family+comp -> libs in first-seen order
	rowLabels := map[string][]string{} // family+comp -> row dims joined, first-seen order
	seen := map[string]bool{}

	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		m := line.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		fam, name, ns, rest := m[1], m[2], m[3], m[4]
		segs := strings.Split(name, "/")
		if strings.Contains(segs[len(segs)-1], "=") {
			continue // No server column; not a comparison.
		}
		lib := segs[len(segs)-1]
		comp := ""
		var dims []string
		for _, s := range segs[:len(segs)-1] {
			k, v, _ := strings.Cut(s, "=")
			if k == "compress" {
				comp = v
				continue
			}
			dims = append(dims, s)
		}
		k := key{fam, strings.Join(dims, "/"), comp, lib}
		dimMap := map[string]string{}
		var dimKeys []string
		for _, s := range segs[:len(segs)-1] {
			dk, dv, _ := strings.Cut(s, "=")
			dimMap[dk] = dv
			dimKeys = append(dimKeys, dk)
		}
		var r result
		r.ns, _ = strconv.ParseFloat(ns, 64)
		r.mbs = metric(rest, "MB/s")
		r.msgs = metric(rest, "msgs/s")
		r.bytes = int(metric(rest, "B/op"))
		r.allocs = int(metric(rest, "allocs/op"))
		rows[k] = r
		records = append(records, record{fam, dimMap, dimKeys, lib, r})
		if !seen[fam] {
			seen[fam] = true
			famOrder = append(famOrder, fam)
		}
		if dimOrder[fam] == nil {
			for _, d := range dims {
				kk, _, _ := strings.Cut(d, "=")
				dimOrder[fam] = append(dimOrder[fam], kk)
			}
		}
		fc := fam + "/" + comp
		if !seen["l/"+fc+"/"+lib] {
			seen["l/"+fc+"/"+lib] = true
			libOrder[fc] = append(libOrder[fc], lib)
		}
		if !seen["r/"+fc+"/"+k.dims] {
			seen["r/"+fc+"/"+k.dims] = true
			rowLabels[fc] = append(rowLabels[fc], k.dims)
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no benchmark lines found on stdin")
		os.Exit(1)
	}
	checkBaseline(records)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for _, fam := range famOrder {
		f := families[fam]
		if f.title == "" {
			f.title = fam + " benchmark results"
		}
		fmt.Fprintf(w, "# %s\n\nGenerated %s from `go test -run '^$' -bench %s -benchtime %s | go run ../cmd/results` at ews commit `%s`.\n\n",
			f.title, time.Now().Format("2006-01-02"), fam, *benchtime, run("git", "rev-parse", "--short", "HEAD"))
		build := "default build: SWAR masking and the shift-based UTF-8 validator"
		short := "default build"
		if strings.Contains(os.Getenv("GOEXPERIMENT"), "simd") {
			build = "`GOEXPERIMENT=simd`: SIMD masking and, on amd64, SIMD UTF-8 validation"
			short = "GOEXPERIMENT=simd"
		}
		if *svgDir != "" {
			subtitle := fmt.Sprintf("%s · %s · %s · %s benchtime", runtime.Version(), short, shortCPU(cpu()), *benchtime)
			var famRecords []record
			for _, r := range records {
				if r.family == fam {
					famRecords = append(famRecords, r)
				}
			}
			files, err := writeCharts(*svgDir, fam, subtitle, famRecords)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			for _, name := range files {
				fmt.Fprintf(w, "![%s](%s)\n\n", strings.TrimSuffix(name, ".svg"), name)
			}
		}
		fmt.Fprintf(w, "## Setup\n\n- CPU: %s\n- Kernel: %s\n- Go: %s, %s\n- gws: %s\n- coder/websocket: %s\n- gorilla/websocket: %s\n\n%s\n", cpu(), run("uname", "-r"), runtime.Version(), build, modVersion("lxzan/gws"), modVersion("coder/websocket"), modVersion("gorilla/websocket"), f.setup)
		comps := []string{""}
		if _, ok := rowLabels[fam+"/false"]; ok {
			comps = comps[:0]
			for _, c := range []string{"false", "true", "nocontext"} {
				if _, ok := rowLabels[fam+"/"+c]; ok {
					comps = append(comps, c)
				}
			}
		}
		for _, comp := range comps {
			fc := fam + "/" + comp
			libs := libOrder[fc]
			sort.SliceStable(libs, func(i, j int) bool { return libRank(libs[i]) < libRank(libs[j]) })
			title := "Results"
			switch comp {
			case "false":
				title = "Uncompressed"
			case "true":
				title = "Compressed with context takeover"
			case "nocontext":
				title = "Compressed without context takeover"
			}
			heads := make([]string, len(dimOrder[fam]))
			for i, d := range dimOrder[fam] {
				heads[i] = strings.ToUpper(d[:1]) + d[1:]
			}
			fmt.Fprintf(w, "## %s\n\n| %s | %s | allocs/op %s |\n|%s%s---|\n", title, strings.Join(heads, " | "), strings.Join(libs, " | "), strings.Join(libs, " / "), strings.Repeat("---|", len(heads)), strings.Repeat("---|", len(libs)))
			labels := rowLabels[fc]
			sort.SliceStable(labels, func(i, j int) bool { return dimLess(labels[i], labels[j]) })
			for _, label := range labels {
				var cells, allocs []string
				for _, lib := range libs {
					r, ok := rows[key{fam, label, comp, lib}]
					if !ok {
						cells, allocs = append(cells, "n/a"), append(allocs, "n/a")
						continue
					}
					cells = append(cells, throughput(r))
					a := strconv.Itoa(r.allocs)
					if r.bytes >= 1024 {
						a += fmt.Sprintf(" (%d KB)", r.bytes/1024)
					}
					allocs = append(allocs, a)
				}
				fmt.Fprintf(w, "| %s | %s | %s |\n", rowCells(label), strings.Join(cells, " | "), strings.Join(allocs, " / "))
			}
			fmt.Fprintln(w)
		}
		if f.reading != "" {
			fmt.Fprintf(w, "## Reading the numbers\n\n%s\n", f.reading)
		}
	}
}

func metric(rest, unit string) float64 {
	m := regexp.MustCompile(`([\d.]+) ` + regexp.QuoteMeta(unit)).FindStringSubmatch(rest)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

// throughput prefers the benchmark's own metric, messages per second, over
// the bytes-per-second figure SetBytes derives.
func throughput(r result) string {
	switch {
	case r.msgs >= 1e6:
		return fmt.Sprintf("%.2fM msgs/s", r.msgs/1e6)
	case r.msgs > 0:
		return fmt.Sprintf("%.0fk msgs/s", r.msgs/1e3)
	case r.mbs >= 1000:
		return fmt.Sprintf("%.1f GB/s", r.mbs/1000)
	case r.mbs > 0:
		return fmt.Sprintf("%.0f MB/s", r.mbs)
	default:
		return fmt.Sprintf("%.0f µs", r.ns/1000)
	}
}

// rowCells renders "size=16384/conns=32" as "16 KiB | 32".
func rowCells(label string) string {
	var out []string
	for _, d := range strings.Split(label, "/") {
		k, v, _ := strings.Cut(d, "=")
		if n, err := strconv.Atoi(v); err == nil && k == "size" {
			if n < 1024 {
				v = fmt.Sprintf("%d B", n)
			} else {
				v = fmt.Sprintf("%d KiB", n/1024)
			}
		}
		out = append(out, v)
	}
	return strings.Join(out, " | ")
}

// dimLess orders rows numerically dimension by dimension.
func dimLess(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := range as {
		if i >= len(bs) {
			return false
		}
		_, av, _ := strings.Cut(as[i], "=")
		_, bv, _ := strings.Cut(bs[i], "=")
		an, aerr := strconv.Atoi(av)
		bn, berr := strconv.Atoi(bv)
		switch {
		case aerr == nil && berr == nil && an != bn:
			return an < bn
		case av != bv:
			return av < bv
		}
	}
	return false
}

// libRank orders server columns: ews first, then each library with its
// variants beside it; unknown names go last in input order.
func libRank(name string) int {
	for i, known := range []string{"ews", "ews-shared", "ews-stream", "ews-sync", "gws", "gws-stream", "coder", "coder-stream", "gorilla", "gorilla-stream"} {
		if name == known {
			return i
		}
	}
	return 100
}

func run(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// shortCPU trims vendor boilerplate from a CPU model name for chart subtitles.
func shortCPU(name string) string {
	for _, junk := range []string{"(R)", "(TM)", "CPU", "Processor", "Core "} {
		name = strings.ReplaceAll(name, junk, "")
	}
	return strings.Join(strings.Fields(name), " ")
}

func cpu() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return runtime.GOARCH
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "model name") {
			_, v, _ := strings.Cut(l, ":")
			return strings.TrimSpace(v)
		}
	}
	return runtime.GOARCH
}

// modVersion reads a dependency's version from the module's go.mod, looked
// for in the working directory and its parent.
func modVersion(mod string) string {
	for _, p := range []string{"go.mod", "../go.mod"} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(data), "\n") {
			if strings.Contains(l, mod) {
				f := strings.Fields(l)
				return f[len(f)-1]
			}
		}
	}
	return "unknown"
}
