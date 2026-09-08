package vocab

import (
	"testing"

	"github.com/apptivitypl/gopage/internal/config"
)

func TestNothingIsNormalisedByDefault(t *testing.T) {
	var rules config.Normalize
	for _, path := range []string{"/Praca/Kraków/", "/", ""} {
		if got := Normalise(path, rules); got != path {
			t.Errorf("Normalise(%q) = %q", path, got)
		}
	}
}

func TestTheTrailingSlashFollowsTheRule(t *testing.T) {
	strip := config.Normalize{TrailingSlash: config.SlashStrip}
	keep := config.Normalize{TrailingSlash: config.SlashKeep}
	cases := []struct {
		rules config.Normalize
		path  string
		want  string
	}{
		{strip, "/jobs/", "/jobs"},
		{strip, "/jobs", "/jobs"},
		{strip, "/", "/"},
		{keep, "/jobs", "/jobs/"},
		{keep, "/jobs/", "/jobs/"},
		{keep, "/", "/"},
	}
	for _, c := range cases {
		if got := Normalise(c.path, c.rules); got != c.want {
			t.Errorf("Normalise(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestCaseAndMarksAreFolded(t *testing.T) {
	rules := config.Normalize{Case: config.CaseLower, Diacritics: config.FoldMarks}
	cases := map[string]string{
		"/Praca/Kraków":   "/praca/krakow",
		"/JOBS/Wien":      "/jobs/wien",
		"/praca/gdansk":   "/praca/gdansk",
		"/arbeit/münchen": "/arbeit/munchen",
	}
	for path, want := range cases {
		if got := Normalise(path, rules); got != want {
			t.Errorf("Normalise(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestFoldingLeavesAsciiAlone(t *testing.T) {
	rules := config.Normalize{Diacritics: config.FoldMarks}
	if got := Normalise("/jobs/warszawa", rules); got != "/jobs/warszawa" {
		t.Errorf("path = %q", got)
	}
}
