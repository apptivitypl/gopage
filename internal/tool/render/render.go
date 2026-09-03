package render

import (
	"fmt"
	"strings"

	"github.com/sonquer/rill/internal/tool/covprofile"
	"github.com/sonquer/rill/internal/tool/gate"
)

const worstFiles = 10

func Table(outcome gate.Outcome) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %9s %10s %8s  status\n", "package", "stmts", "vs lock", "threshold")
	for _, row := range outcome.Rows {
		fmt.Fprintf(&b, "%-24s %8.2f%% %10s %7.1f%%  %s\n",
			row.Package, row.Coverage.Percent(), deltaCell(row), row.Threshold, row.Status)
	}
	return b.String()
}

func deltaCell(row gate.Row) string {
	delta, ok := row.Delta()
	switch {
	case !ok:
		return "-"
	case delta > -0.005 && delta < 0.005:
		return "="
	default:
		return fmt.Sprintf("%+.2f", delta)
	}
}

func WorstFiles(report covprofile.Report) string {
	var b strings.Builder
	for i, file := range report.Files {
		if i == worstFiles {
			break
		}
		fmt.Fprintf(&b, "  %6.2f%%  %s  (%d/%d statements)\n",
			file.Percent(), file.Path, file.Covered, file.Total)
	}
	return b.String()
}

func SummaryMarkdown(outcome gate.Outcome, report covprofile.Report) string {
	var b strings.Builder
	b.WriteString("## Coverage\n\n")
	fmt.Fprintf(&b, "Overall: **%.2f%%** of statements (%d/%d).\n\n",
		report.Total.Percent(), report.Total.Covered, report.Total.Total)
	b.WriteString("| package | stmts | vs lock | threshold | status |\n")
	b.WriteString("|---|---:|---:|---:|---|\n")
	for _, row := range outcome.Rows {
		fmt.Fprintf(&b, "| `%s` | %.2f%% | %s | %.1f%% | %s |\n",
			row.Package, row.Coverage.Percent(), deltaCell(row), row.Threshold, row.Status)
	}
	if len(outcome.Failures) > 0 {
		b.WriteString("\n### Violations\n\n")
		for _, failure := range outcome.Failures {
			fmt.Fprintf(&b, "- %s\n", failure)
		}
	}
	return b.String()
}
