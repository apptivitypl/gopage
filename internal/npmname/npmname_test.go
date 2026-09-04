package npmname

import "testing"

func TestCleanKeepsWhatNpmAccepts(t *testing.T) {
	for _, name := range []string{"my-site", "shop.v2", "a_b", "site123"} {
		if got := Clean(name); got != name {
			t.Errorf("Clean(%q) = %q, want it left alone", name, got)
		}
	}
}

func TestCleanFoldsWhatNpmRejects(t *testing.T) {
	cases := map[string]string{
		"My Site":     "my-site",
		"Sklep Wóz":   "sklep-w-z",
		"café/branch": "caf-branch",
		"--edges--":   "edges",
		"..dots..":    "dots",
		"UPPER":       "upper",
		"a  b":        "a-b",
	}
	for name, want := range cases {
		if got := Clean(name); got != want {
			t.Errorf("Clean(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAnEmptyResultFallsBackToTheProjectName(t *testing.T) {
	for _, name := range []string{"", "///", "___", "!!!"} {
		if got := Clean(name); got != fallback {
			t.Errorf("Clean(%q) = %q, want %q", name, got, fallback)
		}
	}
}
