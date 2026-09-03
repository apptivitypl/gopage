package benchlog

import (
	"strings"
	"testing"
)

const output = `goos: darwin
BenchmarkRender-14           	38377327	        31.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkAppendEscaped-14    	43970406	        26.23 ns/op	      16 B/op	       1 allocs/op
PASS
`

func TestParseReadsEveryBenchmark(t *testing.T) {
	results := Parse(output)
	if len(results) != 2 {
		t.Fatalf("results = %v", results)
	}
	render := results["BenchmarkRender"]
	if render.NsPerOp != 31.82 || render.BytesPerOp != 0 || render.AllocsPerOp != 0 {
		t.Errorf("BenchmarkRender = %+v", render)
	}
	if results["BenchmarkAppendEscaped"].AllocsPerOp != 1 {
		t.Errorf("BenchmarkAppendEscaped = %+v", results["BenchmarkAppendEscaped"])
	}
}

func TestParseIgnoresNonBenchmarkLines(t *testing.T) {
	if got := Parse("PASS\nok  \tpkg\t1.0s\n"); len(got) != 0 {
		t.Errorf("Parse = %v, want nothing", got)
	}
}

func TestParseIgnoresMalformedNumbers(t *testing.T) {
	if got := Parse("BenchmarkX-8\t10\tabc ns/op\n"); len(got) != 0 {
		t.Errorf("Parse = %v, want nothing", got)
	}
}

func TestParseKeepsNamesWithoutACPUSuffix(t *testing.T) {
	results := Parse("BenchmarkPlain\t10\t5.00 ns/op\n")
	if _, ok := results["BenchmarkPlain"]; !ok {
		t.Errorf("Parse = %v", results)
	}
}

func TestParseStripsOnlyNumericSuffixes(t *testing.T) {
	results := Parse("BenchmarkRender/case-name\t10\t5.00 ns/op\n")
	if _, ok := results["BenchmarkRender/case-name"]; !ok {
		t.Errorf("Parse = %v, want the sub-benchmark name kept", results)
	}
}

func record(ns float64, allocs int64, size int) Record {
	return Record{
		Benchmarks: map[string]Result{"BenchmarkRender": {NsPerOp: ns, AllocsPerOp: allocs}},
		WorkerSize: size,
	}
}

func TestCompareAcceptsAnImprovement(t *testing.T) {
	if got := Compare(record(100, 0, 1000), record(80, 0, 900)); len(got) != 0 {
		t.Errorf("regressions = %v, want none", got)
	}
}

func TestCompareAcceptsNoiseWithinTolerance(t *testing.T) {
	if got := Compare(record(100, 0, 1000), record(108, 0, 1000)); len(got) != 0 {
		t.Errorf("regressions = %v, want none inside the tolerance", got)
	}
}

func TestAllocationsHaveNoTolerance(t *testing.T) {
	got := Compare(record(100, 0, 0), record(100, 1, 0))
	if len(got) != 1 {
		t.Fatalf("regressions = %v, want the single extra allocation to block", got)
	}
}

func TestCompareIgnoresASlowdown(t *testing.T) {
	if got := Compare(record(100, 0, 0), record(130, 0, 0)); len(got) != 0 {
		t.Errorf("regressions = %v, want timing left to Slower", got)
	}
}

func TestSlowerReportsASlowdown(t *testing.T) {
	got := Slower(record(100, 0, 0), record(130, 0, 0))
	if len(got) != 1 || !strings.Contains(got[0], "ns/op") {
		t.Errorf("observations = %v", got)
	}
}

func TestSlowerAcceptsNoiseWithinTolerance(t *testing.T) {
	if got := Slower(record(100, 0, 0), record(108, 0, 0)); len(got) != 0 {
		t.Errorf("observations = %v, want none", got)
	}
}

func TestSlowerSkipsWhatItCannotCompare(t *testing.T) {
	if got := Slower(record(0, 0, 0), record(500, 0, 0)); len(got) != 0 {
		t.Errorf("observations = %v, want no baseline to mean no comparison", got)
	}
	if got := Slower(record(100, 0, 0), Record{Benchmarks: map[string]Result{}}); len(got) != 0 {
		t.Errorf("observations = %v, want a missing benchmark left to Compare", got)
	}
}

func TestCompareFlagsAnyNewAllocation(t *testing.T) {
	got := Compare(record(100, 0, 0), record(100, 1, 0))
	if len(got) != 1 || !strings.Contains(got[0], "allocs/op") {
		t.Errorf("regressions = %v, want the allocation called out", got)
	}
}

func TestCompareFlagsAGrowingWorkerModule(t *testing.T) {
	got := Compare(record(100, 0, 1_000_000), record(100, 0, 1_300_000))
	if len(got) != 1 || !strings.Contains(got[0], "worker module") {
		t.Errorf("regressions = %v", got)
	}
}

func TestCompareIgnoresAnUnknownWorkerSize(t *testing.T) {
	if got := Compare(record(100, 0, 0), record(100, 0, 9_000_000)); len(got) != 0 {
		t.Errorf("regressions = %v, want none without a baseline size", got)
	}
}

func TestCompareReportsADisappearingBenchmark(t *testing.T) {
	got := Compare(record(100, 0, 0), Record{Benchmarks: map[string]Result{}})
	if len(got) != 1 || !strings.Contains(got[0], "missing") {
		t.Errorf("regressions = %v", got)
	}
}

func TestMergeKeepsTheBestResult(t *testing.T) {
	merged := Merge(record(80, 0, 500), record(100, 0, 600))
	if merged.Benchmarks["BenchmarkRender"].NsPerOp != 80 {
		t.Errorf("merged = %+v, want the faster run kept", merged.Benchmarks["BenchmarkRender"])
	}
	if merged.WorkerSize != 600 {
		t.Errorf("worker size = %d, want the current measurement", merged.WorkerSize)
	}
}

func TestMergeAdoptsNewBenchmarks(t *testing.T) {
	merged := Merge(Record{Benchmarks: map[string]Result{}}, record(50, 0, 0))
	if merged.Benchmarks["BenchmarkRender"].NsPerOp != 50 {
		t.Errorf("merged = %+v", merged.Benchmarks)
	}
}

func TestMergeKeepsTheBaselineSizeWhenUnmeasured(t *testing.T) {
	merged := Merge(record(50, 0, 700), record(50, 0, 0))
	if merged.WorkerSize != 700 {
		t.Errorf("worker size = %d, want the baseline kept", merged.WorkerSize)
	}
}

func TestFormatListsEverything(t *testing.T) {
	got := Format(record(31.82, 0, 1_351_942))
	if !strings.Contains(got, "BenchmarkRender") || !strings.Contains(got, "31.82") {
		t.Errorf("Format = %q", got)
	}
	if !strings.Contains(got, "worker module") {
		t.Errorf("Format = %q, want the module size", got)
	}
}

func TestFormatSkipsAnUnknownWorkerSize(t *testing.T) {
	if strings.Contains(Format(record(1, 0, 0)), "worker module") {
		t.Error("Format must not print a module size it does not have")
	}
}

func TestCompareFlagsGrowingClientBundles(t *testing.T) {
	baseline := Record{ClientRuntime: 2000, ClientChunks: 1000}
	grown := Record{ClientRuntime: 2300, ClientChunks: 1050}
	regressions := Compare(baseline, grown)
	if len(regressions) != 1 || !strings.Contains(regressions[0], "client runtime") {
		t.Errorf("regressions = %v, want the runtime flagged and the chunks within tolerance", regressions)
	}
	if got := Compare(baseline, Record{}); len(got) != 0 {
		t.Errorf("regressions = %v, want a run without a client measurement left alone", got)
	}
}

func TestMergeKeepsTheLatestClientSizes(t *testing.T) {
	merged := Merge(Record{ClientRuntime: 2000, ClientChunks: 1000, WorkerSize: 5}, Record{ClientRuntime: 1900})
	if merged.ClientRuntime != 1900 || merged.ClientChunks != 1000 || merged.WorkerSize != 5 {
		t.Errorf("merged = %+v", merged)
	}
	if text := Format(merged); !strings.Contains(text, "client runtime") || !strings.Contains(text, "client chunks") {
		t.Errorf("format = %q", text)
	}
}
