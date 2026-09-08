package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

const bareLayout = "{% standalone %}<!doctype html><html><head>{% meta %}{% assets %}</head>" +
	"<body>{% outlet %}</body></html>"

func layered(files map[string]string) fstest.MapFS {
	tree := app(files)
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte(
		"<!doctype html><html><head>{% meta %}{% assets %}</head><body><header>site</header>{% outlet %}</body></html>")}
	return tree
}

func compiled(t *testing.T, files fstest.MapFS) (Result, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return result, &bag
}

func chainFor(t *testing.T, result Result, pattern string) []uint32 {
	t.Helper()
	for _, route := range result.Manifest.Routes {
		if route.Pattern == pattern {
			return route.LayoutChain
		}
	}
	t.Fatalf("no route answers %s", pattern)
	return nil
}

func TestAStandaloneLayoutStartsTheChain(t *testing.T) {
	result, bag := compiled(t, layered(map[string]string{
		"app/page.gopage":              "<p>home</p>",
		"app/(auth)/layout.gopage":     bareLayout,
		"app/(auth)/login/page.gopage": "<form>login</form>",
	}))
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if got := chainFor(t, result, "/login"); len(got) != 1 {
		t.Errorf("login chain = %v, want the standalone layout alone", got)
	}
	if got := chainFor(t, result, "/"); len(got) != 1 {
		t.Errorf("home chain = %v, want the root layout", got)
	}
}

func TestWithoutTheDirectiveLayoutsStillNest(t *testing.T) {
	result, bag := compiled(t, layered(map[string]string{
		"app/page.gopage":              "<p>home</p>",
		"app/(auth)/layout.gopage":     "<section>{% outlet %}</section>",
		"app/(auth)/login/page.gopage": "<form>login</form>",
	}))
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if got := chainFor(t, result, "/login"); len(got) != 2 {
		t.Errorf("login chain = %v, want both layouts", got)
	}
}

func TestTheDeeperStandaloneLayoutWins(t *testing.T) {
	result, bag := compiled(t, layered(map[string]string{
		"app/page.gopage":                "<p>home</p>",
		"app/(auth)/layout.gopage":       bareLayout,
		"app/(auth)/login/layout.gopage": bareLayout,
		"app/(auth)/login/page.gopage":   "<form>login</form>",
	}))
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if got := chainFor(t, result, "/login"); len(got) != 1 {
		t.Errorf("chain = %v, want only the deeper layout", got)
	}
}

func TestAFallbackFollowsTheSameBoundary(t *testing.T) {
	result, bag := compiled(t, layered(map[string]string{
		"app/page.gopage":              "<p>home</p>",
		"app/(auth)/layout.gopage":     bareLayout,
		"app/(auth)/not-found.gopage":  "<p>nothing here</p>",
		"app/(auth)/login/page.gopage": "<form>login</form>",
	}))
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	var fallback ir.Fallback
	for _, entry := range result.Manifest.Fallbacks {
		if strings.Contains(entry.Prefix, "/") {
			fallback = entry
		}
	}
	if len(fallback.LayoutChain) != 1 {
		t.Errorf("fallback chain = %v", fallback.LayoutChain)
	}
}

func TestStandaloneOutsideALayoutIsRejected(t *testing.T) {
	_, bag := compiled(t, layered(map[string]string{
		"app/page.gopage": "{% standalone %}<p>home</p>",
	}))
	item, ok := found(bag, diag.C113)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Help, "layout") {
		t.Errorf("help = %q", item.Help)
	}
}

func TestAStandaloneLayoutWithoutAHeadWarns(t *testing.T) {
	cases := map[string]string{
		"{% assets %}": "{% standalone %}<html><head>{% meta %}</head><body>{% outlet %}</body></html>",
		"{% meta %}":   "{% standalone %}<html><head>{% assets %}</head><body>{% outlet %}</body></html>",
	}
	for missing, layout := range cases {
		_, bag := compiled(t, layered(map[string]string{
			"app/page.gopage":              "<p>home</p>",
			"app/(auth)/layout.gopage":     layout,
			"app/(auth)/login/page.gopage": "<form>login</form>",
		}))
		item, ok := found(bag, diag.W705)
		if !ok {
			t.Fatalf("%s: diagnostics = %+v", missing, bag.Items())
		}
		if !strings.Contains(item.Message, missing) {
			t.Errorf("message = %q, want %q named", item.Message, missing)
		}
		if bag.HasErrors() {
			t.Errorf("%s: a warning is not an error: %+v", missing, bag.Items())
		}
	}
}

func TestAnOrdinaryLayoutWithoutAHeadIsSilent(t *testing.T) {
	_, bag := compiled(t, layered(map[string]string{
		"app/page.gopage":              "<p>home</p>",
		"app/(auth)/layout.gopage":     "<section>{% outlet %}</section>",
		"app/(auth)/login/page.gopage": "<form>login</form>",
	}))
	if _, ok := found(bag, diag.W705); ok {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}

func TestAStandaloneLayoutRendersWithoutTheRoot(t *testing.T) {
	result, bag := compiled(t, layered(map[string]string{
		"app/page.gopage":              "<p>home</p>",
		"app/(auth)/layout.gopage":     "{% standalone %}<body>{% assets %}<main>{% outlet %}</main></body>",
		"app/(auth)/login/page.gopage": "<form>login</form>",
	}))
	if _, ok := found(bag, diag.W705); !ok {
		t.Fatalf("a layout without {%% meta %%} warns: %+v", bag.Items())
	}
	login := render(t, result, "/login")
	if strings.Contains(login, "<header>site</header>") {
		t.Errorf("login carries the site chrome: %q", login)
	}
	if !strings.Contains(login, "<form>login</form>") || !strings.Contains(login, "<main>") {
		t.Errorf("login = %q", login)
	}
	if home := render(t, result, "/"); !strings.Contains(home, "<header>site</header>") {
		t.Errorf("home lost the root layout: %q", home)
	}
}

func render(t *testing.T, result Result, pattern string) string {
	t.Helper()
	route, ok := result.Manifest.Lookup(pattern)
	if !ok {
		t.Fatalf("no route answers %s", pattern)
	}
	out := runtime.NewBuffer(512)
	props := runtime.WithMeta(runtime.Empty{}, runtime.Meta{})
	if err := runtime.Render(result.Manifest.Chain(route), props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}
