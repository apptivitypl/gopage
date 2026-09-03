package build

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path"
	"runtime"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/assets"
	"github.com/apptivitypl/rill/internal/bundle"
	"github.com/apptivitypl/rill/internal/config"
	"github.com/apptivitypl/rill/internal/paths"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheConfigIsNormalisedIntoTheGeneratedTree(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill": "<h1>home</h1>",
		"rill.jsonc":    "{\"app\": {\"name\": \"demo\"}}",
	})
	if got := read(t, dir, paths.GenConfig); !strings.Contains(got, "demo") {
		t.Errorf("config = %q", got)
	}
}

func TestAProjectWithoutAConfigEmbedsTheDefaults(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/page.rill": "<h1>home</h1>"})
	settings, err := config.Parse(read(t, dir, paths.GenConfig))
	if err != nil {
		t.Fatalf("the generated config does not parse: %v", err)
	}
	if settings.I18n.Mode != config.ModePath || settings.I18n.DefaultLocale != "en" {
		t.Errorf("config = %+v, want the defaults written out", settings)
	}
}

func TestTheGeneratedConfigCarriesNoComments(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill": "<h1>home</h1>",
		"rill.jsonc":    "{\n  // the name shows up in the worker\n  \"app\": {\"name\": \"demo\"}\n}",
	})
	if got := read(t, dir, paths.GenConfig); strings.Contains(got, "//") {
		t.Errorf("config = %q, want the comments stripped before the embed", got)
	}
}

func TestBrokenConfigStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill": "<h1>home</h1>",
		"rill.jsonc":    "{\"i18n\": {\"mode\": \"domain\"}}",
	}))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Fatal("expected the build to stop")
	}
}

func TestRedirectsAreExportedForTheStaticEdge(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill": "<h1>home</h1>",
		"rill.jsonc":    "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}, \"redirects\": [{\"from\": \"/old\", \"to\": \"/\", \"status\": 302}]}",
	})
	got := read(t, dir, paths.Redirects)
	for _, want := range []string{"/old / 302", "/en/* /:splat 301"} {
		if !strings.Contains(got, want) {
			t.Errorf("_redirects = %q, want %q", got, want)
		}
	}
}

func TestNoRedirectsFileWithoutRules(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill": "<h1>home</h1>",
		"rill.jsonc":    "{\"i18n\": {\"mode\": \"single\"}}",
	})
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(paths.Redirects))); !os.IsNotExist(err) {
		t.Errorf("stat = %v, want the file to be absent", err)
	}
}

func TestFallbackProvidersAreGenerated(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill":         "<h1>home</h1>",
		"app/not-found.rill":    loaderPage,
		"app/api/ping/route.go": apiHandler,
	})
	registry := read(t, dir, RegistryGo)
	if !strings.Contains(registry, "not_found.Route") {
		t.Errorf("registry = %q", registry)
	}
	provider := read(t, dir, "internal/gen/not_found/provider.go")
	if !strings.Contains(provider, "const Route = \"not-found\"") {
		t.Errorf("provider = %q", provider)
	}
}

func TestRedirectsWriteFailureStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill": "<h1>home</h1>",
		"rill.jsonc":    "{\"redirects\": [{\"from\": \"/old\", \"to\": \"/\"}]}",
	}))
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.Redirects)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("redirects that cannot be written must fail the build")
	}
}

func TestConfigWriteFailureStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{"app/page.rill": "<h1>home</h1>"}))
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.GenConfig)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a config that cannot be written must fail the build")
	}
}

func TestFallbackWithoutALoaderNeedsNoProvider(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill":      "<h1>home</h1>",
		"app/not-found.rill": "<p>gone</p>",
	})
	if registry := read(t, dir, RegistryGo); strings.Contains(registry, "not_found") {
		t.Errorf("registry = %q, want no provider for a static fallback", registry)
	}
}

func TestApiRouteParametersReachTheAdapter(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill":                    "<h1>home</h1>",
		"app/api/listings/[id]/route.go":   apiHandler,
		"app/api/files/[...path]/route.go": apiHandler,
	})
	listings := read(t, dir, "internal/gen/api_listings_id/handler.go")
	if !strings.Contains(listings, `const Pattern = "/api/listings/{id}"`) {
		t.Errorf("adapter = %q", listings)
	}
	if !strings.Contains(listings, `var params = []string{"id"}`) {
		t.Errorf("adapter = %q", listings)
	}
	files := read(t, dir, "internal/gen/api_files_path/handler.go")
	if !strings.Contains(files, `const Pattern = "/api/files/{path...}"`) {
		t.Errorf("adapter = %q", files)
	}
}

func TestAdapterWriteFailureStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill":         "<h1>home</h1>",
		"app/api/ping/route.go": apiHandler,
	}))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	sealGen(t, dir)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("an adapter that cannot be written must fail the build")
	}
}

func TestStaticFilesAreCopiedAndHashed(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill":  "<h1>home</h1>",
		"styles/app.css": "body{margin:0}",
	})
	if got := read(t, dir, "internal/gen/styles/app.css"); got != "body{margin:0}" {
		t.Errorf("site copy = %q", got)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "dist", "assets", "assets", "app.*.css"))
	if err != nil || len(matches) != 1 {
		t.Errorf("dist copies = %v, err = %v", matches, err)
	}
}

func TestTheAssetsDirectoryAlwaysExists(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/page.rill": "<h1>home</h1>"})
	if _, err := os.Stat(filepath.Join(dir, "internal", "gen", "styles", ".keep")); err != nil {
		t.Errorf("stat = %v, want a placeholder so the embed compiles", err)
	}
}

func TestAssetsDirectiveInlinesASmallStylesheetAndLinksALargeOne(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/layout.rill": "<head>{% assets %}</head><body>{% outlet %}</body>",
		"app/page.rill":   "<h1>home</h1>",
		"styles/app.css":  "body{margin:0}",
	})
	page := read(t, dir, "dist/assets/index.html")
	if !strings.Contains(page, "<style>body{margin:0}</style>") || strings.Contains(page, `rel="stylesheet"`) {
		t.Errorf("page = %q, want a small sheet inlined", page)
	}

	dir = buildProject(t, map[string]string{
		"app/layout.rill": "<head>{% assets %}</head><body>{% outlet %}</body>",
		"app/page.rill":   "<h1>home</h1>",
		"styles/app.css":  sheet(config.DefaultInlineLimit + 1),
	})
	page = read(t, dir, "dist/assets/index.html")
	if !strings.Contains(page, `<link rel="stylesheet" href="/assets/app.`) || strings.Contains(page, "<style>") {
		t.Errorf("page = %q, want a sheet over the limit linked", page[:200])
	}
	sidecar := assets.ParseSidecar([]byte(read(t, dir, "internal/gen/bundles/"+assets.PreloadFile)))
	if len(sidecar.Links) != 1 || !strings.Contains(sidecar.Links[0], "as=style") {
		t.Errorf("sidecar = %+v, want the linked sheet in the early hints", sidecar)
	}
}

func sheet(size int) string {
	var b strings.Builder
	for index := 0; b.Len() < size; index++ {
		fmt.Fprintf(&b, ".c%x{--v:%x}", index*2654435761, index*40503)
	}
	return b.String()
}

func TestTheInlineLimitIsMeasuredInRawBytes(t *testing.T) {
	cases := map[string]struct {
		size   int
		inline bool
	}{
		"just under": {config.DefaultInlineLimit - 200, true},
		"just over":  {config.DefaultInlineLimit + 200, false},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			dir := buildProject(t, map[string]string{
				"app/layout.rill": "<head>{% assets %}</head><body>{% outlet %}</body>",
				"app/page.rill":   "<h1>home</h1>",
				"styles/app.css":  sheet(want.size),
			})
			page := read(t, dir, "dist/assets/index.html")
			if got := strings.Contains(page, "<style>"); got != want.inline {
				t.Errorf("inlined = %v, want %v for a sheet of about %d bytes", got, want.inline, want.size)
			}
		})
	}
}

func TestAZeroInlineLimitLinksEveryStylesheet(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/layout.rill": "<head>{% assets %}</head><body>{% outlet %}</body>",
		"app/page.rill":   "<h1>home</h1>",
		"styles/app.css":  "body{margin:0}",
		"rill.jsonc":      `{"css": {"inlineLimit": "0"}}`,
	})
	page := read(t, dir, "dist/assets/index.html")
	if strings.Contains(page, "<style>") || !strings.Contains(page, `<link rel="stylesheet"`) {
		t.Errorf("page = %q, want every sheet linked", page)
	}
	sidecar := assets.ParseSidecar([]byte(read(t, dir, "internal/gen/bundles/"+assets.PreloadFile)))
	if len(sidecar.Links) != 1 || !strings.Contains(sidecar.Links[0], "as=style") {
		t.Errorf("sidecar = %+v, want the sheet preloaded instead", sidecar)
	}
}

func TestTheInlineLimitCanBeRaised(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/layout.rill": "<head>{% assets %}</head><body>{% outlet %}</body>",
		"app/page.rill":   "<h1>home</h1>",
		"styles/app.css":  sheet(config.DefaultInlineLimit + 200),
		"rill.jsonc":      `{"css": {"inlineLimit": "64kb"}}`,
	})
	if page := read(t, dir, "dist/assets/index.html"); !strings.Contains(page, "<style>") {
		t.Errorf("page = %q, want a sheet under the raised limit inlined", page[:200])
	}
}

func TestAssetCopyFailureStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill":  "<h1>home</h1>",
		"styles/app.css": "body{margin:0}",
	}))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	sealGen(t, dir)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("assets that cannot be copied must fail the build")
	}
}

func TestStaleSiteAssetsCannotBeReplacedIsReported(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill":  "<h1>home</h1>",
		"styles/app.css": "body{margin:0}",
	}))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	denyWrites(t, filepath.Join(dir, "internal", "gen"))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a site directory that cannot be rewritten must fail the build")
	}
}

func TestHashedAssetCopyFailureStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill":  "<h1>home</h1>",
		"styles/app.css": "body{margin:0}",
	}))
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "assets"), []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a hashed asset that cannot be written must fail the build")
	}
}

func TestHeadersAreExportedForTheStaticEdge(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/page.rill": "<h1>home</h1>"})
	got := read(t, dir, paths.Headers)
	for _, want := range []string{"/assets/*", "immutable", "X-Content-Type-Options: nosniff"} {
		if !strings.Contains(got, want) {
			t.Errorf("_headers = %q, want %q", got, want)
		}
	}
}

func TestHeadersWriteFailureStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{"app/page.rill": "<h1>home</h1>"}))
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.Headers)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("headers that cannot be written must fail the build")
	}
}

const submitPage = `---
type Props struct {
	Heading string
}

type ContactForm struct {
	Email string ` + "`validate:\"required,email\"`" + `
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{Heading: "contact"}, nil
}

func Submit(ctx *rill.Ctx, params rill.Params, form ContactForm) (rill.Action, error) {
	return rill.RedirectTo("/"), nil
}
---
<Form><Field name="Email" /></Form>
`

func TestSubmitProvidersAreGenerated(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill":         "<h1>home</h1>",
		"app/contact/page.rill": submitPage,
	})
	provider := read(t, dir, "internal/gen/contact/provider.go")
	if !strings.Contains(provider, "var submitted ContactForm") {
		t.Errorf("provider = %q", provider)
	}
	if !strings.Contains(provider, "func SubmitProvider(") || !strings.Contains(provider, "rill.DecodeForm") {
		t.Errorf("provider = %q", provider)
	}
	registry := read(t, dir, RegistryGo)
	if !strings.Contains(registry, "contact.Route: contact.SubmitProvider") {
		t.Errorf("registry = %q", registry)
	}
}

func TestAPageWithOnlySubmitNeedsNoPropsProvider(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill": "<h1>home</h1>",
		"app/contact/page.rill": `---
type ContactForm struct {
	Email string
}

func Submit(ctx *rill.Ctx, params rill.Params, form ContactForm) (rill.Action, error) {
	return rill.RedirectTo("/"), nil
}
---
<Form><Field name="Email" /></Form>
`,
	})
	provider := read(t, dir, "internal/gen/contact/provider.go")
	if strings.Contains(provider, "func Provider(") {
		t.Errorf("provider = %q, want no props provider", provider)
	}
	registry := read(t, dir, RegistryGo)
	if strings.Contains(registry, "contact.Provider") {
		t.Errorf("registry = %q", registry)
	}
}

func TestAMalformedSubmitStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill": `---
func Submit(ctx *rill.Ctx) (rill.Action, error) {
	return nil, nil
}
---
<h1>home</h1>
`,
	}))
	_, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err == nil || !strings.Contains(err.Error(), "1 errors") {
		t.Errorf("err = %v", err)
	}
}

const islandClient = `export function mount(element: HTMLElement, props: { Label: string }): () => void {
	element.title = props.Label;
	return () => {};
}
`

func islandProject() map[string]string {
	return map[string]string{
		"app/layout.rill":                  "<head>{% assets %}</head><body>{% outlet %}</body>",
		"app/page.rill":                    `<Counter client="load" Label="hi" />`,
		"components/Counter/props.go":      "package counter\n\ntype Props struct {\n\tLabel string\n}\n",
		"components/Counter/template.rill": `<div class="counter">{{ Label }}</div>`,
		"components/Counter/client.ts":     islandClient,
	}
}

func TestIslandsAreBundledAndServed(t *testing.T) {
	dir := buildProject(t, islandProject())
	entries, err := os.ReadDir(filepath.Join(dir, "internal", "gen", "bundles"))
	if err != nil {
		t.Fatalf("read bundles: %v", err)
	}
	var entry, chunk string
	for _, item := range entries {
		name := item.Name()
		if strings.HasSuffix(name, assets.BrotliSuffix) || strings.HasSuffix(name, assets.GzipSuffix) {
			continue
		}
		switch {
		case strings.HasPrefix(name, bundle.RuntimePrefix):
			entry = name
		case strings.HasPrefix(name, "island."):
			chunk = name
		}
	}
	if entry == "" || chunk == "" {
		t.Fatalf("bundles = %v", entries)
	}
	for _, name := range []string{entry, chunk} {
		if _, err := os.Stat(filepath.Join(dir, "dist", "assets", "assets", name)); err != nil {
			t.Errorf("%s is not in dist: %v", name, err)
		}
	}
	page := read(t, dir, "dist/assets/index.html")
	if !strings.Contains(page, `src="/assets/`+entry+`"`) {
		t.Errorf("page = %q, want the hashed runtime linked", page)
	}
	if !strings.Contains(page, `<rill-island`) {
		t.Errorf("page = %q", page)
	}
}

func TestAProjectWithoutIslandsKeepsAnEmptyBundleStore(t *testing.T) {
	dir := buildProject(t, map[string]string{"app/page.rill": "<h1>home</h1>"})
	entries, err := os.ReadDir(filepath.Join(dir, "internal", "gen", "bundles"))
	if err != nil {
		t.Fatalf("the store must exist so the embed compiles: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, " ") != ".keep "+assets.PreloadFile {
		t.Errorf("entries = %v, want only the placeholder and the preload list", names)
	}
}

func TestABrokenIslandStopsTheBuild(t *testing.T) {
	files := islandProject()
	files["components/Counter/client.ts"] = "export function mount( {"
	dir := project(t, withModule(files))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("an island that does not compile must fail the build")
	}
}

func TestBundleWriteFailureStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(islandProject()))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "internal", "gen", "bundles")); err != nil {
		t.Fatal(err)
	}
	denyWrites(t, filepath.Join(dir, "internal", "gen"))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("bundles that cannot be written must fail the build")
	}
}

func TestAStaleBundleStoreThatCannotBeClearedStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(islandProject()))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	denyWrites(t, filepath.Join(dir, "internal", "gen"))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a bundle store that cannot be cleared must fail the build")
	}
}

func TestABundleThatCannotReachDistStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(islandProject()))
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "assets"), []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a bundle that cannot be written must fail the build")
	}
}

func TestAGeneratedPageThatCannotBeWrittenStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{
		"app/page.rill":         "<h1>home</h1>",
		"app/contact/page.rill": submitPage,
	}))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	sealGen(t, dir)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a generated page that cannot be written must fail the build")
	}
}

func TestIslandTypesAreGenerated(t *testing.T) {
	dir := buildProject(t, islandProject())
	types := read(t, dir, "internal/gen/props/Counter.ts")
	if !strings.Contains(types, "export interface Props {") || !strings.Contains(types, "Label: string;") {
		t.Errorf("types = %q", types)
	}
}

func TestATypeFileThatCannotBeWrittenStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(islandProject()))
	if err := os.MkdirAll(filepath.Join(dir, "internal", "gen", "props", "Counter.ts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a type file that cannot be written must fail the build")
	}
}

func TestAComponentWithoutASchemaGetsNoTypeFile(t *testing.T) {
	files := islandProject()
	delete(files, "components/Counter/props.go")
	files["app/page.rill"] = `<Counter client="load" />`
	files["components/Counter/template.rill"] = `<div class="counter">x</div>`
	dir := buildProject(t, files)
	if _, err := os.Stat(filepath.Join(dir, "internal", "gen", "props", "Counter.ts")); !os.IsNotExist(err) {
		t.Errorf("stat = %v, want no type file", err)
	}
}

func TestTheClassInventoryIsWritten(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill": `<div class="card lead"><span :class="{ 'wide': true }">x</span></div>`,
	})
	if got := read(t, dir, paths.Inventory); got != "card\nlead\nwide\n" {
		t.Errorf("inventory = %q", got)
	}
}

func TestAnInventoryThatCannotBeWrittenStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{"app/page.rill": `<div class="card">x</div>`}))
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.Inventory)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("an inventory that cannot be written must fail the build")
	}
}

const paramsPage = `---
type Props struct {
	Slug string
}

func Meta(ctx *rill.Ctx, p Props) rill.Meta {
	return rill.Meta{Title: p.Slug}
}

func Load(ctx *rill.Ctx, params rill.Params) (Props, error) {
	return Props{Slug: params["slug"]}, nil
}
---
<h1>{{ Slug }}</h1>
`

func TestALoaderCanTakeTheRouteParams(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill":              "<h1>home</h1>",
		"app/posts/[slug]/page.rill": paramsPage,
	})
	provider := read(t, dir, "internal/gen/posts_slug/provider.go")
	if !strings.Contains(provider, "Load(rill.NewCtx(request, params), params)") {
		t.Errorf("provider = %q, want the params passed on", provider)
	}
	if !strings.Contains(provider, "Load(ctx, params)") {
		t.Errorf("provider = %q, want the meta provider to pass them too", provider)
	}
}

func TestALoaderWithoutParamsIsCalledWithoutThem(t *testing.T) {
	dir := buildProject(t, map[string]string{
		"app/page.rill":         "<h1>home</h1>",
		"app/contact/page.rill": submitPage,
	})
	provider := read(t, dir, "internal/gen/contact/provider.go")
	if strings.Contains(provider, "Load(rill.NewCtx(request, params), params)") {
		t.Errorf("provider = %q, want the one argument form", provider)
	}
}

func publicProject() map[string]string {
	files := helloWorld()
	files["public/favicon.ico"] = "icon-bytes"
	files["public/img/hero.svg"] = "<svg/>"
	return files
}

func TestPublicFilesAreCopiedVerbatim(t *testing.T) {
	dir := buildProject(t, publicProject())
	for _, name := range []string{
		paths.GenPublic + "/favicon.ico",
		paths.GenPublic + "/img/hero.svg",
		paths.AssetsDir + "/favicon.ico",
		paths.AssetsDir + "/img/hero.svg",
	} {
		if got := read(t, dir, name); got == "" {
			t.Errorf("%s is empty", name)
		}
	}
	if got := read(t, dir, paths.AssetsDir+"/favicon.ico"); got != "icon-bytes" {
		t.Errorf("favicon = %q, want the bytes unchanged and unhashed", got)
	}
}

func TestAPublicFileThatCannotBeWrittenStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(publicProject()))
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.GenPublic), "favicon.ico", "blocked"), 0o755); err != nil {
		t.Fatal(err)
	}
	denyWrites(t, filepath.Join(dir, filepath.FromSlash(paths.GenPublic)))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a public file that cannot be written must fail the build")
	}
}

func TestBootstrapLeavesAProjectThatCompiles(t *testing.T) {
	dir := project(t, withModule(helloWorld()))
	if err := Bootstrap(dir); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	for _, name := range []string{RegistryGo, AppGo, EmbedGo, ServedGo, paths.Manifest, paths.GenConfig, paths.GenStyles + "/.keep", paths.GenBundles + "/.keep", paths.GenPublic + "/.keep"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	app := read(t, dir, AppGo)
	for _, want := range []string{
		"//go:embed manifest.bin",
		"//go:embed bundles/" + assets.PreloadFile,
		"Preload:  Preload,",
		"func Options() rill.Options",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("app.go is missing %q:\n%s", want, app)
		}
	}

	embedded := read(t, dir, EmbedGo)
	for _, want := range []string{"//go:build !js", "//go:embed all:public", "//go:embed all:styles", "//go:embed all:bundles"} {
		if !strings.Contains(embedded, want) {
			t.Errorf("embed.go is missing %q:\n%s", want, embedded)
		}
	}

	served := read(t, dir, ServedGo)
	if !strings.Contains(served, "//go:build js") {
		t.Errorf("embed_js.go is missing its build tag:\n%s", served)
	}
	if strings.Contains(served, "//go:embed") {
		t.Errorf("the worker build must embed no assets:\n%s", served)
	}

	mustParse(t, read(t, dir, RegistryGo))
	mustParse(t, app)
	mustParse(t, embedded)
	mustParse(t, served)
}

func TestBootstrapNeedsAModule(t *testing.T) {
	if err := Bootstrap(t.TempDir()); err == nil {
		t.Error("Bootstrap must report a directory without a go.mod")
	}
}

func TestIslandTypesAreDeclaredForTheAlias(t *testing.T) {
	dir := buildProject(t, islandProject())
	declarations := read(t, dir, paths.PropsTypes)
	if !strings.Contains(declarations, `declare module "rill:props/Counter"`) {
		t.Errorf("props.d.ts = %q, want the alias declared", declarations)
	}
	if !strings.Contains(declarations, `export * from "./props/Counter"`) {
		t.Errorf("props.d.ts = %q, want it to point at the generated types", declarations)
	}
}

func TestASingleFileIslandIsBundled(t *testing.T) {
	files := withModule(map[string]string{
		"app/layout.rill": "<html><body>{% outlet %}</body></html>",
		"app/page.rill":   `<h1>x</h1><Ticker client="load" :Start="2" />`,
		"components/Ticker.rill": `---
type Props struct {
	Start int
}
---
<output>{{ Start }}</output>

<script client>
export function mount(el: HTMLElement): () => void {
	el.dataset.ready = "1";
	return () => {};
}
</script>
`,
	})
	dir := project(t, files)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(paths.AssetsDir), "assets"))
	if err != nil {
		t.Fatalf("read assets: %v", err)
	}
	var chunks int
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".js") {
			chunks++
		}
	}
	if chunks < 2 {
		t.Errorf("assets = %v, want the runtime and the island chunk", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(paths.CacheDir), "islands", "Ticker.ts")); err != nil {
		t.Errorf("the island source was not staged for the bundler: %v", err)
	}
}

func TestAStalePublicFileIsRemoved(t *testing.T) {
	dir := buildProject(t, publicProject())
	stale := filepath.Join(dir, filepath.FromSlash(paths.GenPublic), "gone.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("a public file that no longer exists must not survive a rebuild")
	}
}

func TestAPublicDirectoryThatCannotBeClearedStopsTheBuild(t *testing.T) {
	dir := buildProject(t, publicProject())
	genDir := filepath.Join(dir, paths.GenRoot)
	denyWrites(t, genDir)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a generated tree that cannot be cleared must fail the build")
	}
}

func TestBootstrapReportsAFileItCannotWrite(t *testing.T) {
	dir := project(t, withModule(helloWorld()))
	if err := os.MkdirAll(filepath.Join(dir, paths.GenRoot, "registry.go", "blocked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(dir); err == nil {
		t.Error("Bootstrap must report a file it cannot write")
	}
}

func TestStalePackagesThatCannotBeRemovedStopTheBuild(t *testing.T) {
	dir := project(t, withModule(map[string]string{"app/features/page.rill": loaderPage}))
	stale := filepath.Join(dir, paths.GenRoot, "gone")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	denyWrites(t, filepath.Join(dir, paths.GenRoot))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a stale package that cannot be removed must fail the build")
	}
}

type unreadablePublic struct{ fs.FS }

func (u unreadablePublic) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, ".ico") {
		return nil, errors.New("unreadable")
	}
	return u.FS.Open(name)
}

func TestWritePublicReportsAFileItCannotRead(t *testing.T) {
	source := unreadablePublic{FS: fstest.MapFS{
		"public/favicon.ico": &fstest.MapFile{Data: []byte("icon")},
	}}
	if err := writePublic(t.TempDir(), source); err == nil {
		t.Error("a public file that cannot be read must fail the build")
	}
}

func TestWritePublicReportsAnUnreadableTree(t *testing.T) {
	if err := writePublic(t.TempDir(), brokenTree{}); err == nil {
		t.Error("a public tree that cannot be walked must fail the build")
	}
}

type brokenTree struct{}

func (brokenTree) Open(name string) (fs.File, error) {
	if name == assets.PublicDir {
		return fstest.MapFS{"favicon.ico": &fstest.MapFile{Data: []byte("icon")}}.Open(".")
	}
	return nil, errors.New("unreadable")
}

func TestWriteIslandTypesReportsADeclarationItCannotWrite(t *testing.T) {
	dir := project(t, withModule(islandProject()))
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.PropsTypes)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a declaration file that cannot be written must fail the build")
	}
}

func TestWritePublicKeepsTheDirectoryForTheEmbed(t *testing.T) {
	dir := buildProject(t, helloWorld())
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(paths.GenPublic), ".keep")); err != nil {
		t.Errorf("a project without public files still needs the directory: %v", err)
	}
}

func TestABundleThatCannotBeWrittenStopsTheBuild(t *testing.T) {
	dir := project(t, withModule(islandProject()))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	bundles := filepath.Join(dir, filepath.FromSlash(paths.GenBundles))
	if err := os.RemoveAll(bundles); err != nil {
		t.Fatal(err)
	}
	denyWrites(t, filepath.Join(dir, paths.GenRoot))
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a bundle that cannot be written must fail the build")
	}
}

func TestBrokenCssStopsTheBuild(t *testing.T) {
	files := withModule(helloWorld())
	files["styles/app.css"] = "@media screen { .a { color"
	dir := project(t, files)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Logf("css that the minifier rejects fails the build: %v", err)
	}
}

func TestCompressedVariantsSitNextToTheAsset(t *testing.T) {
	files := withModule(helloWorld())
	var css strings.Builder
	for index := range 200 {
		fmt.Fprintf(&css, ".card-%d{color:#fff;background:#000;padding:%drem}\n", index, index)
	}
	files["styles/app.css"] = css.String()
	dir := project(t, files)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(paths.GenStyles)))
	if err != nil {
		t.Fatalf("read assets: %v", err)
	}
	var brotli, gzip bool
	for _, entry := range entries {
		brotli = brotli || strings.HasSuffix(entry.Name(), assets.BrotliSuffix)
		gzip = gzip || strings.HasSuffix(entry.Name(), assets.GzipSuffix)
	}
	if !brotli || !gzip {
		t.Errorf("assets = %v, want precompressed variants", entries)
	}
}

func TestWriteServedReportsAVariantItCannotCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.css")
	if err := os.MkdirAll(path+assets.BrotliSuffix, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte(strings.Repeat("body{color:#fff}", 100))
	if err := writeServed(path, "text/css", content); err == nil {
		t.Error("a compressed variant that cannot be created must be reported")
	}
}

func TestWriteServedSkipsCompressionForSmallFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.css")
	if err := writeServed(path, "text/css", []byte("body{}")); err != nil {
		t.Fatalf("writeServed: %v", err)
	}
	if _, err := os.Stat(path + assets.BrotliSuffix); err == nil {
		t.Error("a file below the threshold must not be compressed")
	}
}

func TestMinifiedOnlyTouchesStylesheets(t *testing.T) {
	script := []byte("const a = 1;\n\n")
	got, err := minified(assets.Asset{Kind: assets.KindScript}, script)
	if err != nil {
		t.Fatalf("minified: %v", err)
	}
	if string(got) != string(script) {
		t.Errorf("script = %q, want it untouched", got)
	}
	style, err := minified(assets.Asset{Kind: assets.KindStyle}, []byte("body {\n  color: #ffffff;\n}\n"))
	if err != nil {
		t.Fatalf("minified: %v", err)
	}
	if len(style) >= len("body {\n  color: #ffffff;\n}\n") {
		t.Errorf("style = %q, want it minified", style)
	}
}

type recordingStyles struct {
	calls  int
	input  string
	output string
}

func (r *recordingStyles) Process(input, output, _ string) error {
	r.calls++
	r.input, r.output = input, output
	expanded := ".from-tailwind{color:red}" + sheet(config.DefaultInlineLimit)
	return os.WriteFile(output, []byte(expanded), 0o644)
}

func TestTailwindReplacesTheStylesheet(t *testing.T) {
	files := withModule(helloWorld())
	files["app/layout.rill"] = "<html><head>{% assets %}</head><body>{% outlet %}</body></html>"
	files["styles/app.css"] = "@import \"tailwindcss\";"
	files["rill.jsonc"] = "{\"app\": {\"name\": \"demo\"}, \"css\": {\"engine\": \"tailwind\"}}"
	dir := project(t, files)
	styles := &recordingStyles{}
	if _, err := Run(Options{Dir: dir, Styles: styles, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if styles.calls != 1 {
		t.Fatalf("calls = %d, want the processor run once", styles.calls)
	}
	entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(paths.AssetsDir), "assets"))
	if err != nil {
		t.Fatalf("read assets: %v", err)
	}
	var served string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".css") {
			served = entry.Name()
		}
	}
	if served == "" {
		t.Fatalf("assets = %v, want a stylesheet", entries)
	}
	content := read(t, dir, paths.AssetsDir+"/assets/"+served)
	if !strings.Contains(content, "from-tailwind") {
		t.Errorf("stylesheet = %q, want the processed css", content)
	}
	page := read(t, dir, paths.AssetsDir+"/index.html")
	if !strings.Contains(page, `<link rel="stylesheet" href="/assets/`+served+`"`) {
		t.Errorf("page = %q, want the processed sheet linked, not copied into every document", page)
	}
}

func TestThePlainEngineSkipsTheProcessor(t *testing.T) {
	files := withModule(helloWorld())
	files["styles/app.css"] = "body {\n  color: #ffffff;\n}\n"
	files["rill.jsonc"] = "{\"app\": {\"name\": \"demo\"}, \"css\": {\"engine\": \"plain\"}}"
	dir := project(t, files)
	styles := &recordingStyles{}
	if _, err := Run(Options{Dir: dir, Styles: styles, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if styles.calls != 0 {
		t.Errorf("calls = %d, want the processor left alone", styles.calls)
	}
}

func TestAStylesheetTheProcessorRejectsStopsTheBuild(t *testing.T) {
	files := withModule(helloWorld())
	files["styles/app.css"] = "@import \"tailwindcss\";"
	files["rill.jsonc"] = "{\"app\": {\"name\": \"demo\"}, \"css\": {\"engine\": \"tailwind\"}}"
	dir := project(t, files)
	if _, err := Run(Options{Dir: dir, Styles: failingStyles{}, Runner: &recorder{}}); err == nil {
		t.Error("a processor failure must stop the build")
	}
}

type failingStyles struct{}

func (failingStyles) Process(string, string, string) error {
	return errors.New("tailwind is missing")
}

func TestDeferredProvidersReachTheRegistry(t *testing.T) {
	files := withModule(map[string]string{
		"app/layout.rill": "<html><body>{% outlet %}</body></html>",
		"app/page.rill": `---
type Row struct {
	Name string
}

type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Rows(ctx *rill.Ctx) ([]Row, error) {
	return nil, nil
}
---
<h1>{{ Heading }}</h1>
{% fragment "Rows" defer %}{% for row in Rows %}<b>{{ row.Name }}</b>{% endfor %}{% endfragment %}
`,
	})
	dir := project(t, files)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	registry := read(t, dir, RegistryGo)
	if !strings.Contains(registry, `"Rows": index.RowsProvider`) {
		t.Errorf("registry is missing the deferred provider:\n%s", registry)
	}
	provider := read(t, dir, paths.GenRoot+"/index/provider.go")
	if !strings.Contains(provider, "func RowsProvider(") || !strings.Contains(provider, "deferredRows{value: value}") {
		t.Errorf("provider = %q", provider)
	}
	mustParse(t, registry)
	mustParse(t, provider)
}

func TestGeneratedGoIsFormatted(t *testing.T) {
	files := withModule(map[string]string{
		"app/layout.rill":    "<html><body>{% outlet %}</body></html>",
		"app/page.rill":      loaderPage,
		"app/api/x/route.go": "package route\n\nimport \"github.com/apptivitypl/rill\"\n\nfunc GET(ctx *rill.Ctx, params rill.Params) (rill.Response, error) {\n\treturn rill.Text(\"ok\"), nil\n}\n",
	})
	dir := project(t, files)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	err := filepath.WalkDir(filepath.Join(dir, paths.GenRoot), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		formatted, err := format.Source(source)
		if err != nil {
			return fmt.Errorf("%s does not parse: %w", path, err)
		}
		if !bytes.Equal(source, formatted) {
			t.Errorf("%s is not gofmt clean", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestWriteGoRefusesCodeThatDoesNotParse(t *testing.T) {
	err := writeGo(filepath.Join(t.TempDir(), "broken.go"), []byte("package x\n\nfunc ("))
	if err == nil || !strings.Contains(err.Error(), "does not parse") {
		t.Errorf("err = %v, want the parse failure", err)
	}
}

func TestWriteGoFormatsBeforeWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ok.go")
	if err := writeGo(path, []byte("package x\n\nvar  a   =  1\n")); err != nil {
		t.Fatalf("writeGo: %v", err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(source) != "package x\n\nvar a = 1\n" {
		t.Errorf("source = %q, want it gofmt clean", source)
	}
}

func TestBootstrapReportsAPlaceholderItCannotWrite(t *testing.T) {
	dir := project(t, withModule(helloWorld()))
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.Manifest), "blocked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(dir); err == nil {
		t.Error("Bootstrap must report a placeholder it cannot write")
	}
}

func TestBootstrapReportsGeneratedCodeItCannotWrite(t *testing.T) {
	dir := project(t, withModule(helloWorld()))
	if err := os.MkdirAll(filepath.Join(dir, paths.GenRoot, "app.go", "blocked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(dir); err == nil {
		t.Error("Bootstrap must report generated code it cannot write")
	}
}

func reactIslandProject() map[string]string {
	files := islandProject()
	files["app/page.rill"] = `<Counter client="load" Label="hi" /><Chart client="visible" Label="five" />`
	files["components/Chart/props.go"] = "package chart\n\ntype Props struct {\n\tLabel string\n}\n"
	files["components/Chart/template.rill"] = `<div class="chart"></div>`
	files["components/Chart/client.tsx"] = `import type { Props } from "rill:props/Chart";
export default function Chart(props: Props) { return <b>{props.Label}</b>; }
`
	files["node_modules/react/package.json"] = `{"name":"react","type":"module","exports":{".":"./index.js","./jsx-runtime":"./jsx-runtime.js"}}`
	files["node_modules/react/index.js"] = "export function createElement(type, props) { return { type, props }; }\nexport const marker = 'react-shared';\n"
	files["node_modules/react/jsx-runtime.js"] = "export function jsx(type, props) { return { type, props }; }\nexport const jsxs = jsx;\n"
	files["node_modules/react-dom/package.json"] = `{"name":"react-dom","type":"module","exports":{"./client":"./client.js"}}`
	files["node_modules/react-dom/client.js"] = "import { marker } from 'react';\nexport function createRoot(el) { return { render() { el.dataset.m = marker; }, unmount() {} }; }\n"
	return files
}

func TestAReactIslandBuildsNextToAPlainOne(t *testing.T) {
	dir := buildProject(t, reactIslandProject())
	entries, err := os.ReadDir(filepath.Join(dir, "internal", "gen", "bundles"))
	if err != nil {
		t.Fatalf("read bundles: %v", err)
	}
	var chunks, shared int
	for _, item := range entries {
		name := item.Name()
		if strings.HasSuffix(name, assets.BrotliSuffix) || strings.HasSuffix(name, assets.GzipSuffix) {
			continue
		}
		if !strings.HasPrefix(name, "island.") {
			continue
		}
		chunks++
		body := read(t, dir, "internal/gen/bundles/"+name)
		if strings.Contains(body, "react-shared") {
			shared++
		}
	}
	if chunks < 2 || shared != 1 {
		t.Errorf("chunks = %d, react in %d of them; want both islands bundled and react in exactly one place", chunks, shared)
	}
	page := read(t, dir, "dist/assets/index.html")
	if strings.Contains(page, "island.") {
		t.Errorf("page = %q, want island chunks reached only through the entry's dynamic imports", page)
	}
	if !strings.Contains(page, `name="Chart"`) || !strings.Contains(page, `<div class="chart"></div>`) {
		t.Errorf("page = %q, want the react island's server markup as the first paint", page)
	}
}

func TestAnEagerHelperChunkIsPreloadedAndRecorded(t *testing.T) {
	files := reactIslandProject()
	files["node_modules/react/package.json"] = `{"name":"react","main":"index.js"}`
	files["node_modules/react/index.js"] = "module.exports = { createElement(type, props) { return { type, props }; }, marker: 'react-shared' };\n"
	files["node_modules/react/jsx-runtime.js"] = "module.exports = { jsx(t, p) { return { t, p }; }, jsxs(t, p) { return { t, p }; } };\n"
	files["node_modules/react-dom/package.json"] = `{"name":"react-dom","exports":{"./client":"./client.js"}}`
	files["node_modules/react-dom/client.js"] = "const react = require('react');\nmodule.exports = { createRoot(el) { return { render() { el.dataset.m = react.marker; }, unmount() {} }; } };\n"
	dir := buildProject(t, files)

	sidecar := assets.ParseSidecar([]byte(read(t, dir, "internal/gen/bundles/"+assets.PreloadFile)))
	if len(sidecar.Modules) != 1 {
		t.Fatalf("sidecar = %+v, want the helper chunk the entry imports statically", sidecar)
	}
	helper := sidecar.Modules[0]
	if len(sidecar.Islands["Chart"]) < 1 || len(sidecar.Links) == 0 {
		t.Errorf("sidecar = %+v, want the island map and the link list", sidecar)
	}
	page := read(t, dir, "dist/assets/index.html")
	if !strings.Contains(page, `<link rel="modulepreload" href="/assets/`+helper+`">`) {
		t.Errorf("page = %q, want the helper chunk preloaded", page)
	}
	if strings.Count(page, "modulepreload") != 1 {
		t.Errorf("page = %q, want lazy island chunks left out of the preloads", page)
	}
}

func sealGen(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(paths.GenRoot))); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, filepath.FromSlash(path.Dir(paths.GenRoot)))
	denyWrites(t, parent)
}

func denyWrites(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("windows leaves the owner able to write into a read-only directory")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skipf("cannot make %s read-only: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

func TestTheWorkerKeepsTheSidecarWithoutTheAssets(t *testing.T) {
	dir := buildProject(t, islandProject())

	app := read(t, dir, AppGo)
	if !strings.Contains(app, "//go:embed bundles/"+assets.PreloadFile) {
		t.Errorf("app.go must embed the sidecar on every target:\n%s", app)
	}
	if strings.Contains(app, "//go:embed all:") {
		t.Errorf("the asset trees belong in the tagged file, not app.go:\n%s", app)
	}

	served := read(t, dir, ServedGo)
	if strings.Contains(served, "//go:embed") {
		t.Errorf("the worker build must carry no asset bytes:\n%s", served)
	}

	sidecar := filepath.Join(dir, filepath.FromSlash(paths.GenBundles), assets.PreloadFile)
	held, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if !strings.Contains(string(held), "island ") {
		t.Errorf("sidecar = %q, want the island chunks the worker preloads", held)
	}
}
