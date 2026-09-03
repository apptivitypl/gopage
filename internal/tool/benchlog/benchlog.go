package benchlog

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
)

type Result struct {
	NsPerOp     float64 `json:"ns_per_op"`
	BytesPerOp  int64   `json:"bytes_per_op"`
	AllocsPerOp int64   `json:"allocs_per_op"`
}

type Record struct {
	Benchmarks    map[string]Result `json:"benchmarks"`
	WorkerSize    int               `json:"worker_module_brotli_bytes,omitempty"`
	ClientRuntime int               `json:"client_runtime_brotli_bytes,omitempty"`
	ClientChunks  int               `json:"client_chunks_brotli_bytes,omitempty"`
}

const RegressionTolerance = 0.10

func Parse(output string) map[string]Result {
	results := map[string]Result{}
	for line := range strings.SplitSeq(output, "\n") {
		name, result, ok := parseLine(line)
		if ok {
			results[name] = result
		}
	}
	return results
}

func parseLine(line string) (string, Result, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") {
		return "", Result{}, false
	}
	var result Result
	var seen bool
	for i := 1; i+1 < len(fields); i++ {
		value := fields[i]
		switch fields[i+1] {
		case "ns/op":
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				result.NsPerOp = parsed
				seen = true
			}
		case "B/op":
			if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
				result.BytesPerOp = parsed
			}
		case "allocs/op":
			if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
				result.AllocsPerOp = parsed
			}
		}
	}
	if !seen {
		return "", Result{}, false
	}
	return trimCPUSuffix(fields[0]), result, true
}

func trimCPUSuffix(name string) string {
	if index := strings.LastIndex(name, "-"); index > 0 {
		if _, err := strconv.Atoi(name[index+1:]); err == nil {
			return name[:index]
		}
	}
	return name
}

func Compare(baseline, current Record) []string {
	var regressions []string
	for _, name := range sortedNames(baseline.Benchmarks) {
		before := baseline.Benchmarks[name]
		after, ok := current.Benchmarks[name]
		if !ok {
			regressions = append(regressions, fmt.Sprintf("%s: missing from this run", name))
			continue
		}
		if after.AllocsPerOp > before.AllocsPerOp {
			regressions = append(regressions, fmt.Sprintf(
				"%s: %d allocs/op, up from %d", name, after.AllocsPerOp, before.AllocsPerOp))
		}
	}
	sizes := []struct {
		label  string
		before int
		after  int
	}{
		{"worker module", baseline.WorkerSize, current.WorkerSize},
		{"client runtime", baseline.ClientRuntime, current.ClientRuntime},
		{"client chunks", baseline.ClientChunks, current.ClientChunks},
	}
	for _, size := range sizes {
		if size.before > 0 && size.after > 0 && float64(size.after) > float64(size.before)*(1+RegressionTolerance) {
			regressions = append(regressions, fmt.Sprintf(
				"%s: %d bytes after brotli, up from %d", size.label, size.after, size.before))
		}
	}
	return regressions
}

func Slower(baseline, current Record) []string {
	var observations []string
	for _, name := range sortedNames(baseline.Benchmarks) {
		before := baseline.Benchmarks[name]
		after, ok := current.Benchmarks[name]
		if !ok || before.NsPerOp <= 0 {
			continue
		}
		if after.NsPerOp > before.NsPerOp*(1+RegressionTolerance) {
			observations = append(observations, fmt.Sprintf(
				"%s: %.2f ns/op, up from %.2f (tolerance %.0f%%)",
				name, after.NsPerOp, before.NsPerOp, RegressionTolerance*100))
		}
	}
	return observations
}

func Merge(baseline, current Record) Record {
	merged := Record{
		Benchmarks:    map[string]Result{},
		WorkerSize:    keep(current.WorkerSize, baseline.WorkerSize),
		ClientRuntime: keep(current.ClientRuntime, baseline.ClientRuntime),
		ClientChunks:  keep(current.ClientChunks, baseline.ClientChunks),
	}
	maps.Copy(merged.Benchmarks, baseline.Benchmarks)
	for name, result := range current.Benchmarks {
		best, ok := merged.Benchmarks[name]
		if !ok || result.NsPerOp < best.NsPerOp {
			merged.Benchmarks[name] = result
		}
	}
	return merged
}

func keep(current, fallback int) int {
	if current == 0 {
		return fallback
	}
	return current
}

func Format(record Record) string {
	var b strings.Builder
	for _, name := range sortedNames(record.Benchmarks) {
		result := record.Benchmarks[name]
		fmt.Fprintf(&b, "%-28s %10.2f ns/op %8d B/op %6d allocs/op\n",
			name, result.NsPerOp, result.BytesPerOp, result.AllocsPerOp)
	}
	sizes := []struct {
		label string
		value int
	}{
		{"worker module", record.WorkerSize},
		{"client runtime", record.ClientRuntime},
		{"client chunks", record.ClientChunks},
	}
	for _, size := range sizes {
		if size.value > 0 {
			fmt.Fprintf(&b, "%-28s %10d bytes after brotli\n", size.label, size.value)
		}
	}
	return b.String()
}

func sortedNames(results map[string]Result) []string {
	names := make([]string, 0, len(results))
	for name := range results {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
