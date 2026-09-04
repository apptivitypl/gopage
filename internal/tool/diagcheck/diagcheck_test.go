package diagcheck

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func set(codes ...string) map[string]bool {
	out := map[string]bool{}
	for _, c := range codes {
		out[c] = true
	}
	return out
}

func TestIsCodeRecognizesTheShape(t *testing.T) {
	cases := map[string]bool{
		"C107":  true,
		"W703":  true,
		"C10":   false,
		"C1077": false,
		"X107":  false,
		"CABC":  false,
	}
	for token, want := range cases {
		if got := IsCode(token); got != want {
			t.Errorf("IsCode(%q) = %v, want %v", token, got, want)
		}
	}
}

func TestCodesInIgnoresSurroundingPunctuation(t *testing.T) {
	got := CodesIn("error[GOPAGE-C107]: broken; see W703 and \"C503\"")
	if !reflect.DeepEqual(got, set("C107", "C503", "W703")) {
		t.Errorf("CodesIn = %v", got)
	}
}

func TestCompleteRegistryHasNoIssues(t *testing.T) {
	all := set("C107")
	if issues := Analyze(all, all, all); len(issues) != 0 {
		t.Errorf("issues = %v, want none", issues)
	}
}

func TestMissingPageIsReported(t *testing.T) {
	issues := Analyze(set("C107"), set(), set("C107"))
	if len(issues) != 1 || issues[0].Kind != Undocumented {
		t.Fatalf("issues = %v, want one undocumented code", issues)
	}
	if !strings.Contains(issues[0].Message(), "docs/errors/C107.md") {
		t.Errorf("message = %q", issues[0].Message())
	}
}

func TestMissingTestIsReported(t *testing.T) {
	issues := Analyze(set("C107"), set("C107"), set())
	if len(issues) != 1 || issues[0].Kind != Untested {
		t.Fatalf("issues = %v, want one untested code", issues)
	}
}

func TestOrphanPageIsReported(t *testing.T) {
	issues := Analyze(set(), set("C999"), set())
	if len(issues) != 1 || issues[0].Kind != OrphanPage {
		t.Fatalf("issues = %v, want one orphan page", issues)
	}
}

func TestEmptyRegistryPasses(t *testing.T) {
	if issues := Analyze(set(), set(), set()); len(issues) != 0 {
		t.Errorf("issues = %v, want none", issues)
	}
}

func TestIssuesAreSorted(t *testing.T) {
	issues := Analyze(set("C200", "C100"), set(), set())
	if issues[0].Code != "C100" {
		t.Errorf("first issue = %q, want the lowest code", issues[0].Code)
	}
}

func TestRenderDescribesEveryIssue(t *testing.T) {
	text := Render(Analyze(set("C107"), set(), set()))
	if !strings.Contains(text, "docs/errors/C107.md") || !strings.Contains(text, "no test produces") {
		t.Errorf("render = %q", text)
	}
}

func TestRenderWithoutIssuesIsShort(t *testing.T) {
	if got := Render(nil); got != "diag: ok\n" {
		t.Errorf("Render = %q", got)
	}
}

func TestCheckReadsTheRepository(t *testing.T) {
	root := fstest.MapFS{
		"internal/diag/codes.go":        {Data: []byte("const C107 Code = iota\nconst W703 Code = iota\n")},
		"docs/errors/C107.md":           {Data: []byte("# C107")},
		"docs/errors/README.md":         {Data: []byte("| C107 | one |\n| W703 | two |")},
		"internal/compile/link_test.go": {Data: []byte("func TestLink(t *testing.T) { want(\"C107\") }")},
	}
	issues, err := Check(root)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	want := []Issue{{Undocumented, "W703"}, {Untested, "W703"}}
	if !reflect.DeepEqual(issues, want) {
		t.Errorf("issues = %v, want %v", issues, want)
	}
}

func TestCheckOnEmptyRepositoryPasses(t *testing.T) {
	issues, err := Check(fstest.MapFS{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("issues = %v, want none", issues)
	}
}

func TestCheckIgnoresNonTestSources(t *testing.T) {
	root := fstest.MapFS{
		"internal/diag/codes.go":   {Data: []byte("const C107 Code = iota")},
		"docs/errors/C107.md":      {Data: []byte("# C107")},
		"internal/compile/link.go": {Data: []byte("return diag.New(\"C107\")")},
	}
	issues, err := Check(root)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(issues) != 1 || issues[0].Kind != Untested {
		t.Errorf("issues = %v, want C107 reported as untested", issues)
	}
}

func TestACodeMissingFromTheIndexIsReported(t *testing.T) {
	root := fstest.MapFS{
		"internal/diag/codes.go": &fstest.MapFile{Data: []byte(`const (
	C001 Code = "C001"
	C002 Code = "C002"
)`)},
		"docs/errors/C001.md":   &fstest.MapFile{Data: []byte("# C001")},
		"docs/errors/C002.md":   &fstest.MapFile{Data: []byte("# C002")},
		"docs/errors/README.md": &fstest.MapFile{Data: []byte("| [C001](C001.md) | one |")},
		"internal/a/a_test.go":  &fstest.MapFile{Data: []byte("diag.C001\ndiag.C002")},
	}
	issues, err := Check(root)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(issues) != 1 || issues[0].Kind != Unindexed || issues[0].Code != "C002" {
		t.Fatalf("issues = %+v", issues)
	}
	if !strings.Contains(issues[0].Message(), "README.md") {
		t.Errorf("message = %q", issues[0].Message())
	}
}

func TestAProjectWithoutAnIndexIsNotChecked(t *testing.T) {
	root := fstest.MapFS{
		"internal/diag/codes.go": &fstest.MapFile{Data: []byte(`C001 Code = "C001"`)},
		"docs/errors/C001.md":    &fstest.MapFile{Data: []byte("# C001")},
		"internal/a/a_test.go":   &fstest.MapFile{Data: []byte("diag.C001")},
	}
	issues, err := Check(root)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("issues = %+v", issues)
	}
}
