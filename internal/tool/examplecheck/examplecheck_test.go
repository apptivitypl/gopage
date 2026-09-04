package examplecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/scaffold"
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
	if config.GopageVersion != "v1.2.3" {
		t.Errorf("GopageVersion = %q", config.GopageVersion)
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
	if config.GopagePath != "" {
		t.Error("a committed example must not replace gopage with a local path")
	}
}

func TestFingerprintSkipsWhatABuildLeavesBehind(t *testing.T) {
	tree := fstest.MapFS{
		"app/page.gopage":           {Data: []byte("<h1>hi</h1>")},
		"internal/gen/manifest.bin": {Data: []byte("binary")},
		"dist/server":               {Data: []byte("binary")},
		"node_modules/react/x.js":   {Data: []byte("js")},
		".gopage/cache/thing":       {Data: []byte("cache")},
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
	if _, ok := sums["app/page.gopage"]; !ok {
		t.Errorf("fingerprinted %v", keys(sums))
	}
}

func TestCompareNamesEveryKindOfDifference(t *testing.T) {
	committed := fstest.MapFS{
		"same.gopage":    {Data: []byte("a")},
		"changed.gopage": {Data: []byte("old")},
		"stale.gopage":   {Data: []byte("gone")},
	}
	generated := fstest.MapFS{
		"same.gopage":    {Data: []byte("a")},
		"changed.gopage": {Data: []byte("new")},
		"fresh.gopage":   {Data: []byte("added")},
	}
	differences, err := Compare("example", committed, generated)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	found := map[string]Kind{}
	for _, difference := range differences {
		found[difference.Path] = difference.Kind
	}
	want := map[string]Kind{"changed.gopage": Changed, "fresh.gopage": Missing, "stale.gopage": Extra}
	for path, kind := range want {
		if found[path] != kind {
			t.Errorf("%s = %q, want %q", path, found[path], kind)
		}
	}
	if _, ok := found["same.gopage"]; ok {
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
	out := Render([]Difference{{Example: "blog", Path: "app/page.gopage", Kind: Changed}})
	if !strings.Contains(out, "--update") {
		t.Errorf("Render = %q, want the command that fixes it", out)
	}
}

func TestTheCommittedExamplesMatchTheTemplates(t *testing.T) {
	for _, example := range Examples() {
		committed := filepath.Join(repoRoot, filepath.FromSlash(example.Dir()))
		if _, err := os.Stat(committed); err != nil {
			t.Fatalf("%s is not committed: %v", example.Dir(), err)
		}
		version, err := example.PinnedVersion(repoRoot)
		if err != nil {
			t.Fatalf("PinnedVersion %s: %v", example.Name, err)
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

func keys(sums map[string][32]byte) []string {
	var names []string
	for name := range sums {
		names = append(names, name)
	}
	return names
}

func TestGoModComparesOnlyWhatTheTemplateWrites(t *testing.T) {
	template := []byte(`module example.com/site

go 1.26.0

require (
	github.com/apptivitypl/gopage v0.1.1
	github.com/syumai/workers v0.33.0
)
`)
	tidied := []byte(`module example.com/site

go 1.26.0

require (
	github.com/apptivitypl/gopage v0.1.1
	github.com/syumai/workers v0.33.0
)

require (
	github.com/andybalholm/brotli v1.2.3 // indirect
	golang.org/x/net v0.58.0 // indirect
)
`)
	if string(direct(tidied)) != string(direct(template)) {
		t.Errorf("tidy's indirect block changed the comparison:\n%s", direct(tidied))
	}

	bumped := []byte(strings.Replace(string(template), "v0.33.0", "v0.34.0", 1))
	if string(direct(bumped)) == string(direct(template)) {
		t.Error("a changed direct requirement must still be a difference")
	}
}

func TestThePinIsReadFromTheCommittedExample(t *testing.T) {
	root := t.TempDir()
	example := Examples()[0]
	dir := filepath.Join(root, filepath.FromSlash(example.Dir()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	text := "module " + example.Module() + "\n\ngo 1.26.0\n\nrequire (\n\t" +
		Module + " v0.4.2\n\tgithub.com/syumai/workers v0.33.0\n)\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(text), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := example.PinnedVersion(root)
	if err != nil {
		t.Fatalf("PinnedVersion: %v", err)
	}
	if got != "v0.4.2" {
		t.Errorf("PinnedVersion = %q, want what the example requires, not what versions.jsonc holds", got)
	}
}

func TestTheModuleLineIsNotMistakenForThePin(t *testing.T) {
	root := t.TempDir()
	example := Examples()[0]
	dir := filepath.Join(root, filepath.FromSlash(example.Dir()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	text := "module " + example.Module() + "\n\ngo 1.26.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(text), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := example.PinnedVersion(root); err == nil {
		t.Error("an example that requires nothing must not report a pin")
	}
}

func TestAnExampleWithoutAGoModSaysSo(t *testing.T) {
	if _, err := Examples()[0].PinnedVersion(t.TempDir()); !os.IsNotExist(err) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}
