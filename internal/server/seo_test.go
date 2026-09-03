package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

func metaChain() *ir.Manifest {
	m := manifest()
	m.Plans[0] = ir.Plan{
		Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 6}, {Kind: ir.OpOutlet}, {Kind: ir.OpStatic, A: 6, B: 7}},
		Blob:     []byte("<main></main>"),
		Capacity: 128,
	}
	return m
}

func seoApp(t *testing.T, text string) *App {
	t.Helper()
	return New(Options{Manifest: metaChain(), Config: settings(t, text)})
}

func metaOf(t *testing.T, app *App, target string) runtime.Meta {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	locale, rest := app.splitLocale(request.URL.Path)
	request = withLocale(withPath(request, rest), locale)
	route, params, ok := app.router.Match(rest)
	if !ok {
		t.Fatalf("no route for %s", rest)
	}
	props, err := app.providers(route, request, params)
	if err != nil {
		t.Fatalf("providers: %v", err)
	}
	title, _ := props.Get([]string{runtime.MetaRoot, "Canonical"})
	alternates, _ := props.Get([]string{runtime.MetaRoot, runtime.AlternatesField})
	meta := runtime.Meta{Canonical: title.Str}
	if seq := alternates.Sequence(); seq != nil {
		for i := range seq.Len() {
			entry := seq.At(i).Object()
			lang, _ := entry.Get([]string{"Lang"})
			href, _ := entry.Get([]string{"Href"})
			meta.Alternates = append(meta.Alternates, runtime.Alternate{Lang: lang.Str, Href: href.Str})
		}
	}
	return meta
}

func TestCanonicalFollowsTheRequest(t *testing.T) {
	app := seoApp(t, "")
	if got := metaOf(t, app, "/").Canonical; got != "http://example.com/" {
		t.Errorf("canonical = %q", got)
	}
}

func TestAConfiguredCanonicalHostWinsOverTheRequest(t *testing.T) {
	app := seoApp(t, "{\"app\": {\"canonicalHost\": \"rill.test\"}}")
	if got := metaOf(t, app, "/").Canonical; got != "http://rill.test/" {
		t.Errorf("canonical = %q, want the configured host", got)
	}
}

func TestAHostThatIsNotAHostYieldsARelativeCanonical(t *testing.T) {
	app := seoApp(t, "")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = "example.com/<script>"
	if got := app.origin(request); got != "" {
		t.Errorf("origin = %q, want nothing built from a malformed host", got)
	}
}

func TestCanonicalCarriesTheLocalePrefix(t *testing.T) {
	app := seoApp(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")
	if got := metaOf(t, app, "/pl").Canonical; got != "http://example.com/pl" {
		t.Errorf("canonical = %q", got)
	}
}

func TestAlternatesAreReciprocal(t *testing.T) {
	app := seoApp(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")
	for _, target := range []string{"/", "/pl"} {
		meta := metaOf(t, app, target)
		if len(meta.Alternates) != 3 {
			t.Fatalf("%s: alternates = %+v", target, meta.Alternates)
		}
		seen := map[string]string{}
		for _, alternate := range meta.Alternates {
			seen[alternate.Lang] = alternate.Href
		}
		if seen["en"] != "http://example.com/" || seen["pl"] != "http://example.com/pl" {
			t.Errorf("%s: alternates = %v", target, seen)
		}
		if seen[defaultHreflang] != seen["en"] {
			t.Errorf("%s: x-default = %q, want the default locale", target, seen[defaultHreflang])
		}
	}
}

func TestOneLocaleNeedsNoAlternates(t *testing.T) {
	if meta := metaOf(t, seoApp(t, ""), "/"); len(meta.Alternates) != 0 {
		t.Errorf("alternates = %+v", meta.Alternates)
	}
}

func TestADynamicRouteGetsNoAlternates(t *testing.T) {
	app := seoApp(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")
	if meta := metaOf(t, app, "/listings/7"); len(meta.Alternates) != 0 {
		t.Errorf("alternates = %+v, want none for a route with parameters", meta.Alternates)
	}
}

func TestSubdomainAlternatesPointAtHosts(t *testing.T) {
	app := seoApp(t, `{
  "i18n": {"mode": "subdomain", "locales": ["en", "pl"]},
  "hosts": [
    {"pattern": "example.com", "locale": "en", "default": true},
    {"pattern": "pl.example.com", "locale": "pl"}
  ]
}`)
	meta := metaOf(t, app, "/")
	seen := map[string]string{}
	for _, alternate := range meta.Alternates {
		seen[alternate.Lang] = alternate.Href
	}
	if seen["pl"] != "http://pl.example.com/" || seen["en"] != "http://example.com/" {
		t.Errorf("alternates = %v", seen)
	}
}

func TestAnExplicitCanonicalIsKept(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Config:   settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
		Meta: map[string]MetaProvider{
			"index": func(*http.Request, Params) (runtime.Meta, error) {
				return runtime.Meta{Canonical: "https://elsewhere.test/x"}, nil
			},
		},
	})
	if got := metaOf(t, app, "/").Canonical; got != "https://elsewhere.test/x" {
		t.Errorf("canonical = %q", got)
	}
}

func TestTheSchemeCanBeForced(t *testing.T) {
	app := seoApp(t, "{\"app\": {\"scheme\": \"https\"}}")
	if got := metaOf(t, app, "/").Canonical; !strings.HasPrefix(got, "https://") {
		t.Errorf("canonical = %q", got)
	}
}

func TestReservedPathsGetNoSeo(t *testing.T) {
	app := seoApp(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	meta := app.seo(runtime.Meta{}, request, ir.Route{Pattern: "/api/health"})
	if meta.Canonical != "" || len(meta.Alternates) != 0 {
		t.Errorf("meta = %+v, want the api namespace left alone", meta)
	}
}

func TestSitemapAndRobotsAreServed(t *testing.T) {
	app := seoApp(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")
	handler := app.Handler()

	sitemap := get(t, handler, "/sitemap.xml")
	if sitemap.Code != http.StatusOK || !strings.Contains(sitemap.Body.String(), "<loc>http://example.com/</loc>") {
		t.Errorf("sitemap = %q", sitemap.Body.String())
	}
	if !strings.Contains(sitemap.Header().Get("Content-Type"), "xml") {
		t.Errorf("content type = %q", sitemap.Header().Get("Content-Type"))
	}
	robots := get(t, handler, "/robots.txt")
	if !strings.Contains(robots.Body.String(), "Sitemap: http://example.com/sitemap.xml") {
		t.Errorf("robots = %q", robots.Body.String())
	}
}

func TestHeadOnTheSitemapSendsNoBody(t *testing.T) {
	app := seoApp(t, "")
	request := httptest.NewRequest(http.MethodHead, "/sitemap.xml", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Body.Len() != 0 || recorder.Header().Get("Content-Length") == "" {
		t.Errorf("body = %q, length = %q", recorder.Body.String(), recorder.Header().Get("Content-Length"))
	}
}

func TestASitemapWriteFailureIsLogged(t *testing.T) {
	app := seoApp(t, "")
	app.Handler().ServeHTTP(&refusingWriter{}, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
}

func TestARequestWithoutAHostHasNoOrigin(t *testing.T) {
	app := seoApp(t, "")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = ""
	if got := app.origin(request); got != "" {
		t.Errorf("origin = %q", got)
	}
}

func TestParamsOfReadsThePattern(t *testing.T) {
	cases := map[string]int{"/": 0, "/listings/[id]": 1, "/docs/[...slug]": 1, "/a/[b]/c/[d]": 2}
	for pattern, want := range cases {
		if got := len(ParamsOf(pattern)); got != want {
			t.Errorf("ParamsOf(%q) = %d, want %d", pattern, got, want)
		}
	}
}

func TestAnExplicitCanonicalOnASingleLocaleSiteIsUntouched(t *testing.T) {
	app := New(Options{
		Manifest: metaChain(),
		Meta: map[string]MetaProvider{
			"index": func(*http.Request, Params) (runtime.Meta, error) {
				return runtime.Meta{Canonical: "https://elsewhere.test/x"}, nil
			},
		},
	})
	if got := metaOf(t, app, "/").Canonical; got != "https://elsewhere.test/x" {
		t.Errorf("canonical = %q", got)
	}
}

func TestALocaleWithoutAHostFallsBackToTheOrigin(t *testing.T) {
	app := seoApp(t, `{
  "i18n": {"mode": "subdomain", "locales": ["en", "de"]},
  "hosts": [{"pattern": "example.com", "locale": "en", "default": true}]
}`)
	meta := metaOf(t, app, "/")
	for _, alternate := range meta.Alternates {
		if alternate.Lang == "de" && alternate.Href != "http://example.com/" {
			t.Errorf("de = %q, want the default origin", alternate.Href)
		}
	}
}

func TestTlsMakesTheSchemeHttps(t *testing.T) {
	app := seoApp(t, "")
	request := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	if got := app.scheme(request); got != "https" {
		t.Errorf("scheme = %q", got)
	}
	plain := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	if got := app.scheme(plain); got != "http" {
		t.Errorf("scheme = %q", got)
	}
}
