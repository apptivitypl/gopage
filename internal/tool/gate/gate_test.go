package gate

import (
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/tool/config"
	"github.com/apptivitypl/rill/internal/tool/covprofile"
)

func cfg(t *testing.T, extra string) *config.Config {
	t.Helper()
	c, err := config.Parse(`{"coverage": {"globalStatements": 90.0, "stubMinStatements": 10` + extra + `}}`)
	if err != nil {
		t.Fatalf("config.Parse: %v", err)
	}
	return c
}

func report(entries ...[3]int) covprofile.Report {
	r := covprofile.Report{Packages: map[string]covprofile.Coverage{}}
	for i, e := range entries {
		name := string(rune('a' + i))
		cov := covprofile.Coverage{Total: e[0], Covered: e[1]}
		r.Packages[name] = cov
		r.Total.Total += cov.Total
		r.Total.Covered += cov.Covered
	}
	return r
}

func named(pkg string, total, covered int) covprofile.Report {
	cov := covprofile.Coverage{Total: total, Covered: covered}
	return covprofile.Report{
		Packages: map[string]covprofile.Coverage{pkg: cov},
		Total:    cov,
	}
}

func TestCoverageAboveThresholdPasses(t *testing.T) {
	out := Evaluate(named("a", 100, 95), cfg(t, ""), Lock{})
	if !out.Passed() {
		t.Fatalf("failures = %v, want none", out.Failures)
	}
	if out.Rows[0].Status != StatusOK {
		t.Errorf("status = %q, want ok", out.Rows[0].Status)
	}
}

func TestCoverageBelowThresholdBlocks(t *testing.T) {
	out := Evaluate(named("a", 100, 80), cfg(t, ""), Lock{})
	if out.Passed() {
		t.Fatal("expected the gate to block")
	}
	if out.Rows[0].Status != StatusFailed {
		t.Errorf("status = %q, want FAIL", out.Rows[0].Status)
	}
	if !strings.Contains(out.Failures[0], "below") {
		t.Errorf("failure = %q", out.Failures[0])
	}
}

func TestStubPackageIsOutsideTheGate(t *testing.T) {
	out := Evaluate(named("a", 3, 0), cfg(t, ""), Lock{})
	if !out.Passed() {
		t.Fatalf("failures = %v, want none for a stub", out.Failures)
	}
	if out.Rows[0].Status != StatusStub {
		t.Errorf("status = %q, want stub", out.Rows[0].Status)
	}
}

func TestRatchetSlipBlocksEvenAboveThreshold(t *testing.T) {
	lock := Lock{Packages: map[string]float64{"a": 99}, Total: 99}
	out := Evaluate(named("a", 100, 95), cfg(t, ""), lock)
	if out.Passed() {
		t.Fatal("expected the ratchet to block")
	}
	if !strings.Contains(out.Failures[0], "ratchet") {
		t.Errorf("failure = %q", out.Failures[0])
	}
}

func TestNoiseWithinToleranceDoesNotBlock(t *testing.T) {
	lock := Lock{Packages: map[string]float64{"a": 95.05}, Total: 95.05}
	out := Evaluate(named("a", 100, 95), cfg(t, ""), lock)
	if !out.Passed() {
		t.Errorf("failures = %v, want none within the tolerance", out.Failures)
	}
}

func TestFixedModeIgnoresTheRatchet(t *testing.T) {
	lock := Lock{Packages: map[string]float64{"a": 99}, Total: 99}
	out := Evaluate(named("a", 100, 95), cfg(t, `, "mode": "fixed"`), lock)
	if !out.Passed() {
		t.Errorf("failures = %v, want none in fixed mode", out.Failures)
	}
}

func TestPackageThresholdOverridesTheGlobalOne(t *testing.T) {
	out := Evaluate(named("a", 100, 95), cfg(t, `, "packages": {"a": {"statements": 99.0}}`), Lock{})
	if out.Passed() {
		t.Error("expected the stricter package threshold to block")
	}
}

func TestOverallGateCatchesTheSumOfGreenPackages(t *testing.T) {
	c := cfg(t, `, "packages": {"b": {"statements": 85.0, "justification": "deliberate"}}`)
	r := report([3]int{10, 10}, [3]int{1000, 880})
	out := Evaluate(r, c, Lock{})
	if out.Passed() {
		t.Fatal("expected the overall gate to block")
	}
	var found bool
	for _, f := range out.Failures {
		if strings.HasPrefix(f, "overall") {
			found = true
		}
	}
	if !found {
		t.Errorf("failures = %v, want one about the overall coverage", out.Failures)
	}
}

func TestDiffGateStaysSilentWithoutMeasurableLines(t *testing.T) {
	if _, blocked := CheckDiff(0, 0, 90); blocked {
		t.Error("expected no diff gate without measurable lines")
	}
}

func TestDiffGateBlocksWeakChanges(t *testing.T) {
	msg, blocked := CheckDiff(1, 10, 90)
	if !blocked {
		t.Fatal("expected the diff gate to block")
	}
	if !strings.Contains(msg, "10.00%") {
		t.Errorf("message = %q", msg)
	}
}

func TestDiffGatePassesGoodChanges(t *testing.T) {
	if _, blocked := CheckDiff(9, 10, 90); blocked {
		t.Error("expected the diff gate to pass")
	}
}

func TestRatchetOnlyMovesUp(t *testing.T) {
	lock := Lock{Packages: map[string]float64{"a": 95}, Total: 95}

	if got := Raise(lock, named("a", 100, 80)).Packages["a"]; got != 95 {
		t.Errorf("lock = %v, want it to stay at 95", got)
	}
	if got := Raise(lock, named("a", 100, 98)).Packages["a"]; got != 98 {
		t.Errorf("lock = %v, want it raised to 98", got)
	}
}

func TestRatchetRecordsNewPackages(t *testing.T) {
	next := Raise(Lock{}, named("fresh", 100, 91))
	if next.Packages["fresh"] != 91 {
		t.Errorf("lock = %v, want 91", next.Packages["fresh"])
	}
	if next.Total != 91 {
		t.Errorf("total = %v, want 91", next.Total)
	}
}

func TestRowDeltaIsMeasuredAgainstTheLock(t *testing.T) {
	lock := Lock{Packages: map[string]float64{"a": 90}, Total: 90}
	out := Evaluate(named("a", 100, 95), cfg(t, ""), lock)
	delta, ok := out.Rows[0].Delta()
	if !ok {
		t.Fatal("expected a delta when the lock has an entry")
	}
	if delta < 4.99 || delta > 5.01 {
		t.Errorf("delta = %v, want 5", delta)
	}
}

func TestRowWithoutLockHasNoDelta(t *testing.T) {
	out := Evaluate(named("a", 100, 95), cfg(t, ""), Lock{})
	if _, ok := out.Rows[0].Delta(); ok {
		t.Error("expected no delta without a lock entry")
	}
}

func TestRowsAreSortedByPackage(t *testing.T) {
	out := Evaluate(report([3]int{100, 100}, [3]int{100, 100}), cfg(t, ""), Lock{})
	if out.Rows[0].Package != "a" || out.Rows[1].Package != "b" {
		t.Errorf("rows = %q, %q, want them sorted", out.Rows[0].Package, out.Rows[1].Package)
	}
}

func TestResetAdoptsTheCurrentNumbers(t *testing.T) {
	lock := Lock{Packages: map[string]float64{"a": 100, "gone": 90}, Total: 100}
	next := Reset(named("a", 100, 95))

	if next.Packages["a"] != 95 {
		t.Errorf("lock = %v, want the current value even though it dropped", next.Packages["a"])
	}
	if _, ok := next.Packages["gone"]; ok {
		t.Error("Reset must forget packages that no longer exist")
	}
	if next.Total != 95 {
		t.Errorf("total = %v, want the current value", next.Total)
	}
	if lock.Packages["a"] != 100 {
		t.Error("Reset must not mutate the previous lock")
	}
}
