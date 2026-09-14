package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// record is one benchmark line with its dimensions parsed.
type record struct {
	family  string
	dims    map[string]string // Includes "compress" when present.
	dimKeys []string          // Dimension names in the benchmark name's order.
	lib     string
	result  result
}

// chartSpec describes one SVG: a panel per value of one dimension, a column
// per listed value of another, and a bar per server in each. Other
// dimensions are fixed.
type chartSpec struct {
	file    string
	title   string
	panel   string
	column  string
	columns []string
	fixed   map[string]string
}

var charts = map[string][]chartSpec{
	"Echo": {
		{file: "echo-plain", title: "Echo, uncompressed", panel: "size", column: "conns", columns: []string{"1", "128", "2048"}, fixed: map[string]string{"compress": "false"}},
		{file: "echo-compressed", title: "Echo, compressed with context takeover", panel: "size", column: "conns", columns: []string{"1", "128", "2048"}, fixed: map[string]string{"compress": "true"}},
	},
	"Broadcast": {
		{file: "broadcast", title: "Broadcast, one message to every connection", panel: "conns", column: "compress", columns: []string{"false", "true"}},
	},
	"UTF8": {
		{file: "utf8-1conn", title: "Text echo with UTF-8 validation, one connection", panel: "kind", column: "size", columns: []string{"1024", "16384"}, fixed: map[string]string{"conns": "1"}},
		{file: "utf8-128conn", title: "Text echo with UTF-8 validation, 128 connections", panel: "kind", column: "size", columns: []string{"1024", "16384"}, fixed: map[string]string{"conns": "128"}},
	},
}

var colors = map[string]string{
	"ews":          "#50c878",
	"ews-shared":   "#5aa9e6",
	"ews-sync":     "#5aa9e6",
	"gws":          "#ef8354",
	"gws-stream":   "#f2c14e",
	"coder":        "#b388eb",
	"coder-stream": "#e76f9a",
}

const svgStyle = `text{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;fill:#d8dee9}.title{font-family:ui-sans-serif,system-ui,sans-serif;font-size:28px;font-weight:700}.subtitle{font-family:ui-sans-serif,system-ui,sans-serif;font-size:15px;fill:#aab4c3}.group{font-family:ui-sans-serif,system-ui,sans-serif;font-size:18px;font-weight:650}.contract{font-family:ui-sans-serif,system-ui,sans-serif;font-size:14px;font-weight:650;fill:#9eabbc}.series{font-size:13px;fill:#c4ccd8}.value{font-size:13px;font-weight:650}.panel{fill:#151c27}`

// writeCharts renders the family's charts into dir and returns the file
// names written, for embedding in the Markdown.
func writeCharts(dir, family, subtitle string, records []record) ([]string, error) {
	var files []string
	for _, spec := range charts[family] {
		svg := renderChart(spec, subtitle, records)
		if svg == "" {
			continue
		}
		name := spec.file + buildSuffix() + ".svg"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(svg), 0o644); err != nil {
			return nil, err
		}
		files = append(files, name)
	}
	return files, nil
}

func buildSuffix() string {
	if strings.Contains(os.Getenv("GOEXPERIMENT"), "simd") {
		return "-simd"
	}
	return ""
}

func renderChart(spec chartSpec, subtitle string, records []record) string {
	// Select records for this chart and collect panel values and servers.
	var sel []record
	var panels, libs []string
	seen := map[string]bool{}
	for _, r := range records {
		ok := true
		for k, v := range spec.fixed {
			if r.dims[k] != v {
				ok = false
			}
		}
		if !ok || r.dims[spec.panel] == "" || !contains(spec.columns, r.dims[spec.column]) {
			continue
		}
		sel = append(sel, r)
		if !seen["p"+r.dims[spec.panel]] {
			seen["p"+r.dims[spec.panel]] = true
			panels = append(panels, r.dims[spec.panel])
		}
		if !seen["l"+r.lib] {
			seen["l"+r.lib] = true
			libs = append(libs, r.lib)
		}
	}
	if len(sel) == 0 {
		return ""
	}
	sort.SliceStable(panels, func(i, j int) bool { return valueLess(panels[i], panels[j]) })
	sort.SliceStable(libs, func(i, j int) bool { return libRank(libs[i]) < libRank(libs[j]) })
	lookup := func(panel, col, lib string) (result, bool) {
		for _, r := range sel {
			if r.dims[spec.panel] == panel && r.dims[spec.column] == col && r.lib == lib {
				return r.result, true
			}
		}
		return result{}, false
	}

	const width = 1500
	ncol := len(spec.columns)
	colWidth := 1410 / ncol
	labelW := 150
	valueW := 190
	barMax := float64(colWidth - labelW - valueW - 30)
	panelH := 68 + len(libs)*25 + 15
	top := 145
	height := top + len(panels)*(panelH+16) + 35

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-labelledby="title desc">`+"\n", width, height, width, height)
	fmt.Fprintf(&b, "  <title id=\"title\">%s</title>\n", html.EscapeString(spec.title))
	fmt.Fprintf(&b, "  <desc id=\"desc\">%s Higher is better.</desc>\n", html.EscapeString(subtitle))
	fmt.Fprintf(&b, "  <style>%s</style>\n", svgStyle)
	fmt.Fprintf(&b, "  <rect width=\"%d\" height=\"%d\" rx=\"14\" fill=\"#10151d\"/>\n", width, height)
	fmt.Fprintf(&b, "  <text class=\"title\" x=\"40\" y=\"48\">%s</text>\n", html.EscapeString(spec.title))
	fmt.Fprintf(&b, "  <text class=\"subtitle\" x=\"40\" y=\"76\">%s · higher is better</text>\n", html.EscapeString(subtitle))
	x := 40
	for _, lib := range libs {
		fmt.Fprintf(&b, "  <rect x=\"%d\" y=\"94\" width=\"14\" height=\"14\" rx=\"3\" fill=\"%s\"/><text class=\"series\" x=\"%d\" y=\"106\">%s</text>\n", x, colors[lib], x+22, html.EscapeString(lib))
		x += 22 + 9*len(lib) + 40
	}
	fmt.Fprintf(&b, "  <text class=\"subtitle\" x=\"1460\" y=\"106\" text-anchor=\"end\">Each column uses its own linear scale; labels show throughput and allocations per message.</text>\n")

	for pi, panel := range panels {
		fmt.Fprintf(&b, "  <g transform=\"translate(0,%d)\">\n", top+pi*(panelH+16))
		fmt.Fprintf(&b, "    <rect class=\"panel\" x=\"25\" y=\"0\" width=\"1450\" height=\"%d\" rx=\"10\"/>\n", panelH)
		fmt.Fprintf(&b, "    <text class=\"group\" x=\"45\" y=\"27\">%s</text>\n", html.EscapeString(dimLabel(spec.panel, panel)))
		for ci, col := range spec.columns {
			x0 := 45 + ci*colWidth
			fmt.Fprintf(&b, "    <text class=\"contract\" x=\"%d\" y=\"53\">%s</text>\n", x0, html.EscapeString(dimLabel(spec.column, col)))
			var maxv float64
			for _, lib := range libs {
				if r, ok := lookup(panel, col, lib); ok && score(r) > maxv {
					maxv = score(r)
				}
			}
			for li, lib := range libs {
				r, ok := lookup(panel, col, lib)
				if !ok || maxv == 0 {
					continue
				}
				y := 68 + li*25
				w := barMax * score(r) / maxv
				bx := x0 + labelW + 12
				fmt.Fprintf(&b, "    <text class=\"series\" x=\"%d\" y=\"%d\" text-anchor=\"end\">%s</text>\n", bx-12, y+13, html.EscapeString(lib))
				fmt.Fprintf(&b, "    <rect x=\"%d\" y=\"%d\" width=\"%.1f\" height=\"17\" rx=\"4\" fill=\"%s\"/>\n", bx, y, w, colors[lib])
				fmt.Fprintf(&b, "    <text class=\"value\" x=\"%.1f\" y=\"%d\">%s · %s</text>\n", float64(bx)+w+8, y+13, html.EscapeString(throughput(r)), allocLabel(r.allocs))
			}
		}
		b.WriteString("  </g>\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// score is the bar length metric: messages per second when reported, else bytes per second.
func score(r result) float64 {
	if r.msgs > 0 {
		return r.msgs
	}
	return r.mbs
}

func allocLabel(n int) string {
	if n == 1 {
		return "1 alloc"
	}
	return strconv.Itoa(n) + " allocs"
}

func dimLabel(dim, value string) string {
	n, err := strconv.Atoi(value)
	switch dim {
	case "size":
		if err == nil && n >= 1024 {
			return fmt.Sprintf("%d KiB messages", n/1024)
		}
		return value + " B messages"
	case "conns":
		if value == "1" {
			return "1 connection"
		}
		return value + " connections"
	case "compress":
		if value == "true" {
			return "Compressed"
		}
		return "Uncompressed"
	case "kind":
		switch value {
		case "ascii":
			return "ASCII"
		case "mixed":
			return "Mixed, JSON with Japanese values"
		case "multibyte":
			return "Multibyte, Japanese prose"
		}
	}
	return value
}

func valueLess(a, b string) bool {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil {
		return an < bn
	}
	return false // Keep first-seen order for names.
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
