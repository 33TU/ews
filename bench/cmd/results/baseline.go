package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// checkBaseline compares this run's cells with the raw output committed at
// HEAD for the same package and build, and warns on stderr when cells moved
// by more than a quarter either way. A cluster of such cells in one run
// usually means the machine was busy, not that the code changed.
func checkBaseline(records []record) {
	prefix := run("git", "rev-parse", "--show-prefix")
	if prefix == "unknown" {
		return
	}
	name := "raw" + buildSuffix() + ".txt"
	out, err := exec.Command("git", "show", "HEAD:"+prefix+name).Output()
	if err != nil {
		return // First run for this package, or not a repository.
	}
	old := map[string]float64{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		m := line.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		r := result{mbs: metric(m[4], "MB/s"), msgs: metric(m[4], "msgs/s")}
		old[m[1]+"/"+m[2]] = score(r)
	}
	if len(old) == 0 {
		return
	}
	var moved []string
	var ratios []float64
	for _, r := range records {
		key := r.family + "/" + cellName(r)
		prev, ok := old[key]
		cur := score(r.result)
		if !ok || prev == 0 || cur == 0 {
			continue
		}
		ratio := cur / prev
		ratios = append(ratios, ratio)
		if ratio > 1.25 || ratio < 0.8 {
			moved = append(moved, fmt.Sprintf("  %-70s %+.0f%%", key, (ratio-1)*100))
		}
	}
	if len(ratios) == 0 {
		return
	}
	sort.Float64s(ratios)
	median := ratios[len(ratios)/2]
	if len(moved) == 0 {
		fmt.Fprintf(os.Stderr, "results: %d cells within 25%% of the committed run (median %+.1f%%)\n", len(ratios), (median-1)*100)
		return
	}
	fmt.Fprintf(os.Stderr, "results: %d of %d cells moved more than 25%% against the committed %s (median %+.1f%%); a cluster of these usually means the machine was busy:\n", len(moved), len(ratios), name, (median-1)*100)
	for _, l := range moved {
		fmt.Fprintln(os.Stderr, l)
	}
}

// cellName rebuilds the benchmark name after the family, in the input's
// dimension order, so it matches the raw line.
func cellName(r record) string {
	var parts []string
	for _, k := range r.dimKeys {
		parts = append(parts, k+"="+r.dims[k])
	}
	return strings.Join(parts, "/") + "/" + r.lib
}
