package gate

import (
	"fmt"
	"maps"
	"sort"

	"github.com/apptivitypl/rill/internal/tool/config"
	"github.com/apptivitypl/rill/internal/tool/covprofile"
)

type Lock struct {
	Packages map[string]float64 `json:"packages"`
	Total    float64            `json:"total"`
}

type Status string

const (
	StatusOK     Status = "ok"
	StatusStub   Status = "stub"
	StatusFailed Status = "FAIL"
)

type Row struct {
	Package   string
	Coverage  covprofile.Coverage
	Threshold float64
	Locked    float64
	HasLock   bool
	Status    Status
}

func (r Row) Delta() (float64, bool) {
	if !r.HasLock {
		return 0, false
	}
	return r.Coverage.Percent() - r.Locked, true
}

type Outcome struct {
	Rows     []Row
	Failures []string
}

func (o Outcome) Passed() bool {
	return len(o.Failures) == 0
}

func Evaluate(report covprofile.Report, cfg *config.Config, lock Lock) Outcome {
	var outcome Outcome

	for _, pkg := range sortedKeys(report.Packages) {
		cov := report.Packages[pkg]
		row := Row{Package: pkg, Coverage: cov, Threshold: cfg.Threshold(pkg), Status: StatusOK}
		row.Locked, row.HasLock = lock.Packages[pkg]

		switch {
		case cov.Total < cfg.StubMinStatements:
			row.Status = StatusStub
		default:
			if cov.Percent() < row.Threshold {
				row.Status = StatusFailed
				outcome.Failures = append(outcome.Failures, fmt.Sprintf(
					"%s: statement coverage %.2f%% is below the %.2f%% threshold", pkg, cov.Percent(), row.Threshold))
			}
			if cfg.Mode == config.ModeRatchet && row.HasLock && cov.Percent() < row.Locked-cfg.RatchetTolerance {
				row.Status = StatusFailed
				outcome.Failures = append(outcome.Failures, fmt.Sprintf(
					"%s: ratchet slipped to %.2f%% from the best %.2f%% (tolerance %.2f pp)",
					pkg, cov.Percent(), row.Locked, cfg.RatchetTolerance))
			}
		}
		outcome.Rows = append(outcome.Rows, row)
	}

	if report.Total.Total >= cfg.StubMinStatements && report.Total.Percent() < cfg.GlobalStatements {
		outcome.Failures = append(outcome.Failures, fmt.Sprintf(
			"overall: statement coverage %.2f%% is below the %.2f%% threshold",
			report.Total.Percent(), cfg.GlobalStatements))
	}
	return outcome
}

func CheckDiff(covered, total int, threshold float64) (string, bool) {
	if total == 0 {
		return "", false
	}
	value := covprofile.Coverage{Total: total, Covered: covered}.Percent()
	if value >= threshold {
		return "", false
	}
	return fmt.Sprintf("changed lines: coverage %.2f%% (%d/%d) is below the %.2f%% threshold",
		value, covered, total, threshold), true
}

func Reset(report covprofile.Report) Lock {
	next := Lock{Packages: map[string]float64{}, Total: report.Total.Percent()}
	for pkg, cov := range report.Packages {
		next.Packages[pkg] = cov.Percent()
	}
	return next
}

func Raise(lock Lock, report covprofile.Report) Lock {
	next := Lock{Packages: map[string]float64{}, Total: max(lock.Total, report.Total.Percent())}
	maps.Copy(next.Packages, lock.Packages)
	for pkg, cov := range report.Packages {
		next.Packages[pkg] = max(next.Packages[pkg], cov.Percent())
	}
	return next
}

func sortedKeys(m map[string]covprofile.Coverage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
