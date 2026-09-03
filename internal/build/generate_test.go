package build

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/paths"
)

const loaderPage = `---
type Props struct {
	Title string
	Cards []Card
}

type Card struct {
	Name string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{Title: "hello"}, nil
}
---
<h1>{{ Title }}</h1>
`

const apiHandler = `package route

import "github.com/sonquer/rill"

func GET(ctx *rill.Ctx, params rill.Params) (rill.Response, error) {
	return rill.JSON(map[string]string{"status": "ok"}), nil
}
`

func withModule(files map[string]string) map[string]string {
	files["go.mod"] = "module example.com/demo\n\ngo 1.24\n"
	return files
}

func buildProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := project(t, withModule(files))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return dir
}

func mustParse(t *testing.T, source string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", source, parser.SkipObjectResolution); err != nil {
		t.Fatalf("generated code does not parse: %v\n%s", err, source)
	}
}

func TestPagesWithALoaderAreGenerated(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/features/page.rill": loaderPage})

	page := read(t, dir, "internal/gen/features/page.go")
	mustParse(t, page)
	if !strings.Contains(page, "package features") {
		t.Errorf("package clause = %q", page)
	}
	if !strings.Contains(page, "//line app/features/page.rill:2") {
		t.Error("the line directive is missing")
	}
	if !strings.Contains(page, "func (v Props) Get(path []string)") {
		t.Error("the accessor was not generated")
	}
	if !strings.Contains(page, "func (v Card) Get(path []string)") {
		t.Error("nested structs need accessors too")
	}
}

func TestProviderIsGenerated(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/features/page.rill": loaderPage})

	provider := read(t, dir, "internal/gen/features/provider.go")
	mustParse(t, provider)
	if !strings.Contains(provider, `const Route = "features"`) {
		t.Errorf("route name is missing: %q", provider)
	}
	if !strings.Contains(provider, "rill.NewCtx(request, params)") {
		t.Error("the provider does not build a context")
	}
}

func TestRegistryListsEveryGeneratedRoute(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/features/page.rill":      loaderPage,
		"app/listings/[id]/page.rill": loaderPage,
	})
	registry := read(t, dir, "internal/gen/registry.go")
	mustParse(t, registry)
	for _, want := range []string{
		`"example.com/demo/internal/gen/features"`,
		`"example.com/demo/internal/gen/listings_id"`,
		"features.Route:",
		"features.Provider,",
		"listings_id.Route:",
		"listings_id.Provider,",
	} {
		if !strings.Contains(registry, want) {
			t.Errorf("registry is missing %s:\n%s", want, registry)
		}
	}
}

func TestPagesWithoutALoaderAreNotGenerated(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/page.rill": "<h1>static</h1>"})

	if _, err := os.Stat(filepath.Join(dir, paths.GenRoot, "index")); err == nil {
		t.Error("a page without a loader needs no generated package")
	}
	registry := read(t, dir, "internal/gen/registry.go")
	mustParse(t, registry)
	if strings.Contains(registry, "example.com/demo/internal/gen/") {
		t.Errorf("registry imports nothing yet:\n%s", registry)
	}
}

func TestGeneratedDirectoryIsRebuiltFromScratch(t *testing.T) {
	dir := project(t, withModule(map[string]string{"app/features/page.rill": loaderPage}))
	stale := filepath.Join(dir, paths.GenRoot, "gone", "page.go")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("package gone"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("stale generated packages must be removed")
	}
}

func TestModuleCanBeGivenExplicitly(t *testing.T) {
	dir := project(t, map[string]string{"app/features/page.rill": loaderPage})
	if _, err := Run(Options{Dir: dir, Module: "example.com/given", Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(read(t, dir, "internal/gen/registry.go"), "example.com/given/internal/gen/features") {
		t.Error("the explicit module path was ignored")
	}
}

func TestMissingModuleIsReported(t *testing.T) {
	dir := project(t, map[string]string{"app/features/page.rill": loaderPage})
	_, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("err = %v, want it to name the missing go.mod", err)
	}
}

func TestGoModWithoutAModuleDirectiveIsReported(t *testing.T) {
	dir := project(t, map[string]string{
		"app/features/page.rill": loaderPage,
		"go.mod":                 "go 1.24\n",
	})
	_, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err == nil || !strings.Contains(err.Error(), "module directive") {
		t.Errorf("err = %v", err)
	}
}

func TestPagesWithALoaderAreDynamic(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/features/page.rill": loaderPage})
	if _, err := os.Stat(filepath.Join(dir, "dist", "assets", "features", "index.html")); err == nil {
		t.Error("a page with a loader must not be prerendered")
	}
}

func TestWorkerPatternsComeFromTheRouteTable(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill":               "<h1>static</h1>",
		"app/features/page.rill":      loaderPage,
		"app/listings/[id]/page.rill": "<p>detail</p>",
	}))
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var config wrangler
	decode(t, read(t, dir, paths.Wrangler), &config)

	for _, want := range []string{"/api/*", "/features", "/listings/*"} {
		if !slices.Contains(config.RunWorkerFirst, want) {
			t.Errorf("run_worker_first = %v, want %s", config.RunWorkerFirst, want)
		}
	}
	if slices.Contains(config.RunWorkerFirst, "/") {
		t.Error("static routes must not be routed to the worker")
	}
}

func TestWorkerGlobStopsAtTheFirstParameter(t *testing.T) {
	cases := map[string]string{
		"/listings/[id]":  "/listings/*",
		"/docs/[...slug]": "/docs/*",
		"/a/[b]/c/[d]":    "/a/*",
		"/features":       "/features",
	}
	for pattern, want := range cases {
		if got := workerGlob(pattern); got != want {
			t.Errorf("workerGlob(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func TestBrokenFrontmatterStopsGeneration(t *testing.T) {
	page := "---\nfunc Load(ctx *rill.Ctx) (Props, error) { return Props{}, nil }\nfunc (\n---\n<h1>x</h1>"
	dir := project(t, withModule(map[string]string{"app/features/page.rill": page}))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a frontmatter that does not parse must stop the build")
	}
}

func TestGeneratedCountIsReported(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/features/page.rill": loaderPage,
		"app/page.rill":          "<h1>static</h1>",
	}))
	report, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Generated != 1 {
		t.Errorf("generated = %d, want one package", report.Generated)
	}
}

func decode(t *testing.T, text string, out *wrangler) {
	t.Helper()
	if err := unmarshal(text, out); err != nil {
		t.Fatalf("wrangler config: %v", err)
	}
}

func TestUnwritableProjectStopsGeneration(t *testing.T) {
	dir := project(t, withModule(map[string]string{"app/features/page.rill": loaderPage}))
	denyWrites(t, dir)

	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a project that cannot be written must fail the build")
	}
}

const metaPage = `---
type Props struct {
	Title string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{Title: "t"}, nil
}

func Meta(ctx *rill.Ctx, p Props) rill.Meta {
	return rill.Meta{Title: p.Title}
}
---
<h1>{{ Title }}</h1>
`

func TestMetaProviderIsGeneratedWhenDeclared(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/features/page.rill": metaPage})

	provider := read(t, dir, "internal/gen/features/provider.go")
	mustParse(t, provider)
	if !strings.Contains(provider, "func MetaProvider(") {
		t.Errorf("meta provider is missing:\n%s", provider)
	}
	registry := read(t, dir, "internal/gen/registry.go")
	mustParse(t, registry)
	if !strings.Contains(registry, "features.Route: features.MetaProvider") {
		t.Errorf("registry does not wire the meta provider:\n%s", registry)
	}
}

func TestNoMetaProviderWithoutAMetaFunction(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/features/page.rill": loaderPage})

	if strings.Contains(read(t, dir, "internal/gen/features/provider.go"), "MetaProvider") {
		t.Error("a page without Meta needs no meta provider")
	}
	registry := read(t, dir, "internal/gen/registry.go")
	if !strings.Contains(registry, "func Meta() map[string]rill.MetaProvider") {
		t.Errorf("the registry always declares Meta:\n%s", registry)
	}
}
