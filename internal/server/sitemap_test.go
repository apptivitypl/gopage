package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/runtime"
	"github.com/apptivitypl/gopage/internal/seo"
)

func sitemapApp(t *testing.T, text string, opts Options) *App {
	t.Helper()
	opts.Manifest = metaChain()
	opts.Config = settings(t, text)
	if opts.Cache == nil {
		opts.Cache = cache.New(cache.Options{Limit: 1 << 20})
	}
	return New(opts)
}

func metaProvider(meta runtime.Meta, err error, calls *atomic.Int64) MetaProvider {
	return func(*http.Request, Params, runtime.Accessible) (runtime.Meta, error) {
		if calls != nil {
			calls.Add(1)
		}
		return meta, err
	}
}

func TestTheSitemapKeepsTheDerivedEntriesWithoutAProbe(t *testing.T) {
	app := sitemapApp(t, `{"seo": {"sitemap": {"probe": "off"}}}`, Options{
		Meta: map[string]MetaProvider{"index": metaProvider(runtime.Meta{Robots: "noindex"}, nil, nil)},
	})
	body := get(t, app.Handler(), "/sitemap.xml").Body.String()
	if !strings.Contains(body, "<loc>http://example.com/</loc>") {
		t.Errorf("sitemap = %q, want the derived entry", body)
	}
}

func TestAProbedCanonicalBecomesTheLocation(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Meta: map[string]MetaProvider{"index": metaProvider(runtime.Meta{Canonical: "https://example.com/de/arbeit"}, nil, nil)},
	})
	body := get(t, app.Handler(), "/sitemap.xml").Body.String()
	if !strings.Contains(body, "<loc>https://example.com/de/arbeit</loc>") {
		t.Errorf("sitemap = %q, want the canonical", body)
	}
}

func TestANoindexPageStaysOutOfTheSitemap(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Meta: map[string]MetaProvider{"index": metaProvider(runtime.Meta{Robots: "noindex, follow"}, nil, nil)},
	})
	body := get(t, app.Handler(), "/sitemap.xml").Body.String()
	if strings.Contains(body, "<loc>") {
		t.Errorf("sitemap = %q, want no entries", body)
	}
}

func TestAFailingProbeKeepsTheDerivedEntry(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Meta: map[string]MetaProvider{"index": metaProvider(runtime.Meta{}, errors.New("boom"), nil)},
	})
	body := get(t, app.Handler(), "/sitemap.xml").Body.String()
	if !strings.Contains(body, "<loc>http://example.com/</loc>") {
		t.Errorf("sitemap = %q, want the entry to survive a failing loader", body)
	}
}

func TestTheProbeSeesTheLocalisedPath(t *testing.T) {
	var seen []string
	app := sitemapApp(t, `{"i18n": {"locales": ["en", "pl"]}}`, Options{
		Meta: map[string]MetaProvider{"index": func(r *http.Request, _ Params, _ runtime.Accessible) (runtime.Meta, error) {
			seen = append(seen, r.URL.Path+" "+LocaleOf(r))
			return runtime.Meta{}, nil
		}},
	})
	get(t, app.Handler(), "/sitemap.xml")
	if len(seen) != 2 || seen[0] != "/ en" || seen[1] != "/pl pl" {
		t.Errorf("probes = %v", seen)
	}
}

func routeSitemap(entries []seo.Entry, err error, calls *atomic.Int64) SitemapProvider {
	return func(*http.Request) (seo.Seq, error) {
		if calls != nil {
			calls.Add(1)
		}
		if err != nil {
			return nil, err
		}
		return seo.Of(entries), nil
	}
}

func TestARouteSitemapAddsItsOwnEntries(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Sitemap: map[string]SitemapProvider{"listings.id": routeSitemap([]seo.Entry{
			{Path: "/listings/7", ChangeFreq: "daily"},
		}, nil, nil)},
	})
	body := get(t, app.Handler(), "/sitemap.xml").Body.String()
	for _, want := range []string{"<loc>http://example.com/listings/7</loc>", "<changefreq>daily</changefreq>"} {
		if !strings.Contains(body, want) {
			t.Errorf("sitemap = %q, want %q", body, want)
		}
	}
}

func TestARouteSitemapReplacesTheDerivedEntries(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Sitemap: map[string]SitemapProvider{"index": routeSitemap([]seo.Entry{{Path: "/home"}}, nil, nil)},
	})
	body := get(t, app.Handler(), "/sitemap.xml").Body.String()
	if strings.Contains(body, "<loc>http://example.com/</loc>") || !strings.Contains(body, "/home") {
		t.Errorf("sitemap = %q", body)
	}
}

func TestAnExcludedRouteSkipsItsProvider(t *testing.T) {
	var calls atomic.Int64
	app := sitemapApp(t, `{"seo": {"sitemap": {"exclude": ["/listings"]}}}`, Options{
		Sitemap: map[string]SitemapProvider{"listings.id": routeSitemap(nil, nil, &calls)},
	})
	get(t, app.Handler(), "/sitemap.xml")
	if calls.Load() != 0 {
		t.Errorf("calls = %d, want the exclusion to win", calls.Load())
	}
}

func TestANilSequenceContributesNothing(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Sitemap: map[string]SitemapProvider{"listings.id": func(*http.Request) (seo.Seq, error) { return nil, nil }},
	})
	if code := get(t, app.Handler(), "/sitemap.xml").Code; code != http.StatusOK {
		t.Errorf("status = %d", code)
	}
}

func TestAFailingRouteSitemapAnswersWithAnError(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Sitemap: map[string]SitemapProvider{"index": routeSitemap(nil, errors.New("boom"), nil)},
	})
	if code := get(t, app.Handler(), "/sitemap.xml").Code; code != http.StatusInternalServerError {
		t.Errorf("status = %d, want an incomplete sitemap to fail loudly", code)
	}
}

func TestTheSitemapSplitsIntoShards(t *testing.T) {
	app := sitemapApp(t, `{"seo": {"sitemap": {"limit": 1}}}`, Options{
		Sitemap: map[string]SitemapProvider{"listings.id": routeSitemap([]seo.Entry{
			{Path: "/listings/1"}, {Path: "/listings/2"},
		}, nil, nil)},
	})
	handler := app.Handler()
	root := get(t, handler, "/sitemap.xml").Body.String()
	if !strings.Contains(root, "<sitemapindex") || !strings.Contains(root, "http://example.com/sitemap/3.xml") {
		t.Errorf("index = %q", root)
	}
	second := get(t, handler, "/sitemap/2.xml")
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "/listings/1") {
		t.Errorf("shard = %q", second.Body.String())
	}
	if code := get(t, handler, "/sitemap/9.xml").Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want a missing shard to answer 404", code)
	}
	if code := get(t, handler, "/sitemap/none").Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want a malformed shard to answer 404", code)
	}
}

func TestASecondSitemapRequestIsServedFromTheCache(t *testing.T) {
	var calls atomic.Int64
	app := sitemapApp(t, "", Options{
		Meta: map[string]MetaProvider{"index": metaProvider(runtime.Meta{}, nil, &calls)},
	})
	handler := app.Handler()
	first := get(t, handler, "/sitemap.xml")
	second := get(t, handler, "/sitemap.xml")
	if calls.Load() != 1 {
		t.Errorf("probes = %d, want the second request to hit the cache", calls.Load())
	}
	if first.Header().Get(CacheHeader) == second.Header().Get(CacheHeader) {
		t.Errorf("cache header = %q twice", first.Header().Get(CacheHeader))
	}
	if got := second.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=3600") {
		t.Errorf("cache-control = %q", got)
	}
}

func TestARouteShortensTheFreshnessAndTagsTheDocument(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Sitemap: map[string]SitemapProvider{"index": func(r *http.Request) (seo.Seq, error) {
			cache.From(r.Context()).TTL(time.Minute).Tag("listing")
			return seo.Of([]seo.Entry{{Path: "/"}}), nil
		}},
	})
	handler := app.Handler()
	if got := get(t, handler, "/sitemap.xml").Header().Get("Cache-Control"); !strings.Contains(got, "max-age=60") {
		t.Errorf("cache-control = %q, want the shorter ttl", got)
	}
	if dropped := app.Invalidate("listing"); dropped != 1 {
		t.Errorf("invalidated %d entries, want the sitemap to carry the tag", dropped)
	}
}

func TestAPrivateRouteKeepsTheSitemapOutOfTheCache(t *testing.T) {
	var calls atomic.Int64
	app := sitemapApp(t, "", Options{
		Sitemap: map[string]SitemapProvider{"index": func(r *http.Request) (seo.Seq, error) {
			calls.Add(1)
			cache.From(r.Context()).Private()
			return seo.Of([]seo.Entry{{Path: "/"}}), nil
		}},
	})
	handler := app.Handler()
	get(t, handler, "/sitemap.xml")
	response := get(t, handler, "/sitemap.xml")
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want nothing cached", calls.Load())
	}
	if got := response.Header().Get("Cache-Control"); got != PrivateFreshness {
		t.Errorf("cache-control = %q", got)
	}
}

func TestAnAppWithoutACacheStillAnswers(t *testing.T) {
	app := sitemapApp(t, "", Options{Cache: nil})
	app.cache = nil
	response := get(t, app.Handler(), "/sitemap.xml")
	if response.Code != http.StatusOK || response.Header().Get(CacheHeader) != cache.StatusBypass.String() {
		t.Errorf("status = %d, cache = %q", response.Code, response.Header().Get(CacheHeader))
	}
}

func TestAFailureWithoutACacheIsReported(t *testing.T) {
	app := sitemapApp(t, "", Options{
		Sitemap: map[string]SitemapProvider{"index": routeSitemap(nil, errors.New("boom"), nil)},
	})
	app.cache = nil
	if code := get(t, app.Handler(), "/sitemap.xml").Code; code != http.StatusInternalServerError {
		t.Errorf("status = %d", code)
	}
}

func TestTurningTheGeneratorsOffLeavesThePathsFree(t *testing.T) {
	app := sitemapApp(t, `{"seo": {"sitemap": {"mode": "off"}, "robots": {"mode": "off"}}}`, Options{})
	handler := app.Handler()
	for _, target := range []string{"/sitemap.xml", "/robots.txt", "/sitemap/1.xml"} {
		if code := get(t, handler, target).Code; code != http.StatusNotFound {
			t.Errorf("%s answered %d, want the path left to the project", target, code)
		}
	}
}

func TestARouteOfTheProjectWinsOverTheBuiltinEndpoint(t *testing.T) {
	own := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("mine")) })
	app := sitemapApp(t, "", Options{API: map[string]http.Handler{"/sitemap.xml": own}})
	if body := get(t, app.Handler(), "/sitemap.xml").Body.String(); body != "mine" {
		t.Errorf("body = %q, want the project route", body)
	}
}

func TestAPublicFileWinsOverRobots(t *testing.T) {
	own := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("public")) })
	app := sitemapApp(t, "", Options{Assets: own, Public: []string{"/robots.txt"}})
	if body := get(t, app.Handler(), "/robots.txt").Body.String(); body != "public" {
		t.Errorf("body = %q, want the public file", body)
	}
}

func TestTheSitemapRefreshesInTheBackground(t *testing.T) {
	var calls atomic.Int64
	app := sitemapApp(t, `{"seo": {"sitemap": {"ttl": "1ns", "stale": "1h"}}}`, Options{
		Sitemap: map[string]SitemapProvider{"index": routeSitemap([]seo.Entry{{Path: "/"}}, nil, &calls)},
	})
	handler := app.Handler()
	get(t, handler, "/sitemap.xml")
	time.Sleep(time.Millisecond)
	get(t, handler, "/sitemap.xml")
	app.cache.Wait()
	if calls.Load() < 2 {
		t.Errorf("calls = %d, want a background refresh", calls.Load())
	}
}

func TestHeadOnRobotsSendsNoBody(t *testing.T) {
	app := sitemapApp(t, "", Options{})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/robots.txt", nil))
	if recorder.Body.Len() != 0 {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestAShardStopsAtItsLimit(t *testing.T) {
	app := sitemapApp(t, `{"i18n": {"locales": ["en", "pl"]}, "seo": {"sitemap": {"limit": 1}}}`, Options{})
	body := get(t, app.Handler(), "/sitemap/1.xml").Body.String()
	if strings.Count(body, "<url>") != 1 {
		t.Errorf("shard = %q, want a single entry", body)
	}
}

func TestAFailingShardIsReported(t *testing.T) {
	app := sitemapApp(t, `{"seo": {"sitemap": {"limit": 1}}}`, Options{
		Sitemap: map[string]SitemapProvider{"index": routeSitemap(nil, errors.New("boom"), nil)},
	})
	if code := get(t, app.Handler(), "/sitemap/1.xml").Code; code != http.StatusInternalServerError {
		t.Errorf("status = %d", code)
	}
}
