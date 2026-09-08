package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/reply"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func answering(t *testing.T, text string, answer func(*http.Request)) (*App, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, text),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				cache.From(r.Context()).TTL(time.Minute)
				answer(r)
				return runtime.Empty{}, nil
			},
		},
	})
	return app, &calls
}

func withCookie(t *testing.T, handler http.Handler, target, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Cookie", cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

const sessionConfig = `{"security": {"privateCookies": ["session"]}}`

func TestARequestCarryingAPrivateCookieIsNeverCached(t *testing.T) {
	app, calls := answering(t, sessionConfig, func(*http.Request) {})
	handler := app.Handler()
	response := withCookie(t, handler, "/", "session=abc")
	if got := response.Header().Get(CacheHeader); got != cache.StatusBypass.String() {
		t.Errorf("cache = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != PrivateFreshness {
		t.Errorf("cache-control = %q", got)
	}
	if got := response.Header().Get(reply.VaryHeader); !strings.Contains(got, reply.CookieVary) {
		t.Errorf("vary = %q", got)
	}
	withCookie(t, handler, "/", "session=abc")
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want every request rendered fresh", calls.Load())
	}
}

func TestAnAnonymousRequestKeepsTheSharedCopy(t *testing.T) {
	app, calls := answering(t, sessionConfig, func(*http.Request) {})
	handler := app.Handler()
	get(t, handler, "/")
	response := get(t, handler, "/")
	if got := response.Header().Get(CacheHeader); got != "hit" {
		t.Errorf("cache = %q", got)
	}
	if strings.Contains(response.Header().Get(reply.VaryHeader), reply.CookieVary) {
		t.Errorf("vary = %q, want no cookie dependency", response.Header().Get(reply.VaryHeader))
	}
	withCookie(t, handler, "/", "session=abc")
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want the anonymous copy kept", calls.Load())
	}
}

func TestAnotherCookieLeavesTheCacheAlone(t *testing.T) {
	app, calls := answering(t, sessionConfig, func(*http.Request) {})
	handler := app.Handler()
	withCookie(t, handler, "/", "theme=dark")
	response := withCookie(t, handler, "/", "theme=dark")
	if got := response.Header().Get(CacheHeader); got != "hit" {
		t.Errorf("cache = %q", got)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestALoaderThatSetsACookieIsNeverCached(t *testing.T) {
	app, calls := answering(t, "", func(r *http.Request) {
		cache.From(r.Context()).Private()
		reply.From(r.Context()).SetCookie(&http.Cookie{Name: "session", Value: "abc", HttpOnly: true})
	})
	handler := app.Handler()
	response := get(t, handler, "/")
	if got := response.Header().Get("Set-Cookie"); !strings.Contains(got, "session=abc") {
		t.Errorf("set-cookie = %q", got)
	}
	get(t, handler, "/")
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want a response with a cookie kept out of the cache", calls.Load())
	}
}

func TestAStatusFromTheLoaderSurvivesTheCache(t *testing.T) {
	app, _ := answering(t, "", func(r *http.Request) {
		reply.From(r.Context()).Status(http.StatusGone)
		reply.From(r.Context()).Header().Set("X-Robots-Tag", "noindex, follow")
	})
	handler := app.Handler()
	first := get(t, handler, "/")
	second := get(t, handler, "/")
	for _, response := range []*httptest.ResponseRecorder{first, second} {
		if response.Code != http.StatusGone {
			t.Errorf("status = %d", response.Code)
		}
		if got := response.Header().Get("X-Robots-Tag"); got != "noindex, follow" {
			t.Errorf("x-robots-tag = %q", got)
		}
	}
	if second.Header().Get(CacheHeader) != "hit" {
		t.Errorf("cache = %q", second.Header().Get(CacheHeader))
	}
}

func TestTheFrameworkOwnsItsOwnHeaders(t *testing.T) {
	app, _ := answering(t, "", func(r *http.Request) {
		reply.From(r.Context()).Header().Set("Content-Type", "text/plain")
		reply.From(r.Context()).Header().Set("Cache-Control", "public, max-age=99999")
	})
	response := get(t, app.Handler(), "/")
	if got := response.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Errorf("content-type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=60") {
		t.Errorf("cache-control = %q", got)
	}
}

func TestACdnHeaderPassesThrough(t *testing.T) {
	app, _ := answering(t, "", func(r *http.Request) {
		reply.From(r.Context()).Header().Set("CDN-Cache-Control", "public, s-maxage=3600")
	})
	if got := get(t, app.Handler(), "/").Header().Get("CDN-Cache-Control"); got != "public, s-maxage=3600" {
		t.Errorf("cdn-cache-control = %q", got)
	}
}

func TestARefreshInTheBackgroundDropsCookies(t *testing.T) {
	var calls atomic.Int64
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, ""),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				cache.From(r.Context()).TTL(time.Nanosecond).Stale(time.Hour)
				reply.From(r.Context()).Header().Set("X-Pass", "1")
				return runtime.Empty{}, nil
			},
		},
	})
	handler := app.Handler()
	get(t, handler, "/")
	time.Sleep(time.Millisecond)
	response := get(t, handler, "/")
	app.cache.Wait()
	if response.Header().Get("X-Pass") != "1" {
		t.Errorf("headers = %v", response.Header())
	}
	if calls.Load() < 2 {
		t.Errorf("calls = %d, want a background refresh", calls.Load())
	}
}

func TestAStaticExportRefusesAPageThatWritesToTheResponse(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				reply.From(r.Context()).SetCookie(&http.Cookie{Name: "session", Value: "abc"})
				return runtime.Empty{}, nil
			},
		},
	})
	_, err := app.RenderStatic(ir.Route{Pattern: "/", Name: "index", Plan: 1, LayoutChain: []uint32{0}})
	if err == nil || !strings.Contains(err.Error(), "static page") {
		t.Fatalf("RenderStatic: %v, want a refusal", err)
	}
}

func TestAStaticExportKeepsAQuietPage(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	if _, err := app.RenderStatic(ir.Route{Pattern: "/", Name: "index", Plan: 1, LayoutChain: []uint32{0}}); err != nil {
		t.Fatalf("RenderStatic: %v", err)
	}
}
