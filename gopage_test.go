package gopage

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/compile"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/reply"
	"github.com/apptivitypl/gopage/internal/server"
	"github.com/apptivitypl/gopage/internal/vocab"
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

func ctxWithRecorders(t *testing.T) (*Ctx, *reply.Recorder, *cache.Recorder) {
	t.Helper()
	answer, policy := reply.NewRecorder(), cache.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	ctx := reply.WithRecorder(cache.WithRecorder(request.Context(), policy), answer)
	return NewCtx(request.WithContext(ctx), Params{}), answer, policy
}

func TestTheResponseSurfaceReachesTheRecorder(t *testing.T) {
	ctx, answer, _ := ctxWithRecorders(t)
	ctx.Status(http.StatusGone)
	ctx.Header().Set("X-Robots-Tag", "noindex")
	ctx.Vary("Accept-Language")
	if answer.Code() != http.StatusGone {
		t.Errorf("status = %d", answer.Code())
	}
	headers := answer.Headers()
	if headers.Get("X-Robots-Tag") != "noindex" || !strings.Contains(headers.Get(reply.VaryHeader), "Accept-Language") {
		t.Errorf("headers = %v", headers)
	}
}

func TestReadingAPresentCookieMakesTheResponsePersonal(t *testing.T) {
	ctx, answer, policy := ctxWithRecorders(t)
	held, ok := ctx.Cookie("session")
	if !ok || held.Value != "abc" {
		t.Fatalf("cookie = %+v, ok = %v", held, ok)
	}
	if policy.Shared() {
		t.Error("a request carrying the cookie is personal")
	}
	if !strings.Contains(answer.Headers().Get(reply.VaryHeader), reply.CookieVary) {
		t.Errorf("vary = %v", answer.Headers())
	}
}

func TestReadingAnAbsentCookieLeavesTheResponseShared(t *testing.T) {
	ctx, answer, policy := ctxWithRecorders(t)
	if _, ok := ctx.Cookie("cart"); ok {
		t.Fatal("the cookie is not there")
	}
	if !policy.Shared() || answer.Headers() != nil {
		t.Error("an absent cookie is no dependency")
	}
	if _, ok := NewCtx(nil, Params{}).Cookie("session"); ok {
		t.Error("a context without a request carries no cookies")
	}
}

func TestSettingACookieMakesTheResponsePersonal(t *testing.T) {
	ctx, answer, policy := ctxWithRecorders(t)
	ctx.SetCookie(&http.Cookie{Name: "theme", Value: "dark"})
	if policy.Shared() {
		t.Error("a response that sets a cookie is personal")
	}
	response := httptest.NewRecorder()
	answer.Deliver(response, httptest.NewRequest(http.MethodGet, "/", nil), false)
	if got := response.Header().Get("Set-Cookie"); !strings.Contains(got, "theme=dark") {
		t.Errorf("set-cookie = %q", got)
	}
}

func TestRequestValuesTravelByType(t *testing.T) {
	type user struct{ Name string }
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := NewCtx(request.WithContext(WithValue(request.Context(), user{Name: "ada"})), Params{})
	held, ok := ValueOf[user](ctx)
	if !ok || held.Name != "ada" {
		t.Errorf("value = %+v, ok = %v", held, ok)
	}
	if _, ok := ValueOf[int](ctx); ok {
		t.Error("a type nobody stored is missing")
	}
}

func TestPathsAreBuiltForTheCurrentLocale(t *testing.T) {
	settings, err := config.Parse(`{
		"i18n": {"locales": ["en", "pl"]},
		"routing": {"aliases": {"pl": {"listings": "oferty"}}}
	}`)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := NewCtx(request.WithContext(vocab.With(request.Context(), vocab.New(settings))), Params{})

	got, err := PathFor(ctx, "pl", "/listings/[id]", Params{"id": "7"})
	if err != nil || got != "/pl/oferty/7" {
		t.Errorf("path = %q, err = %v", got, err)
	}
	if got, err := PathFor(ctx, "en", "/listings/[id]", Params{"id": "7"}); err != nil || got != "/listings/7" {
		t.Errorf("path = %q, err = %v", got, err)
	}
	if _, err := Path(ctx, "/listings/[id]", nil); err == nil {
		t.Error("a missing parameter is an error")
	}
}

func TestARedirectErrorNamesItsTarget(t *testing.T) {
	err := RedirectError(http.StatusMovedPermanently, "/jobs")
	if err == nil || !strings.Contains(err.Error(), "/jobs") {
		t.Errorf("error = %v", err)
	}
}

func TestLocalsAreReadByType(t *testing.T) {
	type deps struct{ Name string }
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := NewCtx(request.WithContext(server.WithLocals(request.Context(), deps{Name: "es"})), Params{})
	held, ok := LocalsOf[deps](ctx)
	if !ok || held.Name != "es" {
		t.Errorf("locals = %+v, ok = %v", held, ok)
	}
	if _, ok := LocalsOf[int](ctx); ok {
		t.Error("another type is missing")
	}
}

func TestSitemapEntriesAreExported(t *testing.T) {
	var seen []SitemapEntry
	for entry, err := range SitemapOf([]SitemapEntry{
		{Path: "/a", Alternates: []SitemapAlternate{{Lang: "pl", Href: "/pl/a"}}},
	}) {
		if err != nil {
			t.Fatalf("sequence: %v", err)
		}
		seen = append(seen, entry)
	}
	if len(seen) != 1 || seen[0].Path != "/a" || seen[0].Alternates[0].Href != "/pl/a" {
		t.Errorf("entries = %+v", seen)
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

func TestRepeatedQueryValuesAreReadable(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/jobs?market=PL&market=DE&q=go", nil)
	ctx := NewCtx(request, Params{})
	if got := ctx.QueryAll("market"); len(got) != 2 || got[0] != "PL" || got[1] != "DE" {
		t.Errorf("market = %v", got)
	}
	if got := ctx.Query("market"); got != "PL" {
		t.Errorf("first market = %q", got)
	}
	if got := ctx.QueryAll("missing"); got != nil {
		t.Errorf("missing = %v", got)
	}
	if got := NewCtx(nil, Params{}).QueryAll("market"); got != nil {
		t.Errorf("without a request = %v", got)
	}
}

func TestARouteRendersForATest(t *testing.T) {
	app, err := New(Options{Manifest: demo(t)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	body, err := app.Render(context.Background(), "index", Params{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(body), "<h1>home</h1>") {
		t.Errorf("body = %q", body)
	}
	if _, err := app.Render(context.Background(), "nope", Params{}); err == nil {
		t.Error("an unknown route was rendered")
	}
}

func TestATimeSequenceIsExported(t *testing.T) {
	moments := Times{time.Unix(0, 0).UTC(), time.Unix(3600, 0).UTC()}
	if moments.Len() != 2 {
		t.Errorf("len = %d", moments.Len())
	}
	if got := moments.At(1).Text(); got != "1970-01-01T01:00:00Z" {
		t.Errorf("second = %q", got)
	}
	if moments.At(5).Text() != "" || moments.At(-1).Text() != "" {
		t.Error("an index outside the slice is nil")
	}
}

func TestAnOpenGraphCardIsRendered(t *testing.T) {
	data, err := OpenGraph(OpenGraphCard{Title: "Praca w Warszawie", Subtitle: "12 922 oferty"})
	if err != nil {
		t.Fatalf("OpenGraph: %v", err)
	}
	if len(data) < 1000 || string(data[1:4]) != "PNG" {
		t.Errorf("card = %d bytes, header = %q", len(data), data[:8])
	}
}
