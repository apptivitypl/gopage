package compile

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

func file(content string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(content)}
}

func discover(t *testing.T, files fstest.MapFS) ([]Route, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	return Discover(files, &bag), &bag
}

func patterns(routes []Route) []string {
	out := make([]string, len(routes))
	for i, route := range routes {
		out[i] = route.Pattern
	}
	return out
}

func hasCode(bag *diag.Bag, want diag.Code) bool {
	for _, d := range bag.Items() {
		if d.Code == want {
			return true
		}
	}
	return false
}

func TestDiscoverMapsDirectoriesToPatterns(t *testing.T) {
	routes, bag := discover(t, fstest.MapFS{
		"app/page.rill":                file("home"),
		"app/listings/page.rill":       file("list"),
		"app/listings/[id]/page.rill":  file("detail"),
		"app/docs/[...slug]/page.rill": file("docs"),
	})
	if bag.Len() != 0 {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	want := []string{"/", "/docs/[...slug]", "/listings", "/listings/[id]"}
	if got := patterns(routes); !reflect.DeepEqual(got, want) {
		t.Errorf("patterns = %v, want %v", got, want)
	}
}

func TestGroupsDoNotAppearInTheURL(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{"app/(marketing)/about/page.rill": file("about")})
	if routes[0].Pattern != "/about" {
		t.Errorf("pattern = %q, want the group stripped", routes[0].Pattern)
	}
}

func TestGroupOnlyPathIsTheRoot(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{"app/(marketing)/page.rill": file("home")})
	if routes[0].Pattern != "/" {
		t.Errorf("pattern = %q, want /", routes[0].Pattern)
	}
}

func TestRouteNames(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{
		"app/page.rill":               file("home"),
		"app/listings/[id]/page.rill": file("detail"),
	})
	names := map[string]string{}
	for _, route := range routes {
		names[route.Pattern] = route.Name
	}
	if names["/"] != "index" {
		t.Errorf("root name = %q", names["/"])
	}
	if names["/listings/[id]"] != "listings.id" {
		t.Errorf("detail name = %q", names["/listings/[id]"])
	}
}

func TestLayoutChainRunsOutermostFirst(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{
		"app/layout.rill":             file("root"),
		"app/listings/layout.rill":    file("section"),
		"app/listings/[id]/page.rill": file("detail"),
		"app/other/page.rill":         file("other"),
	})
	byPattern := map[string][]string{}
	for _, route := range routes {
		byPattern[route.Pattern] = route.Layouts
	}
	want := []string{"app/layout.rill", "app/listings/layout.rill"}
	if got := byPattern["/listings/[id]"]; !reflect.DeepEqual(got, want) {
		t.Errorf("chain = %v, want %v", got, want)
	}
	if got := byPattern["/other"]; !reflect.DeepEqual(got, []string{"app/layout.rill"}) {
		t.Errorf("sibling chain = %v", got)
	}
}

func TestSiblingLayoutDoesNotLeakAcrossDirectories(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{
		"app/listings/layout.rill":       file("section"),
		"app/listings-archive/page.rill": file("archive"),
	})
	if len(routes[0].Layouts) != 0 {
		t.Errorf("layouts = %v, want none", routes[0].Layouts)
	}
}

func TestRouteHandlersAreDiscovered(t *testing.T) {
	routes, bag := discover(t, fstest.MapFS{"app/api/health/route.go": file("package route")})
	if bag.Len() != 0 {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	if routes[0].Kind != RouteAPI || routes[0].Pattern != "/api/health" {
		t.Errorf("route = %+v", routes[0])
	}
	if routes[0].Localized() {
		t.Error("api routes must never be localized")
	}
}

func TestApiHandlersGetNoLayouts(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{
		"app/layout.rill":         file("root"),
		"app/api/health/route.go": file("package route"),
	})
	if len(routes[0].Layouts) != 0 {
		t.Errorf("layouts = %v, want none for an api handler", routes[0].Layouts)
	}
}

func TestPagesAreLocalized(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{"app/about/page.rill": file("about")})
	if !routes[0].Localized() {
		t.Error("pages outside /api are localized")
	}
}

func TestPageUnderApiReportsC102(t *testing.T) {
	_, bag := discover(t, fstest.MapFS{"app/api/oops/page.rill": file("nope")})
	if !hasCode(bag, diag.C102) {
		t.Errorf("diagnostics = %+v, want C102", bag.Items())
	}
}

func TestPageAndHandlerInOneSegmentReportsC101(t *testing.T) {
	routes, bag := discover(t, fstest.MapFS{
		"app/thing/page.rill": file("page"),
		"app/thing/route.go":  file("package route"),
	})
	if !hasCode(bag, diag.C101) {
		t.Errorf("diagnostics = %+v, want C101", bag.Items())
	}
	if len(routes) != 1 {
		t.Errorf("routes = %v, want the conflict collapsed to one", patterns(routes))
	}
}

func TestGroupsCollidingOnOnePatternReportC101(t *testing.T) {
	_, bag := discover(t, fstest.MapFS{
		"app/(a)/about/page.rill": file("one"),
		"app/(b)/about/page.rill": file("two"),
	})
	if !hasCode(bag, diag.C101) {
		t.Errorf("diagnostics = %+v, want C101", bag.Items())
	}
}

func TestDiscoverOnAnEmptyTreeFindsNothing(t *testing.T) {
	routes, bag := discover(t, fstest.MapFS{})
	if len(routes) != 0 || bag.Len() != 0 {
		t.Errorf("routes = %v, diagnostics = %+v", patterns(routes), bag.Items())
	}
}

func TestParamsOfReadsDynamicSegments(t *testing.T) {
	cases := map[string][]string{
		"/listings/[id]":  {"id"},
		"/docs/[...slug]": {"slug"},
		"/a/[b]/c/[d]":    {"b", "d"},
		"/static":         nil,
	}
	for pattern, want := range cases {
		if got := ParamsOf(pattern); !reflect.DeepEqual(got, want) {
			t.Errorf("ParamsOf(%q) = %v, want %v", pattern, got, want)
		}
	}
}

func TestPhasesAreOrdered(t *testing.T) {
	names := Phases()
	if names[0] != "discover routes" {
		t.Errorf("phases = %v, want discovery first", names)
	}
	if names[len(names)-1] != "build manifest" {
		t.Errorf("phases = %v, want the manifest last", names)
	}
}

func TestCompileProducesARenderableManifest(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{
		"app/layout.rill": file("<html><body>{% outlet %}</body></html>"),
		"app/page.rill":   file("<h1>{{ Title }}</h1>"),
	}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.Len() != 0 {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}

	route, ok := result.Manifest.Lookup("/")
	if !ok {
		t.Fatal("the root route is missing from the manifest")
	}
	if route.Class != ir.ClassStatic {
		t.Errorf("class = %s, want static", route.Class)
	}

	out := runtime.NewBuffer(128)
	chain := result.Manifest.Chain(route)
	if err := runtime.Render(chain, runtime.Map{"Title": runtime.String("hi & bye")}, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "<html><body><h1>hi &amp; bye</h1></body></html>"
	if out.String() != want {
		t.Errorf("render =\n%q\nwant\n%q", out.String(), want)
	}
}

func TestCompileMarksDynamicRoutes(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/listings/[id]/page.rill": file("x")}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	route, _ := result.Manifest.Lookup("/listings/[id]")
	if route.Class != ir.ClassDynamic {
		t.Errorf("class = %s, want dynamic", route.Class)
	}
}

func TestCompileSharesOnePlanPerLayout(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{
		"app/layout.rill": file("{% outlet %}"),
		"app/a/page.rill": file("a"),
		"app/b/page.rill": file("b"),
	}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(result.Manifest.Plans) != 3 {
		t.Errorf("plans = %d, want one layout plus two pages", len(result.Manifest.Plans))
	}
}

func TestCompileKeepsApiRoutesOutOfTheRenderManifest(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/api/health/route.go": file("package route")}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(result.Manifest.Routes) != 0 {
		t.Errorf("manifest routes = %+v, want none", result.Manifest.Routes)
	}
	if len(result.Routes) != 1 {
		t.Errorf("discovered routes = %d, want the handler", len(result.Routes))
	}
}

func TestOutletInsideAPageReportsC103(t *testing.T) {
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.rill": file("{% outlet %}")}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C103) {
		t.Errorf("diagnostics = %+v, want C103", bag.Items())
	}
}

func TestCompileSurvivesABrokenTemplate(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": file("<p>{{ ")}, &bag)
	if err != nil {
		t.Fatalf("Compile must not fail hard on a syntax error: %v", err)
	}
	if !bag.HasErrors() {
		t.Error("a broken template must produce diagnostics")
	}
	if len(result.Manifest.Routes) != 1 {
		t.Error("the route must still be known so the error can be reported against it")
	}
}

func TestLowerDeduplicatesRepeatedPaths(t *testing.T) {
	var bag diag.Bag
	result, _ := Compile(fstest.MapFS{"app/page.rill": file("{{ A }}{{ A }}{{ B }}")}, &bag)
	plan := result.Manifest.Plans[0]
	if len(plan.Paths) != 2 {
		t.Errorf("paths = %v, want A and B once each", plan.Paths)
	}
}

func TestLowerReservesCapacityAboveTheStaticSize(t *testing.T) {
	var bag diag.Bag
	const body = "<h1>a heading</h1>"
	result, _ := Compile(fstest.MapFS{"app/page.rill": file(body)}, &bag)
	if got := result.Manifest.Plans[0].Capacity; got <= uint32(len(body)) {
		t.Errorf("capacity = %d, want more than the %d static bytes", got, len(body))
	}
}

func TestManifestRoundTripsThroughTheCodec(t *testing.T) {
	var bag diag.Bag
	result, _ := Compile(fstest.MapFS{
		"app/layout.rill": file("<main>{% outlet %}</main>"),
		"app/page.rill":   file("<p>{{ Title }}</p>"),
	}, &bag)

	decoded, err := ir.Decode(ir.Encode(result.Manifest))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	route, _ := decoded.Lookup("/")
	out := runtime.NewBuffer(128)
	if err := runtime.Render(decoded.Chain(route), runtime.Map{"Title": runtime.String("x")}, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "<main><p>x</p></main>") {
		t.Errorf("render after a round trip = %q", out.String())
	}
}

func TestUnreadableTemplateIsReported(t *testing.T) {
	var bag diag.Bag
	if _, ok := ReadTemplate(fstest.MapFS{}, "app/missing.rill", &bag); ok {
		t.Error("ReadTemplate must fail for a missing file")
	}
	if bag.Len() == 0 {
		t.Error("a missing template must produce a diagnostic")
	}
}

func TestEveryDiscoveredRouteHasAName(t *testing.T) {
	routes, _ := discover(t, fstest.MapFS{
		"app/page.rill":        file("a"),
		"app/a/page.rill":      file("b"),
		"app/a/[id]/page.rill": file("c"),
		"app/api/x/route.go":   file("package route"),
	})
	for _, route := range routes {
		if route.Name == "" {
			t.Errorf("route %q has no name", route.Pattern)
		}
	}
	if slices.Contains(patterns(routes), "") {
		t.Error("a route ended up with an empty pattern")
	}
}
