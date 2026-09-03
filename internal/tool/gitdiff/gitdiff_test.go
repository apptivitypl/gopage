package gitdiff

import (
	"reflect"
	"testing"

	"github.com/apptivitypl/rill/internal/tool/covprofile"
)

func TestParseUnifiedReadsHunkRanges(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -10,0 +11,3 @@\n+a\n+b\n+c\n"
	want := ChangedLines{"x.go": {11: true, 12: true, 13: true}}
	if got := ParseUnified(diff); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseUnified = %v, want %v", got, want)
	}
}

func TestHunkWithoutCountIsOneLine(t *testing.T) {
	want := ChangedLines{"x.go": {1: true}}
	if got := ParseUnified("+++ b/x.go\n@@ -1 +1 @@\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseUnified = %v, want %v", got, want)
	}
}

func TestDeletionAddsNothing(t *testing.T) {
	if got := ParseUnified("+++ b/x.go\n@@ -5,2 +4,0 @@\n"); len(got) != 0 {
		t.Errorf("ParseUnified = %v, want no changed lines", got)
	}
}

func TestDeletedFileIsSkipped(t *testing.T) {
	if got := ParseUnified("+++ /dev/null\n@@ -1,5 +0,0 @@\n"); len(got) != 0 {
		t.Errorf("ParseUnified = %v, want no changed lines", got)
	}
}

func TestSeveralFilesInOneDiff(t *testing.T) {
	diff := "+++ b/a.go\n@@ -0,0 +1,2 @@\n+++ b/b.go\n@@ -0,0 +7,1 @@\n"
	got := ParseUnified(diff)
	if !reflect.DeepEqual(got["a.go"], map[int]bool{1: true, 2: true}) {
		t.Errorf("a.go = %v", got["a.go"])
	}
	if !reflect.DeepEqual(got["b.go"], map[int]bool{7: true}) {
		t.Errorf("b.go = %v", got["b.go"])
	}
}

func TestMalformedHunkHeaderIsIgnored(t *testing.T) {
	for _, diff := range []string{"+++ b/a.go\n@@ garbage @@\n", "+++ b/a.go\n@@ -1,1 +x,1 @@\n", "+++ b/a.go\n@@ -1,1 +1,y @@\n", "+++ b/a.go\n@@\n"} {
		if got := ParseUnified(diff); len(got) != 0 {
			t.Errorf("ParseUnified(%q) = %v, want no changed lines", diff, got)
		}
	}
}

func TestHunkBeforeAnyFileIsIgnored(t *testing.T) {
	if got := ParseUnified("@@ -0,0 +1,2 @@\n"); len(got) != 0 {
		t.Errorf("ParseUnified = %v, want no changed lines", got)
	}
}

func TestCoverageCountsOnlyMeasurableLines(t *testing.T) {
	blocks := []covprofile.Block{
		{File: "a.go", StartLine: 1, EndLine: 2, NumStmts: 2, Count: 1},
		{File: "a.go", StartLine: 5, EndLine: 6, NumStmts: 1, Count: 0},
	}
	changed := ChangedLines{"a.go": {1: true, 5: true, 99: true}}

	covered, total := Coverage(blocks, changed)
	if covered != 1 || total != 2 {
		t.Errorf("Coverage = %d/%d, want 1/2", covered, total)
	}
}

func TestCoverageSkipsFilesOutsideTheProfile(t *testing.T) {
	covered, total := Coverage(nil, ChangedLines{"unknown.go": {1: true}})
	if covered != 0 || total != 0 {
		t.Errorf("Coverage = %d/%d, want 0/0", covered, total)
	}
}

func TestCoveragePrefersTheInnermostBlock(t *testing.T) {
	blocks := []covprofile.Block{
		{File: "a.go", StartLine: 1, EndLine: 20, NumStmts: 5, Count: 1},
		{File: "a.go", StartLine: 9, EndLine: 11, NumStmts: 1, Count: 0},
	}
	covered, total := Coverage(blocks, ChangedLines{"a.go": {10: true}})
	if covered != 0 || total != 1 {
		t.Errorf("Coverage = %d/%d, want the innermost uncovered block to win", covered, total)
	}
}
