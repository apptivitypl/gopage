package covprofile

import (
	"strings"
	"testing"
)

const modulePath = "github.com/apptivitypl/rill"

type excludeList []string

func (e excludeList) IsExcluded(path string) bool {
	for _, prefix := range e {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func TestParseReadsBlocks(t *testing.T) {
	profile := "mode: atomic\n" +
		"github.com/apptivitypl/rill/internal/ir/plan.go:12.34,15.2 3 1\n" +
		"github.com/apptivitypl/rill/internal/ir/plan.go:17.2,19.4 2 0\n"

	blocks, err := Parse(profile)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	want := Block{
		File:      "github.com/apptivitypl/rill/internal/ir/plan.go",
		StartLine: 12,
		EndLine:   15,
		NumStmts:  3,
		Count:     1,
	}
	if blocks[0] != want {
		t.Errorf("block = %+v, want %+v", blocks[0], want)
	}
}

func TestParseSkipsBlankLines(t *testing.T) {
	blocks, err := Parse("mode: set\n\n\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("got %d blocks, want 0", len(blocks))
	}
}

func TestParseRejectsMalformedLines(t *testing.T) {
	cases := map[string]string{
		"no separator":    "garbage\n",
		"missing fields":  "a/b.go:1.1,2.2 3\n",
		"bad range":       "a/b.go:1.1 3 1\n",
		"bad line number": "a/b.go:x.1,2.2 3 1\n",
		"bad statements":  "a/b.go:1.1,2.2 x 1\n",
		"bad count":       "a/b.go:1.1,2.2 3 x\n",
		"bad position":    "a/b.go:11,2.2 3 1\n",
	}
	for name, profile := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(profile); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestPercentOfEmptyUnitIsFull(t *testing.T) {
	if got := (Coverage{}).Percent(); got != 100 {
		t.Errorf("Percent = %v, want 100", got)
	}
}

func TestPercentIsPlainArithmetic(t *testing.T) {
	if got := (Coverage{Total: 10, Covered: 8}).Percent(); got != 80 {
		t.Errorf("Percent = %v, want 80", got)
	}
}

func TestRelativizeStripsModulePath(t *testing.T) {
	got := Relativize("github.com/apptivitypl/rill/internal/ir/plan.go", modulePath)
	if got != "internal/ir/plan.go" {
		t.Errorf("Relativize = %q", got)
	}
}

func TestPackageOfUsesDirectory(t *testing.T) {
	cases := map[string]string{
		"internal/ir/plan.go": "internal/ir",
		"main.go":             RootPackage,
	}
	for path, want := range cases {
		if got := PackageOf(path); got != want {
			t.Errorf("PackageOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestAggregateSumsBlocksWithinAPackage(t *testing.T) {
	blocks := []Block{
		{File: modulePath + "/internal/ir/plan.go", NumStmts: 10, Count: 1},
		{File: modulePath + "/internal/ir/expr.go", NumStmts: 10, Count: 0},
	}
	report := Aggregate(blocks, modulePath, nil)
	got := report.Packages["internal/ir"]
	if got.Total != 20 || got.Covered != 10 {
		t.Errorf("coverage = %+v, want 10/20", got)
	}
	if report.Total != got {
		t.Errorf("total = %+v, want it to match the only package", report.Total)
	}
}

func TestAggregateSkipsExcludedPaths(t *testing.T) {
	blocks := []Block{
		{File: modulePath + "/internal/ir/plan.go", NumStmts: 10, Count: 1},
		{File: modulePath + "/cmd/rilltool/main.go", NumStmts: 100, Count: 0},
	}
	report := Aggregate(blocks, modulePath, excludeList{"cmd/"})
	if _, ok := report.Packages["cmd/rilltool"]; ok {
		t.Error("excluded package must not appear in the report")
	}
	if report.Total.Total != 10 {
		t.Errorf("total statements = %d, want 10", report.Total.Total)
	}
}

func TestAggregateMeasuresTheRootPackage(t *testing.T) {
	blocks := []Block{{File: modulePath + "/rill.go", NumStmts: 5, Count: 1}}
	report := Aggregate(blocks, modulePath, nil)
	if got, ok := report.Packages[RootPackage]; !ok || got.Total != 5 {
		t.Errorf("packages = %v, want the root package measured", report.Packages)
	}
	if report.Total.Total != 5 {
		t.Errorf("total = %d, want the root package counted", report.Total.Total)
	}
}

func TestAggregateSortsFilesWorstFirst(t *testing.T) {
	blocks := []Block{
		{File: modulePath + "/internal/a/good.go", NumStmts: 10, Count: 1},
		{File: modulePath + "/internal/a/bad.go", NumStmts: 10, Count: 0},
	}
	report := Aggregate(blocks, modulePath, nil)
	if report.Files[0].Path != "internal/a/bad.go" {
		t.Errorf("first file = %q, want the least covered one", report.Files[0].Path)
	}
}

func TestAggregateKeepsRelativeBlockPaths(t *testing.T) {
	blocks := []Block{{File: modulePath + "/internal/a/x.go", StartLine: 3, EndLine: 5, NumStmts: 1, Count: 1}}
	report := Aggregate(blocks, modulePath, nil)
	if report.Blocks[0].File != "internal/a/x.go" {
		t.Errorf("block path = %q, want it relative to the module", report.Blocks[0].File)
	}
}
