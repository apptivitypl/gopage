package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/ir"
	"github.com/sonquer/rill/internal/runtime"
)

func manifest() *ir.Manifest {
	return &ir.Manifest{
		Version: ir.Version,
		Plans: []ir.Plan{
			{
				Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 6}, {Kind: ir.OpOutlet}, {Kind: ir.OpStatic, A: 6, B: 7}},
				Blob:     []byte("<main></main>"),
				Capacity: 32,
			},
			{
				Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 5}},
				Blob:     []byte("<home"),
				Capacity: 16,
			},
			{
				Ops:      []ir.Op{{Kind: ir.OpText, A: 0}},
				Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
				Paths:    [][]string{{"ID"}},
				Capacity: 16,
			},
		},
		Routes: []ir.Route{
			{Pattern: "/", Name: "index", Plan: 1, LayoutChain: []uint32{0}, Class: ir.ClassStatic},
			{Pattern: "/listings/[id]", Name: "listings.id", Plan: 2, Class: ir.ClassDynamic},
			{Pattern: "/docs/[...slug]", Name: "docs.slug", Plan: 1, Class: ir.ClassDynamic},
		},
	}
}

func TestRouterMatchesStaticPatterns(t *testing.T) {
	router := NewRouter(manifest().Routes)
	route, params, ok := router.Match("/")
	if !ok || route.Name != "index" || len(params) != 0 {
		t.Errorf("route = %+v, params = %v, ok = %v", route, params, ok)
	}
}

func TestRouterCapturesParams(t *testing.T) {
	router := NewRouter(manifest().Routes)
	route, params, ok := router.Match("/listings/42")
	if !ok || route.Name != "listings.id" {
		t.Fatalf("route = %+v, ok = %v", route, ok)
	}
	if !reflect.DeepEqual(params, Params{"id": "42"}) {
		t.Errorf("params = %v", params)
	}
}

func TestRouterCatchAllTakesTheRest(t *testing.T) {
	router := NewRouter(manifest().Routes)
	_, params, ok := router.Match("/docs/a/b/c")
	if !ok || params["slug"] != "a/b/c" {
		t.Errorf("params = %v, ok = %v", params, ok)
	}
}

func TestRouterRejectsAnEmptyCatchAll(t *testing.T) {
	router := NewRouter(manifest().Routes)
	if _, _, ok := router.Match("/docs"); ok {
		t.Error("a required catch-all must not match an empty tail")
	}
}

func TestOptionalCatchAllMatchesNothing(t *testing.T) {
	router := NewRouter([]ir.Route{{Pattern: "/docs/[[...slug]]", Name: "docs"}})
	_, params, ok := router.Match("/docs")
	if !ok {
		t.Fatal("an optional catch-all must match an empty tail")
	}
	if params["slug"] != "" {
		t.Errorf("slug = %q, want empty", params["slug"])
	}
}

func TestRouterRejectsUnknownPaths(t *testing.T) {
	router := NewRouter(manifest().Routes)
	for _, path := range []string{"/nope", "/listings", "/listings/1/2"} {
		if _, _, ok := router.Match(path); ok {
			t.Errorf("Match(%q) must not succeed", path)
		}
	}
}

func TestStaticPatternsWinOverDynamicOnes(t *testing.T) {
	router := NewRouter([]ir.Route{
		{Pattern: "/listings/[id]", Name: "dynamic"},
		{Pattern: "/listings/new", Name: "static"},
	})
	route, _, _ := router.Match("/listings/new")
	if route.Name != "static" {
		t.Errorf("route = %q, want the literal segment to win", route.Name)
	}
}

func TestParamsBeatCatchAll(t *testing.T) {
	router := NewRouter([]ir.Route{
		{Pattern: "/docs/[...rest]", Name: "catchall"},
		{Pattern: "/docs/[page]", Name: "param"},
	})
	route, _, _ := router.Match("/docs/intro")
	if route.Name != "param" {
		t.Errorf("route = %q, want the single-segment param to win", route.Name)
	}
}

func TestRoutesAreExposedInMatchOrder(t *testing.T) {
	router := NewRouter(manifest().Routes)
	if len(router.Routes()) != 3 {
		t.Errorf("routes = %d", len(router.Routes()))
	}
}

func serve(t *testing.T, app *App, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func TestServerRendersThroughTheLayout(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	res := serve(t, app, http.MethodGet, "/")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	if body := res.Body.String(); body != "<main><home</main>" {
		t.Errorf("body = %q", body)
	}
	if got := res.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type = %q", got)
	}
	if got := res.Header().Get("Content-Length"); got != "18" {
		t.Errorf("content length = %q", got)
	}
}

func TestHeadReturnsHeadersWithoutABody(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	res := serve(t, app, http.MethodHead, "/")
	if res.Code != http.StatusOK || res.Body.Len() != 0 {
		t.Errorf("status = %d, body = %d bytes", res.Code, res.Body.Len())
	}
	if res.Header().Get("Content-Length") != "18" {
		t.Error("HEAD must still report the length")
	}
}

func TestUnknownPathIs404(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	if res := serve(t, app, http.MethodGet, "/nope"); res.Code != http.StatusNotFound {
		t.Errorf("status = %d", res.Code)
	}
}

func TestNonGetMethodIsRejected(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	res := serve(t, app, http.MethodPost, "/")
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Header().Get("Allow"); got != "GET, HEAD" {
		t.Errorf("allow = %q", got)
	}
}

func TestPropsProviderFeedsTheTemplate(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"listings.id": func(_ *http.Request, params Params) (runtime.Accessible, error) {
				return runtime.Map{"ID": runtime.String(params["id"])}, nil
			},
		},
	})
	res := serve(t, app, http.MethodGet, "/listings/7")
	if res.Body.String() != "7" {
		t.Errorf("body = %q, want the param rendered", res.Body.String())
	}
}

func TestPropsErrorBecomesA500(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"listings.id": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, http.ErrNoLocation
			},
		},
	})
	if res := serve(t, app, http.MethodGet, "/listings/7"); res.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", res.Code)
	}
}

func TestMissingPropsBecomeA500RatherThanBrokenHTML(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	res := serve(t, app, http.MethodGet, "/listings/7")
	if res.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the render error surfaced", res.Code)
	}
}

func TestApiHandlersAreMountedBeforePages(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		API: map[string]http.Handler{
			"/api/health": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}),
		},
	})
	res := serve(t, app, http.MethodGet, "/api/health")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "ok") {
		t.Errorf("status = %d, body = %q", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content type = %q", got)
	}
}

func TestAnAppWithoutAManifestStillAnswers(t *testing.T) {
	app := New(Options{})
	if res := serve(t, app, http.MethodGet, "/"); res.Code != http.StatusNotFound {
		t.Errorf("status = %d", res.Code)
	}
	if len(app.Routes()) != 0 {
		t.Errorf("routes = %v", app.Routes())
	}
}

func TestRenderStaticProducesTheSameBytesAsTheServer(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	route, _ := app.manifest.Lookup("/")

	body, err := app.RenderStatic(route)
	if err != nil {
		t.Fatalf("RenderStatic: %v", err)
	}
	if string(body) != serve(t, app, http.MethodGet, "/").Body.String() {
		t.Errorf("static render = %q, differs from the served page", body)
	}
}

func TestRenderStaticReportsErrors(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	route, _ := app.manifest.Lookup("/listings/[id]")
	if _, err := app.RenderStatic(route); err == nil {
		t.Error("expected an error for a route with unresolved props")
	}
}

func TestMetaProviderFeedsTheMetaPath(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Meta: map[string]MetaProvider{
			"index": func(*http.Request, Params) (runtime.Meta, error) {
				return runtime.Meta{Title: "home"}, nil
			},
		},
	})
	route, _ := app.manifest.Lookup("/")
	props, err := app.propsFor(route, httptest.NewRequest(http.MethodGet, "/", nil), Params{})
	if err != nil {
		t.Fatalf("propsFor: %v", err)
	}
	value, ok := props.Get([]string{runtime.MetaRoot, "Title"})
	if !ok || value.Str != "home" {
		t.Errorf("meta title = %+v, %v", value, ok)
	}
}

func TestMetaProviderErrorBecomesA500(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Meta: map[string]MetaProvider{
			"index": func(*http.Request, Params) (runtime.Meta, error) {
				return runtime.Meta{}, http.ErrNoLocation
			},
		},
	})
	if res := serve(t, app, http.MethodGet, "/"); res.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", res.Code)
	}
}

func TestRoutesWithoutAMetaProviderStillRender(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	route, _ := app.manifest.Lookup("/")
	props, err := app.propsFor(route, httptest.NewRequest(http.MethodGet, "/", nil), Params{})
	if err != nil {
		t.Fatalf("propsFor: %v", err)
	}
	if value, ok := props.Get([]string{runtime.MetaRoot, "Title"}); !ok || value.Str != "" {
		t.Errorf("meta title = %+v, %v, want an empty value", value, ok)
	}
}
