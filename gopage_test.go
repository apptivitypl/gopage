package gopage

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/compile"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/ir"
)

func build(t *testing.T, files fstest.MapFS) []byte {
	t.Helper()
	var bag diag.Bag
	result, err := compile.Compile(files, &bag)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	return ir.Encode(result.Manifest)
}

func demo(t *testing.T) []byte {
	t.Helper()
	return build(t, fstest.MapFS{
		"app/layout.gopage":             {Data: []byte("<main>{% outlet %}</main>")},
		"app/page.gopage":               {Data: []byte("<h1>home</h1>")},
		"app/listings/[id]/page.gopage": {Data: []byte("<p>{{ ID }}</p>")},
	})
}

func TestNewRejectsAForeignManifest(t *testing.T) {
	if _, err := New(Options{Manifest: []byte("garbage")}); err == nil {
		t.Error("expected an error for a manifest this binary cannot read")
	}
}

func TestHandlerServesACompiledPage(t *testing.T) {
	app, err := New(Options{Manifest: demo(t)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := httptest.NewRecorder()
	app.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	if body := res.Body.String(); body != "<main><h1>home</h1></main>" {
		t.Errorf("body = %q", body)
	}
}

func TestPropsProviderIsWiredThrough(t *testing.T) {
	app, err := New(Options{
		Manifest: demo(t),
		Props: map[string]PropsProvider{
			"listings.id": func(_ *http.Request, params Params) (Accessible, error) {
				return Props{"ID": String(params["id"])}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := httptest.NewRecorder()
	app.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/listings/9", nil))
	if !strings.Contains(res.Body.String(), "<p>9</p>") {
		t.Errorf("body = %q", res.Body.String())
	}
}

func TestApiHandlerIsMounted(t *testing.T) {
	app, err := New(Options{
		Manifest: demo(t),
		API: map[string]http.Handler{
			"/api/health": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := httptest.NewRecorder()
	app.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if res.Body.String() != `{"status":"ok"}` {
		t.Errorf("body = %q", res.Body.String())
	}
}

func TestRoutesReportTheirClass(t *testing.T) {
	app, err := New(Options{Manifest: demo(t)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	byName := map[string]Route{}
	for _, route := range app.Routes() {
		byName[route.Name] = route
	}
	if !byName["index"].Static {
		t.Error("the root route is static")
	}
	if byName["listings.id"].Static {
		t.Error("a route with a param is dynamic")
	}
}

func TestRenderStaticByName(t *testing.T) {
	app, err := New(Options{Manifest: demo(t)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	body, err := app.RenderStatic("index")
	if err != nil {
		t.Fatalf("RenderStatic: %v", err)
	}
	if string(body) != "<main><h1>home</h1></main>" {
		t.Errorf("body = %q", body)
	}
	if _, err := app.RenderStatic("nope"); err == nil {
		t.Error("expected an error for an unknown route name")
	}
}

func TestValueHelpersAreExported(t *testing.T) {
	if String("a").Text() != "a" || Int(2).Text() != "2" || Bool(true).Text() != "true" {
		t.Error("the exported value helpers changed behaviour")
	}
}

func TestPublicFilesAreServedFromTheRoot(t *testing.T) {
	app, err := New(Options{
		Manifest: demo(t),
		Public: fstest.MapFS{
			"public/favicon.ico": {Data: []byte("icon")},
			"public/icon.svg":    {Data: []byte("<svg/>")},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler := app.Handler()
	for path, want := range map[string]string{"/favicon.ico": "icon", "/icon.svg": "<svg/>"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", path, recorder.Code)
			continue
		}
		if recorder.Body.String() != want {
			t.Errorf("%s = %q, want %q", path, recorder.Body.String(), want)
		}
	}
}

func TestPublicFilesStayOutOfThePreloadHeader(t *testing.T) {
	app, err := New(Options{
		Manifest: demo(t),
		Static:   fstest.MapFS{"styles/app.css": {Data: []byte("body{}")}},
		Public:   fstest.MapFS{"public/favicon.ico": {Data: []byte("icon")}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, link, public, err := staticAssets(
		fstest.MapFS{"styles/app.css": {Data: []byte("body{}")}},
		nil,
		fstest.MapFS{"public/favicon.ico": {Data: []byte("icon")}},
	)
	if err != nil {
		t.Fatalf("staticAssets: %v", err)
	}
	if !strings.Contains(link, "app.") {
		t.Errorf("link = %q, want the stylesheet preloaded", link)
	}
	if strings.Contains(link, "favicon") {
		t.Errorf("link = %q, want public files left out of the preload list", link)
	}
	if len(public) != 1 || public[0] != "/favicon.ico" {
		t.Errorf("public = %v, want the favicon routed at the root", public)
	}
	_ = app
}

func TestAnUnreadablePublicTreeIsReported(t *testing.T) {
	_, err := New(Options{Manifest: demo(t), Public: brokenPublic{}})
	if err == nil {
		t.Error("a public tree that cannot be read must fail New")
	}
}

type brokenPublic struct{}

func (brokenPublic) Open(name string) (fs.File, error) {
	if name == "public" {
		return fstest.MapFS{"favicon.ico": {Data: []byte("icon")}}.Open(".")
	}
	return nil, errors.New("unreadable")
}

func TestTheContextHelpersReachTheRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/listings/7?city=krakow", nil)
	ctx := NewCtx(request, Params{"id": "7"})
	if ctx.Param("id") != "7" || ctx.Query("city") != "krakow" {
		t.Errorf("params = %v, query = %q", ctx.Params(), ctx.Query("city"))
	}
	if ctx.Request() != request {
		t.Error("Request did not return the request it was built with")
	}
	if ctx.Cache() == nil {
		t.Error("Cache must always answer with a recorder")
	}
	if ctx.Locale() != "" {
		t.Errorf("locale = %q, want empty outside a localised request", ctx.Locale())
	}
	if got := ctx.T("some.key"); got != "some.key" {
		t.Errorf("T = %q, want the key when nothing translated it", got)
	}
	if got := ctx.Count("some.key", 3); got != "some.key" {
		t.Errorf("Count = %q, want the key", got)
	}
}

func TestACtxWithoutARequestIsHarmless(t *testing.T) {
	ctx := NewCtx(nil, nil)
	if ctx.Request() != nil || ctx.Locale() != "" || ctx.Query("x") != "" {
		t.Error("a context without a request must answer with zero values")
	}
	if ctx.Context() == nil {
		t.Error("Context must always answer")
	}
}

func TestTheRedirectHelpersBuildActions(t *testing.T) {
	if Redirect("home") == nil || RedirectTo("/x") == nil {
		t.Error("the redirect helpers must build an action")
	}
}

func TestInvalidateAndStatsReachTheCache(t *testing.T) {
	app, err := New(Options{Manifest: demo(t), CacheBytes: 1 << 20})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := app.Invalidate("listing:1"); got != 0 {
		t.Errorf("Invalidate = %d, want nothing to drop", got)
	}
	if stats := app.CacheStats(); stats.Bytes != 0 {
		t.Errorf("stats = %+v, want an empty cache", stats)
	}
}

func TestDecodeFormReadsAPostedForm(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Name=Ada"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var target struct {
		Name string
	}
	if _, err := DecodeForm(request, &target); err != nil {
		t.Fatalf("DecodeForm: %v", err)
	}
	if target.Name != "Ada" {
		t.Errorf("target = %+v", target)
	}
}

func TestSafeRedirectKeepsAPathAndDropsTheRest(t *testing.T) {
	cases := map[string]string{
		"/thanks":                 "/thanks",
		"/a?next=/b":              "/a?next=/b",
		"https://payments.test/x": "/",
		"https://evil.test/x":     "/",
		"//evil.com":              "/",
		"/\\evil.com":             "/",
		"javascript:alert(1)":     "/",
		"":                        "/",
	}
	for target, want := range cases {
		if got := SafeRedirect(target, "/"); got != want {
			t.Errorf("SafeRedirect(%q) = %q, want %q", target, got, want)
		}
	}
}

func TestSafeRedirectLeavesTheSiteOnlyForANamedHost(t *testing.T) {
	if got := SafeRedirect("https://payments.test/x", "/", "payments.test"); got != "https://payments.test/x" {
		t.Errorf("SafeRedirect = %q, want the named host allowed", got)
	}
	if got := SafeRedirect("https://evil.test/x", "/", "payments.test"); got != "/" {
		t.Errorf("SafeRedirect = %q, want a host nobody named refused", got)
	}
	if got := SafeRedirect("https://PAYMENTS.test/x", "/", "payments.test"); got != "https://PAYMENTS.test/x" {
		t.Errorf("SafeRedirect = %q, want the host matched without case", got)
	}
	if got := SafeRedirect("javascript:alert(1)", "/", "payments.test"); got != "/" {
		t.Errorf("SafeRedirect = %q, want the scheme still checked", got)
	}
}

func TestTheSidecarDecidesTheEarlyHints(t *testing.T) {
	bundles := fstest.MapFS{
		"bundles/gopage.client.ABC.js": &fstest.MapFile{Data: []byte("export {};")},
		"bundles/island.R.js":          &fstest.MapFile{Data: []byte("export {};")},
		"bundles/gopage.preload": &fstest.MapFile{Data: []byte(
			"link </assets/gopage.client.ABC.js>; rel=modulepreload\nlink </fonts/mono.woff2>; rel=preload; as=font; crossorigin\nisland Stars island.R.js\n")},
	}
	app, err := New(Options{Manifest: demo(t), Config: []byte("{\"app\": {\"name\": \"demo\"}}"), Bundles: bundles})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := recorder.Header().Get("GOPAGE-Assets"); !strings.Contains(got, "mono.woff2") || strings.Contains(got, "island.R.js") {
		t.Errorf("assets = %q, want the sidecar's list with the lazy island chunk kept out", got)
	}
}
