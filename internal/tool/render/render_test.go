package render

import (
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/tool/covprofile"
	"github.com/apptivitypl/rill/internal/tool/gate"
)

func outcome() gate.Outcome {
	return gate.Outcome{
		Rows: []gate.Row{{
			Package:   "internal/ir",
			Coverage:  covprofile.Coverage{Total: 100, Covered: 93},
			Threshold: 92,
			Locked:    91,
			HasLock:   true,
			Status:    gate.StatusOK,
		}},
		Failures: []string{"internal/ir: something slipped"},
	}
}

func TestTableShowsPackageAndValues(t *testing.T) {
	table := Table(outcome())
	for _, want := range []string{"internal/ir", "93.00%", "+2.00", "ok"} {
		if !strings.Contains(table, want) {
			t.Errorf("table is missing %q:\n%s", want, table)
		}
	}
}

func TestMissingLockRendersAsDash(t *testing.T) {
	row := gate.Row{Package: "a", Coverage: covprofile.Coverage{Total: 1, Covered: 1}}
	if got := deltaCell(row); got != "-" {
		t.Errorf("deltaCell = %q, want a dash", got)
	}
}

func TestUnchangedCoverageRendersAsEquals(t *testing.T) {
	row := gate.Row{
		Coverage: covprofile.Coverage{Total: 100, Covered: 90},
		Locked:   90,
		HasLock:  true,
	}
	if got := deltaCell(row); got != "=" {
		t.Errorf("deltaCell = %q, want an equals sign", got)
	}
}

func TestNegativeDeltaKeepsItsSign(t *testing.T) {
	row := gate.Row{
		Coverage: covprofile.Coverage{Total: 100, Covered: 90},
		Locked:   95,
		HasLock:  true,
	}
	if got := deltaCell(row); got != "-5.00" {
		t.Errorf("deltaCell = %q, want -5.00", got)
	}
}

func TestSummaryListsViolations(t *testing.T) {
	summary := SummaryMarkdown(outcome(), covprofile.Report{})
	if !strings.Contains(summary, "### Violations") {
		t.Error("summary is missing the violations section")
	}
	if !strings.Contains(summary, "something slipped") {
		t.Error("summary is missing the failure text")
	}
}

func TestSummaryWithoutViolationsHasNoSection(t *testing.T) {
	clean := outcome()
	clean.Failures = nil
	if strings.Contains(SummaryMarkdown(clean, covprofile.Report{}), "Violations") {
		t.Error("summary must not show an empty violations section")
	}
}

func TestWorstFilesIsTruncated(t *testing.T) {
	var report covprofile.Report
	for i := range 20 {
		report.Files = append(report.Files, covprofile.FileCoverage{
			Path:     "internal/a/f.go",
			Coverage: covprofile.Coverage{Total: 10, Covered: i},
		})
	}
	if got := strings.Count(WorstFiles(report), "\n"); got != worstFiles {
		t.Errorf("listed %d files, want %d", got, worstFiles)
	}
}

func TestWorstFilesOfEmptyReportIsEmpty(t *testing.T) {
	if got := WorstFiles(covprofile.Report{}); got != "" {
		t.Errorf("WorstFiles = %q, want an empty string", got)
	}
}
