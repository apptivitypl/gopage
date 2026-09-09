package grammarcheck

import (
	"os"
	"path/filepath"
	"testing"
)

func repository(t *testing.T, name string) []byte {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return source
}

func kinds(issues []Issue) map[IssueKind]int {
	counted := map[IssueKind]int{}
	for _, issue := range issues {
		counted[issue.Kind]++
	}
	return counted
}

func TestCommittedGrammarHoldsTheWholeVocabulary(t *testing.T) {
	want := map[string][]string{
		"directive-name":    {"assets", "standalone", "vary", "when"},
		"filter-name":       {"date", "relative", "upper"},
		"strategy-value":    {"idle", "media"},
		"builtin-component": {"Field", "Form", "Image"},
	}
	issues, err := Check(repository(t, Path), want)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, issue := range issues {
		if issue.Kind != Extra {
			t.Errorf("%s", issue.Message())
		}
	}
}

func TestCheckReportsBothDirectionsOfDrift(t *testing.T) {
	source := []byte(`{"repository": {"filter-name": {"match": "\\b(len|upper|gone)\\b"}}}`)
	issues, err := Check(source, map[string][]string{"filter-name": {"len", "upper", "added"}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	counted := kinds(issues)
	if counted[Missing] != 1 || counted[Extra] != 1 || len(issues) != 2 {
		t.Fatalf("issues = %v, want one missing and one extra", issues)
	}
	if issues[0].Symbol != "added" || issues[1].Symbol != "gone" {
		t.Errorf("symbols = %q and %q", issues[0].Symbol, issues[1].Symbol)
	}
}

func TestCheckRefusesARuleItCannotRead(t *testing.T) {
	cases := map[string]string{
		"absent":        `{"repository": {}}`,
		"not a list":    `{"repository": {"filter-name": {"match": "[a-z]+"}}}`,
		"no repository": `{}`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			issues, err := Check([]byte(source), map[string][]string{"filter-name": {"len"}})
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if len(issues) != 1 || issues[0].Kind != Malformed {
				t.Fatalf("issues = %v, want one malformed", issues)
			}
			if issues[0].Message() == "" {
				t.Error("a malformed issue says nothing")
			}
		})
	}
}

func TestCheckRefusesSourceThatIsNotJSON(t *testing.T) {
	if _, err := Check([]byte("{"), nil); err == nil {
		t.Fatal("Check accepted a broken grammar")
	}
}

func TestManifestCarriesNoVersionUntilARelease(t *testing.T) {
	issues, err := CheckManifest(repository(t, ManifestPath))
	if err != nil {
		t.Fatalf("CheckManifest: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestManifestRefusesAStampedVersion(t *testing.T) {
	issues, err := CheckManifest([]byte(`{"version": "0.1.0"}`))
	if err != nil {
		t.Fatalf("CheckManifest: %v", err)
	}
	if len(issues) != 1 || issues[0].Kind != Versioned {
		t.Fatalf("issues = %v, want one versioned", issues)
	}
	if issues[0].Message() == "" {
		t.Error("a versioned issue says nothing")
	}
}

func TestManifestRefusesSourceThatIsNotJSON(t *testing.T) {
	if _, err := CheckManifest([]byte("{")); err == nil {
		t.Fatal("CheckManifest accepted a broken manifest")
	}
}

func TestCommittedSnippetsWriteEveryDirective(t *testing.T) {
	issues, err := CheckSnippets(repository(t, SnippetsPath), []string{"if", "raw", "vary", "elif"})
	if err != nil {
		t.Fatalf("CheckSnippets: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestCheckSnippetsNamesEveryDirectiveNobodyWrote(t *testing.T) {
	source := []byte(`{"If": {"prefix": "if", "body": ["{% if ${1:c} %}", "{% endif %}"]}}`)
	issues, err := CheckSnippets(source, []string{"if", "endif", "raw", "vary"})
	if err != nil {
		t.Fatalf("CheckSnippets: %v", err)
	}
	if len(issues) != 2 || issues[0].Symbol != "raw" || issues[1].Symbol != "vary" {
		t.Fatalf("issues = %v, want raw and vary", issues)
	}
	if issues[0].Kind != Unwritten || issues[0].Message() == "" {
		t.Errorf("issue = %+v", issues[0])
	}
}

func TestCheckSnippetsRefusesSourceThatIsNotJSON(t *testing.T) {
	if _, err := CheckSnippets([]byte("{"), nil); err == nil {
		t.Fatal("CheckSnippets accepted a broken snippet file")
	}
}
