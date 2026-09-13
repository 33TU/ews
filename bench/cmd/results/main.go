// Command results turns the output of the echo benchmark into RESULTS.md.
//
//	go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results > RESULTS.md
package main

import (
	"bufio"
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
	compress bool
	size     int
	conns    int
	lib      string
}

type result struct {
	mbs    float64
	bytes  int
	allocs int
}

var line = regexp.MustCompile(`^BenchmarkEcho/compress=(\w+)/size=(\d+)/conns=(\d+)/([\w-]+)-\d+\s+\d+\s+[\d.]+ ns/op\s+([\d.]+) MB/s\s+(\d+) B/op\s+(\d+) allocs/op`)

func main() {
	rows := map[key]result{}
	var sizes, conns []int
	var libs []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		m := line.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		k := key{m[1] == "true", atoi(m[2]), atoi(m[3]), m[4]}
		mbs, _ := strconv.ParseFloat(m[5], 64)
		rows[k] = result{mbs, atoi(m[6]), atoi(m[7])}
		if !seen["s"+m[2]] {
			seen["s"+m[2]] = true
			sizes = append(sizes, k.size)
		}
		if !seen["c"+m[3]] {
			seen["c"+m[3]] = true
			conns = append(conns, k.conns)
		}
		if !seen["l"+m[4]] {
			seen["l"+m[4]] = true
			libs = append(libs, k.lib)
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no benchmark lines found on stdin")
		os.Exit(1)
	}
	sort.Ints(sizes)
	sort.Ints(conns)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	fmt.Fprintf(w, "# Echo benchmark results\n\nGenerated %s from `go test -run '^$' -bench . -benchtime=1s | go run ./cmd/results` at ews commit `%s`.\n\n",
		time.Now().Format("2006-01-02"), run("git", "rev-parse", "--short", "HEAD"))
	fmt.Fprintf(w, "## Setup\n\n- CPU: %s\n- Kernel: %s\n- Go: %s\n- gws: %s\n\n", cpu(), run("uname", "-r"), runtime.Version(), gwsVersion())
	fmt.Fprint(w, setup)
	for _, compress := range []bool{false, true} {
		title := "Uncompressed"
		if compress {
			title = "Compressed"
		}
		fmt.Fprintf(w, "## %s\n\n| Size | Conns | %s | allocs/op %s |\n|---|---|%s---|\n", title, strings.Join(libs, " | "), strings.Join(libs, " / "), strings.Repeat("---|", len(libs)))
		for _, size := range sizes {
			for _, c := range conns {
				var cells, allocs []string
				for _, lib := range libs {
					r, ok := rows[key{compress, size, c, lib}]
					if !ok {
						cells, allocs = append(cells, "n/a"), append(allocs, "n/a")
						continue
					}
					cells = append(cells, throughput(r.mbs))
					a := strconv.Itoa(r.allocs)
					if r.bytes >= 1024 {
						a += fmt.Sprintf(" (%d KB)", r.bytes/1024)
					}
					allocs = append(allocs, a)
				}
				fmt.Fprintf(w, "| %s | %d | %s | %s |\n", sizeLabel(size), c, strings.Join(cells, " | "), strings.Join(allocs, " / "))
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprint(w, reading)
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func throughput(mbs float64) string {
	if mbs >= 1000 {
		return fmt.Sprintf("%.1f GB/s", mbs/1000)
	}
	return fmt.Sprintf("%.0f MB/s", mbs)
}

func sizeLabel(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%d KiB", n/1024)
}

func run(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
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

func gwsVersion() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, "lxzan/gws") {
			f := strings.Fields(l)
			return f[len(f)-1]
		}
	}
	return "unknown"
}

const setup = `Echo servers behind ` + "`httptest`" + ` on loopback TCP, all driven by the same ews client, one ping-pong at a time per connection. Throughput counts payload bytes in one direction per round trip. Allocations are process-wide per message; the ews client allocates nothing, so they are effectively the server's.

- ` + "`ews`" + `: ` + "`ws.Conn`" + ` with ` + "`ReadMessage`" + ` and ` + "`Write`" + `, default 4 KiB read buffer.
- ` + "`gws`" + `: event-driven ` + "`ReadLoop`" + ` with an ` + "`OnMessage`" + ` echo, gws's documented server shape.
- ` + "`gws-pull`" + `: gws's ` + "`ReadMessage`" + ` in a loop, the like-for-like shape against ews.

Compression is permessage-deflate at flate level 1 with context takeover in both directions. gws is configured for 15-bit windows to match the 32 KB window ews uses; its default is 12 bits, which ews does not implement. Compressed payloads are repeated JSON-like text; uncompressed payloads are random bytes.

Single-connection small-message cells are loopback round trips of 12 to 15 µs and vary by 10 to 20 percent between runs. Large-message and allocation figures are stable. Beyond the machine's thread count, more connections measure scheduling and per-connection overhead rather than parallelism.

`

const reading = `## Reading the numbers

- Small messages are bound by loopback round trips, so all three tie uncompressed. Compressed, ews leads because its deflate path allocates nothing and reuses pooled or per-connection helpers.
- 16 KiB frames exceed the 4 KiB read buffer. ews reads the remainder straight into the message buffer, so both libraries do two reads and one copy, and they tie.
- Large messages favor ews. gws allocates a buffer above its pool threshold on every such message, over half a megabyte uncompressed and over a megabyte compressed.
- With hundreds of connections and 256 KiB messages both libraries are bound by memory bandwidth, with a quarter-megabyte buffer per connection in flight on each side.
- gws's ` + "`ReadLoop`" + ` and ` + "`ReadMessage`" + ` share the whole frame path and measure the same within noise.
`
