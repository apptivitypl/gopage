package config

import (
	"strings"
	"testing"
)

func source(packages string) string {
	return `{
  "coverage": {
    "globalStatements": 90.0,
    "exclude": ["cmd/**", "**/shell.go"]` + packages + `
  }
}`
}

func parse(t *testing.T, packages string) *Config {
	t.Helper()
	cfg, err := Parse(source(packages))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cfg
}

func parseErr(t *testing.T, packages string) error {
	t.Helper()
	if _, err := Parse(source(packages)); err != nil {
		return err
	}
	t.Fatal("expected an error, got none")
	return nil
}

func TestDefaultsAreFilledIn(t *testing.T) {
	cfg := parse(t, "")
	if cfg.Mode != ModeRatchet {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeRatchet)
	}
	if cfg.Lock != "dev.lock.json" {
		t.Errorf("Lock = %q", cfg.Lock)
	}
	if cfg.StubMinStatements != 10 {
		t.Errorf("StubMinStatements = %d, want 10", cfg.StubMinStatements)
	}
	if cfg.RatchetTolerance != 0.1 {
		t.Errorf("RatchetTolerance = %v, want 0.1", cfg.RatchetTolerance)
	}
	if cfg.DiffStatements != 90.0 {
		t.Errorf("DiffStatements = %v, want the global threshold", cfg.DiffStatements)
	}
}

func TestThresholdBelowGlobalNeedsJustification(t *testing.T) {
	err := parseErr(t, `, "packages": {"internal/assets": {"statements": 82.0}}`)
	if !strings.Contains(err.Error(), "justification") {
		t.Errorf("error = %q, want it to mention the missing justification", err)
	}
}

func TestBlankJustificationIsNotEnough(t *testing.T) {
	err := parseErr(t, `, "packages": {"a": {"statements": 10.0, "justification": "   "}}`)
	if !strings.Contains(err.Error(), "justification") {
		t.Errorf("error = %q", err)
	}
}

func TestThresholdBelowGlobalWithJustificationIsAccepted(t *testing.T) {
	cfg := parse(t, `, "packages": {"a": {"statements": 82.0, "justification": "adapters behind a port"}}`)
	if got := cfg.Threshold("a"); got != 82.0 {
		t.Errorf("Threshold = %v, want 82", got)
	}
}

func TestThresholdAboveGlobalNeedsNoJustification(t *testing.T) {
	cfg := parse(t, `, "packages": {"a": {"statements": 95.0}}`)
	if got := cfg.Threshold("a"); got != 95.0 {
		t.Errorf("Threshold = %v, want 95", got)
	}
}

func TestPackageWithoutRuleFallsBackToGlobal(t *testing.T) {
	if got := parse(t, "").Threshold("unknown"); got != 90.0 {
		t.Errorf("Threshold = %v, want 90", got)
	}
}

func TestExclusionsMatchPaths(t *testing.T) {
	cfg := parse(t, "")
	cases := []struct {
		path string
		want bool
	}{
		{"cmd/gopagetool/main.go", true},
		{"internal/assets/shell.go", true},
		{"internal/ir/plan.go", false},
	}
	for _, c := range cases {
		if got := cfg.IsExcluded(c.path); got != c.want {
			t.Errorf("IsExcluded(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestInvalidPatternFailsTheParse(t *testing.T) {
	_, err := Parse(`{"coverage": {"globalStatements": 90.0, "exclude": ["["]}}`)
	if err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Errorf("error = %v, want it to mention the invalid pattern", err)
	}
}

func TestMalformedConfigFails(t *testing.T) {
	if _, err := Parse(`{"coverage"`); err == nil {
		t.Error("expected an error for a malformed config")
	}
}

func TestCommentsAndTrailingCommasAreAccepted(t *testing.T) {
	cfg, err := Parse(`{
  // the gate every package answers to
  "coverage": {
    "globalStatements": 90.0,
    "exclude": ["cmd/**",],
  },
}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.GlobalStatements != 90.0 {
		t.Errorf("GlobalStatements = %v", cfg.GlobalStatements)
	}
}

func TestAnUnknownKeyIsReported(t *testing.T) {
	_, err := Parse(`{"coverage": {"globalStatement": 90.0}}`)
	if err == nil || !strings.Contains(err.Error(), "globalStatement") {
		t.Errorf("error = %v, want the unknown key named", err)
	}
}
