package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func settings(t *testing.T, text string) config.Config {
	t.Helper()
	parsed, err := config.Parse(text)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return parsed
}

func withFallbacks(m *ir.Manifest) *ir.Manifest {
	m.Plans = append(m.Plans,
		ir.Plan{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 9}}, Blob: []byte("not found"), Capacity: 16},
		ir.Plan{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 5}}, Blob: []byte("boom!"), Capacity: 16},
		ir.Plan{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 9}}, Blob: []byte("deep gone"), Capacity: 16},
	)
	m.Fallbacks = []ir.Fallback{
		{Prefix: "/", Name: "not-found", Kind: ir.FallbackNotFound, Plan: 3, LayoutChain: []uint32{0}},
		{Prefix: "/", Name: "error", Kind: ir.FallbackError, Plan: 4},
		{Prefix: "/listings", Name: "listings.not-found", Kind: ir.FallbackNotFound, Plan: 5},
	}
	return m
}

func get(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestUnknownHostIsMisdirected(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"hosts\": [{\"pattern\": \"example.com\", \"locale\": \"en\"}]}"),
	})
	request := httptest.NewRequest(http.MethodGet, "http://evil.test/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMisdirectedRequest {
		t.Errorf("status = %d, want 421", recorder.Code)
	}
}

func TestKnownHostPasses(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"hosts\": [{\"pattern\": \"example.com\", \"locale\": \"en\"}]}"),
	})
	request := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestRedirectsRunBeforeRouting(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"redirects\": [{\"from\": \"/old\", \"to\": \"/\", \"status\": 302}]}"),
	})
	recorder := get(t, app.Handler(), "/old")
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/" {
		t.Errorf("status = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestRewritesChangeThePathWithoutARedirect(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"rewrites\": [{\"from\": \"/start\", \"to\": \"/\"}]}"),
	})
	recorder := get(t, app.Handler(), "/start")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "<home") {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestDefaultLocalePrefixRedirects(t *testing.T) {
	app := New(Options{Manifest: manifest(), Config: settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")})
	cases := map[string]string{"/en": "/", "/en/listings/1": "/listings/1"}
	for target, want := range cases {
		recorder := get(t, app.Handler(), target)
		if recorder.Code != http.StatusMovedPermanently || recorder.Header().Get("Location") != want {
			t.Errorf("%s: status = %d, location = %q, want %q", target, recorder.Code, recorder.Header().Get("Location"), want)
		}
	}
}

func TestPrefixDefaultKeepsTheDefaultLocalePrefix(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"], \"prefixDefault\": true}}"),
	})
	recorder := get(t, app.Handler(), "/en")
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestLocalePrefixIsStrippedBeforeRouting(t *testing.T) {
	app := New(Options{Manifest: manifest(), Config: settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")})
	for _, target := range []string{"/pl", "/pl/docs/guide"} {
		recorder := get(t, app.Handler(), target)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: status = %d", target, recorder.Code)
		}
		if got := recorder.Header().Get(LocaleHeader); got != "pl" {
			t.Errorf("%s: locale = %q", target, got)
		}
	}
}

func TestReservedPrefixesKeepTheirPath(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
		API: map[string]http.Handler{"/api/health": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(r.URL.Path))
		})},
	})
	recorder := get(t, app.Handler(), "/api/health")
	if recorder.Body.String() != "/api/health" {
		t.Errorf("path = %q", recorder.Body.String())
	}
}

func TestSubdomainModeTakesTheLocaleFromTheHost(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config: settings(t, `{
  "i18n": {"mode": "subdomain", "locales": ["en", "pl"]},
  "hosts": [
    {"pattern": "example.com", "locale": "en", "default": true},
    {"pattern": "pl.example.com", "locale": "pl"}
  ]
}`),
	})
	request := httptest.NewRequest(http.MethodGet, "http://pl.example.com/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if got := recorder.Header().Get(LocaleHeader); got != "pl" {
		t.Errorf("locale = %q", got)
	}
}

func TestSingleModeIgnoresPathPrefixes(t *testing.T) {
	app := New(Options{Manifest: manifest(), Config: settings(t, "{\"i18n\": {\"mode\": \"single\", \"locales\": [\"en\", \"pl\"]}}")})
	recorder := get(t, app.Handler(), "/pl")
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}

func TestLocaleOfReadsTheRequest(t *testing.T) {
	var seen string
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
		API: map[string]http.Handler{"/api/echo": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = LocaleOf(r)
		})},
	})
	get(t, app.Handler(), "/api/echo")
	if seen != "en" {
		t.Errorf("locale = %q", seen)
	}
	if got := LocaleOf(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Errorf("bare request locale = %q", got)
	}
}

func TestNotFoundRendersTheFallbackPage(t *testing.T) {
	app := New(Options{Manifest: withFallbacks(manifest())})
	recorder := get(t, app.Handler(), "/missing")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "not found") || !strings.Contains(body, "<main>") {
		t.Errorf("body = %q, want the layout around the fallback", body)
	}
}

func TestTheNearestFallbackWins(t *testing.T) {
	app := New(Options{Manifest: withFallbacks(manifest())})
	recorder := get(t, app.Handler(), "/listings/1/extra")
	if body := recorder.Body.String(); !strings.Contains(body, "deep gone") {
		t.Errorf("body = %q", body)
	}
}

func TestNotFoundFallsBackToPlainTextWithoutAPage(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	recorder := get(t, app.Handler(), "/missing")
	if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), "<main>") {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestRenderFailureRendersTheErrorPage(t *testing.T) {
	app := New(Options{
		Manifest: withFallbacks(manifest()),
		Props: map[string]PropsProvider{
			"listings.id": func(*http.Request, Params) (runtime.Accessible, error) { return nil, errors.New("nope") },
		},
	})
	recorder := get(t, app.Handler(), "/listings/1")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "boom!") {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestFallbackPropsFailureStillRendersThePage(t *testing.T) {
	app := New(Options{
		Manifest: withFallbacks(manifest()),
		Props: map[string]PropsProvider{
			"not-found": func(*http.Request, Params) (runtime.Accessible, error) { return nil, errors.New("nope") },
		},
	})
	recorder := get(t, app.Handler(), "/missing")
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "not found") {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestFallbackRenderFailureDegradesToPlainText(t *testing.T) {
	broken := withFallbacks(manifest())
	broken.Plans[3] = ir.Plan{Ops: []ir.Op{{Kind: ir.OpText, A: 9}}, Capacity: 8}
	app := New(Options{Manifest: broken})
	recorder := get(t, app.Handler(), "/missing")
	if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), "<main>") {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestMiddlewareRunsInOrder(t *testing.T) {
	var order []string
	tag := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	app := New(Options{Manifest: manifest(), Middleware: []Middleware{tag("outer"), tag("inner")}})
	get(t, app.Handler(), "/")
	if strings.Join(order, ",") != "outer,inner" {
		t.Errorf("order = %v", order)
	}
}

func TestApplyRule(t *testing.T) {
	cases := []struct {
		from, to, path, want string
		ok                   bool
	}{
		{"/a", "/b", "/a", "/b", true},
		{"/a", "/b", "/c", "", false},
		{"/old/*", "/new/*", "/old/deep/page", "/new/deep/page", true},
		{"/old/*", "/new/*", "/old", "/new", true},
		{"/old/*", "/new", "/old/deep", "/new", true},
		{"/old/*", "/new/*", "/older", "", false},
		{"/*", "/*", "/", "/", true},
	}
	for _, c := range cases {
		got, ok := applyRule(c.from, c.to, c.path)
		if got != c.want || ok != c.ok {
			t.Errorf("applyRule(%q, %q, %q) = %q, %v", c.from, c.to, c.path, got, ok)
		}
	}
}

func TestFallbackLookupIgnoresOtherKinds(t *testing.T) {
	m := withFallbacks(manifest())
	if _, ok := m.Fallback(ir.FallbackError, "/listings/x"); !ok {
		t.Error("the root error page answers every path")
	}
	empty := &ir.Manifest{Version: ir.Version}
	if _, ok := empty.Fallback(ir.FallbackNotFound, "/"); ok {
		t.Error("a manifest without fallbacks answers nothing")
	}
}

func TestParamsFromReadsThePathValues(t *testing.T) {
	mux := http.NewServeMux()
	var seen Params
	mux.HandleFunc("/api/listings/{id}", func(w http.ResponseWriter, r *http.Request) {
		seen = ParamsFrom(r, []string{"id"})
	})
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/listings/42", nil))
	if seen["id"] != "42" {
		t.Errorf("params = %v", seen)
	}
	if got := ParamsFrom(httptest.NewRequest(http.MethodGet, "/", nil), nil); len(got) != 0 {
		t.Errorf("params = %v, want empty", got)
	}
}

func TestAHostWithoutALocaleLeavesTheRequestUnmarked(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"i18n\": {\"mode\": \"subdomain\"}, \"hosts\": [{\"pattern\": \"example.com\", \"locale\": \"\"}]}"),
	})
	request := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if got := recorder.Header().Get(LocaleHeader); got != "" {
		t.Errorf("locale = %q, want none", got)
	}
}

func TestAssetsAreMountedUnderTheirPrefix(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(r.URL.Path))
		}),
		Config: settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
	})
	recorder := get(t, app.Handler(), "/assets/app.abc.css")
	if recorder.Body.String() != "/assets/app.abc.css" {
		t.Errorf("path = %q, want the prefix left untouched", recorder.Body.String())
	}
}

func TestEarlyHintsCarryTheAssetLink(t *testing.T) {
	link := `</assets/app.abc.css>; rel=preload; as=style`
	app := New(Options{Manifest: manifest(), AssetLink: link})
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	var hinted string
	request, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			if code == http.StatusEarlyHints {
				hinted = header.Get("Link")
			}
			return nil
		},
	}))
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if hinted != link {
		t.Errorf("early hints link = %q", hinted)
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d", response.StatusCode)
	}
	if got := response.Header.Get(AssetsHeader); got != link {
		t.Errorf("assets header = %q", got)
	}
	if got := response.Header.Get("Link"); got != "" {
		t.Errorf("link = %q, want it dropped from the final response", got)
	}
}

func TestNoEarlyHintsWithoutAssets(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	recorder := get(t, app.Handler(), "/")
	if got := recorder.Header().Get(AssetsHeader); got != "" {
		t.Errorf("assets header = %q, want none", got)
	}
}

func TestSecurityHeadersAreSetOnEveryResponse(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	for _, target := range []string{"/", "/missing"} {
		recorder := get(t, app.Handler(), target)
		if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: nosniff = %q", target, got)
		}
		if got := recorder.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
			t.Errorf("%s: referrer policy = %q", target, got)
		}
	}
}

func preloadManifest(strategy string) *ir.Manifest {
	layout := ir.Plan{
		Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 6}, {Kind: ir.OpPreload}, {Kind: ir.OpStatic, A: 6, B: 7}, {Kind: ir.OpOutlet}},
		Blob:     []byte("<head></head>"),
		Capacity: 64,
	}
	page := ir.Plan{
		Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 5}},
		Blob:     []byte("<home"),
		Islands:  []ir.IslandUse{{Name: "Stars", Strategy: strategy}},
		Capacity: 16,
	}
	return &ir.Manifest{
		Plans:  []ir.Plan{layout, page},
		Routes: []ir.Route{{Pattern: "/", Name: "index", Plan: 1, LayoutChain: []uint32{0}}},
	}
}

func TestAnEagerIslandPreloadsItsChunksForTheRoute(t *testing.T) {
	app := New(Options{
		Manifest:  preloadManifest("idle"),
		AssetLink: `</assets/app.css>; rel=preload; as=style`,
		Preloads:  map[string][]string{"Stars": {"island.REACT.js", "island.STARS.js"}},
	})
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	var hinted string
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			if code == http.StatusEarlyHints {
				hinted = header.Get("Link")
			}
			return nil
		},
	}))
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)

	want := `<link rel="modulepreload" href="/assets/island.REACT.js" fetchpriority="low"><link rel="modulepreload" href="/assets/island.STARS.js" fetchpriority="low">`
	if !strings.Contains(string(body), "<head>"+want+"</head>") {
		t.Errorf("body = %q, want the route's chunks preloaded in the head", body)
	}
	if !strings.Contains(hinted, "</assets/island.REACT.js>; rel=modulepreload") || !strings.Contains(hinted, "app.css") {
		t.Errorf("early hints = %q, want the route chunks after the shared assets", hinted)
	}
}

func TestAVisibleIslandPreloadsNothing(t *testing.T) {
	app := New(Options{
		Manifest: preloadManifest("visible"),
		Preloads: map[string][]string{"Stars": {"island.REACT.js"}},
	})
	recorder := get(t, app.Handler(), "/")
	if strings.Contains(recorder.Body.String(), "modulepreload") {
		t.Errorf("body = %q, want nothing preloaded for an island that may never activate", recorder.Body.String())
	}
	if len(preloadsFor(nil, nil)) != 0 || len(preloadsFor(preloadManifest("load"), nil)) != 0 {
		t.Error("no manifest or no chunk map means no preloads")
	}
}

func TestAMixedCaseLocalePrefixReachesItsLocale(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"], "prefixDefault": true},
			"routing": {"normalize": {"case": "lower"}}
		}`),
	})
	handler := app.Handler()
	cases := map[string]string{
		"/PL/docs/a": "/pl/docs/a",
		"/PL":        "/pl",
		"/EN/docs/a": "/en/docs/a",
		"/Pl/docs/A": "/pl/docs/a",
	}
	for target, want := range cases {
		response := get(t, handler, target)
		if response.Code != http.StatusMovedPermanently {
			t.Errorf("%s answered %d", target, response.Code)
			continue
		}
		if got := response.Header().Get("Location"); got != want {
			t.Errorf("%s went to %q, want %q", target, got, want)
		}
	}
}

func TestAMixedCasePrefixIsFoundWithoutNormalisation(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config:   settings(t, `{"i18n": {"locales": ["en", "pl"], "prefixDefault": true}}`),
	})
	response := get(t, app.Handler(), "/PL/docs/a")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/pl/docs/a" {
		t.Errorf("location = %q", got)
	}
}

func TestAMixedCasePrefixOfTheDefaultLocaleLosesThePrefix(t *testing.T) {
	app := New(Options{Manifest: metaChain(), Config: settings(t, `{"i18n": {"locales": ["en", "pl"]}}`)})
	response := get(t, app.Handler(), "/EN/docs/a")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/docs/a" {
		t.Errorf("location = %q", got)
	}
}

func TestALocaleTagKeepsTheCaseItWasConfiguredWith(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config:   settings(t, `{"i18n": {"locales": ["en-US", "pl"], "defaultLocale": "en-US", "prefixDefault": true}}`),
	})
	handler := app.Handler()
	if code := get(t, handler, "/en-US/docs/a").Code; code != http.StatusOK {
		t.Errorf("the configured spelling answered %d", code)
	}
	response := get(t, handler, "/en-us/docs/a")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/en-US/docs/a" {
		t.Errorf("location = %q", got)
	}
}

func TestALocalePrefixIsCanonicalisedInOneHop(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"], "prefixDefault": true},
			"routing": {"aliases": {"pl": {"docs": "dokumenty"}}, "normalize": {"case": "lower", "trailingSlash": "strip"}}
		}`),
	})
	handler := app.Handler()
	for _, target := range []string{"/PL/Docs/A/", "/PL/docs/a", "/Pl", "/EN/docs/A"} {
		response := get(t, handler, target)
		if response.Code != http.StatusMovedPermanently {
			t.Errorf("%s answered %d", target, response.Code)
			continue
		}
		next := response.Header().Get("Location")
		if code := get(t, handler, next).Code; code != http.StatusOK {
			t.Errorf("%s went to %q, which answered %d", target, next, code)
		}
	}
}

func TestARedirectKeepsTheQuery(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"], "prefixDefault": true},
			"routing": {"normalize": {"case": "lower"}},
			"redirects": [{"from": "/old", "to": "/", "status": 302}, {"from": "/kept", "to": "/?x=1", "status": 302}]
		}`),
	})
	handler := app.Handler()
	cases := map[string]string{
		"/old?page=2":       "/?page=2",
		"/kept?page=2":      "/?x=1",
		"/PL/docs/a?page=2": "/pl/docs/a?page=2",
		"/pl/docs/a":        "",
	}
	for target, want := range cases {
		response := get(t, handler, target)
		if want == "" {
			if response.Code != http.StatusOK {
				t.Errorf("%s answered %d", target, response.Code)
			}
			continue
		}
		if got := response.Header().Get("Location"); got != want {
			t.Errorf("%s went to %q, want %q", target, got, want)
		}
	}
}

func fromHost(t *testing.T, handler http.Handler, host, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = host
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestARedirectCanBeScopedToAHost(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"], "prefixDefault": true},
			"redirects": [{"from": "/*", "to": "https://example.com/pl", "host": "pl.example.com"}]
		}`),
	})
	handler := app.Handler()
	cases := map[string]string{
		"pl.example.com":      "https://example.com/pl",
		"PL.Example.com":      "https://example.com/pl",
		"pl.example.com:8443": "https://example.com/pl",
		"example.com":         "",
	}
	for host, want := range cases {
		response := fromHost(t, handler, host, "/")
		if want == "" {
			if response.Code != http.StatusOK {
				t.Errorf("%s answered %d, want the page", host, response.Code)
			}
			continue
		}
		if response.Code != http.StatusMovedPermanently {
			t.Errorf("%s answered %d", host, response.Code)
			continue
		}
		if got := response.Header().Get("Location"); got != want {
			t.Errorf("%s went to %q, want %q", host, got, want)
		}
	}
}

func TestAHostScopedRedirectAnswersForAnUnlistedHost(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"hosts": [{"pattern": "example.com", "locale": "en"}],
			"redirects": [{"from": "/*", "to": "https://example.com/", "host": "old.example.com"}]
		}`),
	})
	handler := app.Handler()
	response := fromHost(t, handler, "old.example.com", "/anything")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want the redirect rather than a refusal", response.Code)
	}
	if code := fromHost(t, handler, "evil.test", "/").Code; code != http.StatusMisdirectedRequest {
		t.Errorf("an unnamed host answered %d, want 421", code)
	}
}

func TestEntryMiddlewareRunsBeforeTheRedirects(t *testing.T) {
	consolidate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host == "pl.example.com" {
				http.Redirect(w, r, "https://example.com/pl", http.StatusMovedPermanently)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"], "prefixDefault": true},
			"redirects": [{"from": "/", "to": "/en", "status": 302}]
		}`),
		Entry: []Middleware{consolidate},
	})
	handler := app.Handler()
	response := fromHost(t, handler, "pl.example.com", "/")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "https://example.com/pl" {
		t.Errorf("location = %q", got)
	}
	other := fromHost(t, handler, "example.com", "/")
	if other.Code != http.StatusFound || other.Header().Get("Location") != "/en" {
		t.Errorf("status = %d, location = %q", other.Code, other.Header().Get("Location"))
	}
}

func TestEntryMiddlewareRunsInOrder(t *testing.T) {
	var order []string
	tag := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	app := New(Options{
		Manifest:   manifest(),
		Entry:      []Middleware{tag("first"), tag("second")},
		Middleware: []Middleware{tag("late")},
	})
	get(t, app.Handler(), "/")
	if strings.Join(order, ",") != "first,second,late" {
		t.Errorf("order = %v", order)
	}
}

func TestTheAskedPathSurvivesTheLocalePrefix(t *testing.T) {
	type sighting struct {
		path   string
		prefix string
		locale string
	}
	var seen sighting
	watch := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = sighting{path: AskedPath(r), prefix: AskedPrefix(r), locale: LocaleOf(r)}
			next.ServeHTTP(w, r)
		})
	}
	app := New(Options{
		Manifest:   metaChain(),
		Config:     settings(t, `{"i18n": {"locales": ["en", "pl"], "prefixDefault": true}}`),
		Middleware: []Middleware{watch},
	})
	handler := app.Handler()
	cases := map[string]sighting{
		"/":          {path: "/", prefix: "", locale: "en"},
		"/en":        {path: "/en", prefix: "/en", locale: "en"},
		"/pl/docs/a": {path: "/pl/docs/a", prefix: "/pl", locale: "pl"},
		"/docs/a":    {path: "/docs/a", prefix: "", locale: "en"},
	}
	for target, want := range cases {
		seen = sighting{}
		get(t, handler, target)
		if seen != want {
			t.Errorf("%s was seen as %+v, want %+v", target, seen, want)
		}
	}
}

func TestASubdomainRequestStillCarriesItsPath(t *testing.T) {
	var seen string
	watch := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = AskedPath(r)
			next.ServeHTTP(w, r)
		})
	}
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"mode": "subdomain", "locales": ["en", "pl"]},
			"hosts": [{"pattern": "example.com", "locale": "en", "default": true}, {"pattern": "pl.example.com", "locale": "pl"}]
		}`),
		Middleware: []Middleware{watch},
	})
	fromHost(t, app.Handler(), "pl.example.com", "/docs/a")
	if seen != "/docs/a" {
		t.Errorf("original path = %q", seen)
	}
}

func TestAShoutedPathThatIsNoLocaleIsOnlyNormalised(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"]},
			"routing": {"normalize": {"case": "lower"}}
		}`),
	})
	response := get(t, app.Handler(), "/Docs/A")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/docs/a" {
		t.Errorf("location = %q", got)
	}
}

func TestALocalePrefixIsAWholeSegment(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"], "prefixDefault": true},
			"routing": {"normalize": {"case": "lower"}}
		}`),
	})
	handler := app.Handler()
	for _, target := range []string{"/plx/docs", "/PLX/docs"} {
		response := get(t, handler, target)
		if got := response.Header().Get(LocaleHeader); got == "pl" {
			t.Errorf("%s was read as a locale prefix", target)
		}
	}
}

func TestARequestOutsideTheChainCarriesNoRouting(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/docs/a", nil)
	if got := AskedPath(request); got != "" {
		t.Errorf("asked path = %q", got)
	}
	if got := AskedPrefix(request); got != "" {
		t.Errorf("asked prefix = %q, want none on a request that never met the chain", got)
	}
	if got := LocaleOf(request); got != "" {
		t.Errorf("locale = %q", got)
	}
}
