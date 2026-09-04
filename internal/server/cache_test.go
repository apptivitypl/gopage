package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/action"
	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/form"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func counting(t *testing.T, policy func(*cache.Recorder)) (*App, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	app := New(Options{
		Manifest: manifest(),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				policy(cache.From(r.Context()))
				return runtime.Empty{}, nil
			},
		},
	})
	return app, &calls
}

func TestASecondRequestIsServedFromTheCache(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	handler := app.Handler()
	if got := get(t, handler, "/").Header().Get(CacheHeader); got != "miss" {
		t.Errorf("first = %q", got)
	}
	if got := get(t, handler, "/").Header().Get(CacheHeader); got != "hit" {
		t.Errorf("second = %q", got)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want one", calls.Load())
	}
}

func TestAPageWithoutATtlIsNeverCached(t *testing.T) {
	app, calls := counting(t, func(*cache.Recorder) {})
	handler := app.Handler()
	get(t, handler, "/")
	recorder := get(t, handler, "/")
	if got := recorder.Header().Get(CacheHeader); got != "bypass" {
		t.Errorf("header = %q", got)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want every request served fresh", calls.Load())
	}
}

func TestAPrivatePageIsNeverCached(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute).Private() })
	handler := app.Handler()
	get(t, handler, "/")
	get(t, handler, "/")
	if calls.Load() != 2 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestTagsInvalidateCachedPages(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute).Tag("home") })
	handler := app.Handler()
	get(t, handler, "/")
	get(t, handler, "/")
	if removed := app.Invalidate("home"); removed != 1 {
		t.Errorf("removed = %d", removed)
	}
	get(t, handler, "/")
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want a reload after the invalidation", calls.Load())
	}
	if stats := app.CacheStats(); stats.Entries != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestAnAppWithoutACacheBypasses(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	if got := get(t, app.Handler(), "/").Header().Get(CacheHeader); got != "bypass" {
		t.Errorf("header = %q", got)
	}
	if app.Invalidate("x") != 0 || app.CacheStats().Entries != 0 {
		t.Error("an app without a cache reports nothing")
	}
}

func TestARequestCarryingAFlashIsNeverServedFromTheCache(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	handler := app.Handler()
	get(t, handler, "/")

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: hosted(action.FlashCookie), Value: "sent"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Header().Get(CacheHeader); got != "bypass" {
		t.Errorf("header = %q", got)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestAPageThatAcceptsSubmissionsIsNeverCached(t *testing.T) {
	var calls atomic.Int64
	app := New(Options{
		Manifest: manifest(),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				cache.From(r.Context()).TTL(time.Minute)
				return runtime.Empty{}, nil
			},
		},
		Submit: map[string]SubmitProvider{
			"index": func(*http.Request, Params) (action.Action, form.Result, error) { return nil, form.Result{}, nil },
		},
	})
	handler := app.Handler()
	get(t, handler, "/")
	get(t, handler, "/")
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want the token rebuilt each time", calls.Load())
	}
}

func TestTheLocaleIsPartOfTheKey(t *testing.T) {
	var seen atomic.Int64
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				seen.Add(1)
				cache.From(r.Context()).TTL(time.Minute)
				return runtime.Empty{}, nil
			},
		},
	})
	handler := app.Handler()
	get(t, handler, "/")
	get(t, handler, "/pl")
	if seen.Load() != 2 {
		t.Errorf("calls = %d, want one render per locale", seen.Load())
	}
}

func TestTheApiNamespaceKeepsTheLocaleOutOfTheKey(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
	})
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	if key := app.key(request); key.Locale != "" {
		t.Errorf("key = %+v, want no locale on a reserved path", key)
	}
	page := httptest.NewRequest(http.MethodGet, "/", nil)
	if key := app.key(page); key.Path != "/" {
		t.Errorf("key = %+v", key)
	}
}

func TestTheHostIsPartOfTheKeyWhenHostsAreListed(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config: settings(t, "{\"i18n\": {\"mode\": \"subdomain\"}, \"hosts\": [{\"pattern\": \"example.com\", \"locale\": \"en\"}]}"+
			"{\"hosts\": [{\"pattern\": \"pl.example.com\", \"locale\": \"pl\"}]}"),
	})
	first := app.key(httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	second := app.key(httptest.NewRequest(http.MethodGet, "http://pl.example.com/", nil))
	if first.String() == second.String() {
		t.Errorf("hosts must separate the key: %q", first.String())
	}
}

func TestAForgedHostCannotPoisonACachedPage(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	handler := app.Handler()

	forged := httptest.NewRecorder()
	handler.ServeHTTP(forged, httptest.NewRequest(http.MethodGet, "http://evil.test/", nil))
	if got := forged.Header().Get(CacheHeader); got != "miss" {
		t.Fatalf("first = %q", got)
	}

	honest := httptest.NewRecorder()
	handler.ServeHTTP(honest, httptest.NewRequest(http.MethodGet, "http://real.test/", nil))
	if got := honest.Header().Get(CacheHeader); got != "miss" {
		t.Errorf("second = %q, want the forged entry kept out of the honest key", got)
	}
	if calls.Load() != 2 {
		t.Errorf("loader ran %d times, want one render per host", calls.Load())
	}
}

func TestARenderFailureIsNotCached(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, http.ErrBodyNotAllowed
			},
		},
	})
	if recorder := get(t, app.Handler(), "/"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
	if app.CacheStats().Entries != 0 {
		t.Errorf("stats = %+v", app.CacheStats())
	}
}

func TestHeadRequestsSkipTheBody(t *testing.T) {
	app, _ := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	request := httptest.NewRequest(http.MethodHead, "/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Body.Len() != 0 || recorder.Header().Get("Content-Length") == "" {
		t.Errorf("body = %q, length = %q", recorder.Body.String(), recorder.Header().Get("Content-Length"))
	}
}

func TestACachedBodyIsServedVerbatim(t *testing.T) {
	app, _ := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	handler := app.Handler()
	first := get(t, handler, "/").Body.String()
	second := get(t, handler, "/").Body.String()
	if first != second || !strings.Contains(first, "<home") {
		t.Errorf("first = %q, second = %q", first, second)
	}
}

func fragmentManifest(paths []uint32) *ir.Manifest {
	m := manifest()
	m.Plans = append(m.Plans, ir.Plan{
		Fragments: []ir.Fragment{
			{Name: "cached", TTL: int64(time.Minute), Paths: paths},
			{Name: "live"},
		},
		Ops: []ir.Op{
			{Kind: ir.OpFragment, A: 0, B: 2},
			{Kind: ir.OpText, A: 0},
			{Kind: ir.OpFragment, A: 1, B: 4},
			{Kind: ir.OpText, A: 0},
		},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{"Body"}},
		Capacity: 64,
	})
	m.Routes = append(m.Routes, ir.Route{
		Pattern: "/page", Name: "page", Plan: uint32(len(m.Plans) - 1), Class: ir.ClassDynamic,
	})
	return m
}

func fragmentApp(t *testing.T, bodies *[]string, paths []uint32) *App {
	t.Helper()
	var calls int
	return New(Options{
		Manifest: fragmentManifest(paths),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"page": func(*http.Request, Params) (runtime.Accessible, error) {
				body := (*bodies)[min(calls, len(*bodies)-1)]
				calls++
				return runtime.Map{"Body": runtime.String(body)}, nil
			},
		},
	})
}

func TestACachedFragmentSurvivesAPageRerender(t *testing.T) {
	bodies := []string{"first", "second"}
	app := fragmentApp(t, &bodies, nil)
	handler := app.Handler()
	if got := get(t, handler, "/page").Body.String(); got != "firstfirst" {
		t.Fatalf("first = %q", got)
	}
	if got := get(t, handler, "/page").Body.String(); got != "firstsecond" {
		t.Errorf("second = %q, want the cached fragment beside the fresh one", got)
	}
}

func TestAPrivatePageNeverFillsTheFragmentCache(t *testing.T) {
	bodies := []string{"first", "second"}
	app := New(Options{
		Manifest: fragmentManifest(nil),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"page": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				cache.From(r.Context()).Private()
				body := bodies[0]
				bodies = bodies[1:]
				return runtime.Map{"Body": runtime.String(body)}, nil
			},
		},
	})
	handler := app.Handler()
	if got := get(t, handler, "/page").Body.String(); got != "firstfirst" {
		t.Fatalf("first = %q", got)
	}
	if got := get(t, handler, "/page").Body.String(); got != "secondsecond" {
		t.Errorf("second = %q, want nothing kept from a private render", got)
	}
}

func TestAFragmentKeyFollowsWhatItReads(t *testing.T) {
	bodies := []string{"a", "b", "a"}
	app := fragmentApp(t, &bodies, []uint32{0})
	handler := app.Handler()
	get(t, handler, "/page")
	get(t, handler, "/page")
	if got := get(t, handler, "/page").Body.String(); got != "aa" {
		t.Errorf("third = %q, want the first fragment served again", got)
	}
	if entries := app.CacheStats().Entries; entries != 2 {
		t.Errorf("entries = %d, want one per distinct read", entries)
	}
}

func TestAnAppWithoutACacheRendersFragmentsInline(t *testing.T) {
	bodies := []string{"x"}
	app := New(Options{
		Manifest: fragmentManifest(nil),
		Props: map[string]PropsProvider{
			"page": func(*http.Request, Params) (runtime.Accessible, error) {
				return runtime.Map{"Body": runtime.String(bodies[0])}, nil
			},
		},
	})
	if got := get(t, app.Handler(), "/page").Body.String(); got != "xx" {
		t.Errorf("body = %q", got)
	}
}

func TestAFailingFragmentBodyFailsTheRender(t *testing.T) {
	app := New(Options{
		Manifest: fragmentManifest(nil),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"page": func(*http.Request, Params) (runtime.Accessible, error) { return runtime.Empty{}, nil },
		},
	})
	if recorder := get(t, app.Handler(), "/page"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestTheLocaleSeparatesFragmentCaches(t *testing.T) {
	bodies := []string{"a", "a"}
	app := New(Options{
		Manifest: fragmentManifest(nil),
		Config:   settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"page": func(*http.Request, Params) (runtime.Accessible, error) {
				return runtime.Map{"Body": runtime.String(bodies[0])}, nil
			},
		},
	})
	handler := app.Handler()
	get(t, handler, "/page")
	get(t, handler, "/pl/page")
	if entries := app.CacheStats().Entries; entries != 2 {
		t.Errorf("entries = %d, want one fragment per locale", entries)
	}
}

func TestALoaderCanSayThereIsNothingThere(t *testing.T) {
	app := New(Options{
		Manifest: withFallbacks(manifest()),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"listings.id": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, ErrNotFound
			},
		},
	})
	recorder := get(t, app.Handler(), "/listings/99")
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want the not-found page", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "gone") {
		t.Errorf("body = %q, want the nearest not-found page", recorder.Body.String())
	}
}

func TestAWrappedNotFoundIsStillANotFound(t *testing.T) {
	app := New(Options{
		Manifest: withFallbacks(manifest()),
		Props: map[string]PropsProvider{
			"listings.id": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, fmt.Errorf("looking up 99: %w", ErrNotFound)
			},
		},
	})
	if recorder := get(t, app.Handler(), "/listings/99"); recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d", recorder.Code)
	}
}

func askHost(t *testing.T, handler http.Handler, host string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = host
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestAnOriginTheBodyCarriesIsPartOfTheKey(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	handler := app.Handler()

	askHost(t, handler, "victim.test:1337", nil)
	if got := askHost(t, handler, "victim.test", nil).Header().Get(CacheHeader); got == "hit" {
		t.Error("a host with a port shared an entry with the bare host")
	}
	if got := askHost(t, handler, "www.victim.test", nil).Header().Get(CacheHeader); got == "hit" {
		t.Error("a www host shared an entry with the bare host")
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want one render per origin the body would carry", calls.Load())
	}
}

func TestASecondRequestForTheSameOriginStillHits(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	handler := app.Handler()

	askHost(t, handler, "victim.test", nil)
	if got := askHost(t, handler, "victim.test", nil).Header().Get(CacheHeader); got != "hit" {
		t.Errorf("cache = %q, want the same origin served from the cache", got)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want one render", calls.Load())
	}
}

func TestAForwardedSchemeNobodyTrustsChangesNothing(t *testing.T) {
	app, calls := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	handler := app.Handler()

	askHost(t, handler, "victim.test", map[string]string{ForwardedProto: "https"})
	if got := askHost(t, handler, "victim.test", nil).Header().Get(CacheHeader); got != "hit" {
		t.Errorf("cache = %q, want an untrusted header to change no key", got)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want the header ignored", calls.Load())
	}
}

func TestABackgroundRenderWritesNowhere(t *testing.T) {
	sink := discarded{header: http.Header{}}
	sink.Header().Set("Set-Cookie", "gopage.csrf=leak")
	n, err := sink.Write([]byte("a body nobody receives"))
	if err != nil || n != len("a body nobody receives") {
		t.Errorf("write = %d, %v", n, err)
	}
	sink.WriteHeader(http.StatusTeapot)
	if got := sink.Header().Get("Set-Cookie"); got != "gopage.csrf=leak" {
		t.Errorf("header = %q, want the loader still able to set one harmlessly", got)
	}
}

func TestAPageNobodyDeclaredCacheableIsPrivate(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	res := get(t, app.Handler(), "/")
	if got := res.Header().Get("Cache-Control"); got != PrivateFreshness {
		t.Errorf("cache-control = %q, want %q", got, PrivateFreshness)
	}
}

func TestADeclaredTTLReachesTheDownstreamCache(t *testing.T) {
	app, _ := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute).Stale(time.Hour) })
	handler := app.Handler()

	fresh := get(t, handler, "/")
	if got := fresh.Header().Get("Cache-Control"); got != "public, max-age=60, stale-while-revalidate=3600" {
		t.Errorf("cache-control = %q", got)
	}
	hit := get(t, handler, "/")
	if got := hit.Header().Get(CacheHeader); got != "hit" {
		t.Fatalf("cache = %q", got)
	}
	if got := hit.Header().Get("Cache-Control"); got != "public, max-age=60, stale-while-revalidate=3600" {
		t.Errorf("cache-control on a hit = %q, want the policy the entry was stored with", got)
	}
}

func TestAPrivatePageIsNotAdvertisedAsShareable(t *testing.T) {
	app, _ := counting(t, func(r *cache.Recorder) { r.TTL(time.Minute).Private() })
	if got := get(t, app.Handler(), "/").Header().Get("Cache-Control"); got != PrivateFreshness {
		t.Errorf("cache-control = %q, want a page marked private kept out of shared caches", got)
	}
}

func TestAStreamedAndAPartialAnswerStayPrivate(t *testing.T) {
	app := streamApp(t, 0, "inline")
	if got := get(t, app.Handler(), "/").Header().Get("Cache-Control"); got != PrivateFreshness {
		t.Errorf("streamed cache-control = %q", got)
	}

	plain := New(Options{Manifest: manifest()})
	if got := get(t, plain.Handler(), "/nope").Header().Get("Cache-Control"); got != PrivateFreshness {
		t.Errorf("fallback cache-control = %q", got)
	}
}

func TestFreshnessDescribesThePolicy(t *testing.T) {
	cases := map[cache.Policy]string{
		{}:                                     PrivateFreshness,
		{TTL: -time.Second}:                    PrivateFreshness,
		{TTL: 30 * time.Second}:                "public, max-age=30",
		{TTL: time.Minute, Stale: time.Minute}: "public, max-age=60, stale-while-revalidate=60",
	}
	for policy, want := range cases {
		if got := Freshness(policy); got != want {
			t.Errorf("Freshness(%+v) = %q, want %q", policy, got, want)
		}
	}
}
