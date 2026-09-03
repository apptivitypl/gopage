package examplecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/scaffold"
)

const repoRoot = "../../.."

func TestEveryExampleNamesATemplateThatExists(t *testing.T) {
	for _, example := range Examples() {
		if !scaffold.Has(example.Template) {
			t.Errorf("%s names template %q, which is not shipped", example.Name, example.Template)
		}
		if example.Dir() != "examples/"+example.Name {
			t.Errorf("Dir = %q", example.Dir())
		}
		if !strings.HasSuffix(example.Module(), example.Dir()) {
			t.Errorf("Module = %q", example.Module())
		}
	}
}

func TestTheConfigPinsEveryChoiceItCanMake(t *testing.T) {
	config := Examples()[0].Config("v1.2.3")
	if config.RillVersion != "v1.2.3" {
		t.Errorf("RillVersion = %q", config.RillVersion)
	}
	for name, value := range map[string]string{
		"nav":   config.Nav,
		"css":   config.CSS,
		"theme": config.Theme,
		"react": config.React,
	} {
		if value == "" {
			t.Errorf("%s is left to the default, so a changed default would slip in unseen", name)
		}
	}
	if config.RillPath != "" {
		t.Error("a committed example must not replace rill with a local path")
	}
}

func TestFingerprintSkipsWhatABuildLeavesBehind(t *testing.T) {
	tree := fstest.MapFS{
		"app/page.rill":             {Data: []byte("<h1>hi</h1>")},
		"internal/gen/manifest.bin": {Data: []byte("binary")},
		"dist/server":               {Data: []byte("binary")},
		"node_modules/react/x.js":   {Data: []byte("js")},
		".rill/cache/thing":         {Data: []byte("cache")},
		"wrangler.log":              {Data: []byte("noise")},
		"README.md":                 {Data: []byte("hand written")},
		"go.sum":                    {Data: []byte("hashes")},
	}
	sums, err := Fingerprint(tree)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if len(sums) != 1 {
		t.Fatalf("fingerprinted %v, want only the source file", keys(sums))
	}
	if _, ok := sums["app/page.rill"]; !ok {
		t.Errorf("fingerprinted %v", keys(sums))
	}
}

func TestCompareNamesEveryKindOfDifference(t *testing.T) {
	committed := fstest.MapFS{
		"same.rill":    {Data: []byte("a")},
		"changed.rill": {Data: []byte("old")},
		"stale.rill":   {Data: []byte("gone")},
	}
	generated := fstest.MapFS{
		"same.rill":    {Data: []byte("a")},
		"changed.rill": {Data: []byte("new")},
		"fresh.rill":   {Data: []byte("added")},
	}
	differences, err := Compare("example", committed, generated)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	found := map[string]Kind{}
	for _, difference := range differences {
		found[difference.Path] = difference.Kind
	}
	want := map[string]Kind{"changed.rill": Changed, "fresh.rill": Missing, "stale.rill": Extra}
	for path, kind := range want {
		if found[path] != kind {
			t.Errorf("%s = %q, want %q", path, found[path], kind)
		}
	}
	if _, ok := found["same.rill"]; ok {
		t.Error("an identical file was reported as a difference")
	}
	for _, difference := range differences {
		if difference.Message() == "" {
			t.Errorf("%v has no message", difference)
		}
	}
}

func TestRenderSaysWhatToRun(t *testing.T) {
	if got := Render(nil); got != "example: ok\n" {
		t.Errorf("Render = %q", got)
	}
	out := Render([]Difference{{Example: "blog", Path: "app/page.rill", Kind: Changed}})
	if !strings.Contains(out, "--update") {
		t.Errorf("Render = %q, want the command that fixes it", out)
	}
}

func TestTheCommittedExamplesMatchTheTemplates(t *testing.T) {
	version := requiredVersion(t)
	for _, example := range Examples() {
		committed := filepath.Join(repoRoot, filepath.FromSlash(example.Dir()))
		if _, err := os.Stat(committed); err != nil {
			t.Fatalf("%s is not committed: %v", example.Dir(), err)
		}
		config := example.Config(version)
		config.Dir = filepath.Join(t.TempDir(), example.Name)
		if err := scaffold.Create(config); err != nil {
			t.Fatalf("Create %s: %v", example.Name, err)
		}
		differences, err := Compare(example.Name, os.DirFS(committed), os.DirFS(config.Dir))
		if err != nil {
			t.Fatalf("Compare %s: %v", example.Name, err)
		}
		if len(differences) > 0 {
			t.Errorf("%s", Render(differences))
		}
	}
}

func requiredVersion(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "versions.jsonc"))
	if err != nil {
		t.Fatalf("read versions.jsonc: %v", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.Contains(line, `"version"`) {
			_, value, _ := strings.Cut(line, ":")
			return "v" + strings.Trim(strings.TrimSpace(value), `",`)
		}
	}
	t.Fatal("versions.jsonc names no version")
	return ""
}

func keys(sums map[string][32]byte) []string {
	var names []string
	for name := range sums {
		names = append(names, name)
	}
	return names
}
