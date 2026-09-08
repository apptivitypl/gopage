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
	"github.com/apptivitypl/gopage/internal/runtime"
)

func varyManifest(dimensions []ir.Vary) *ir.Manifest {
	held := manifest()
	held.Routes[0].Vary = dimensions
	return held
}

func themed(t *testing.T, dimensions []ir.Vary) (*App, *atomic.Int64, *[]string) {
	t.Helper()
	var calls atomic.Int64
	seen := make([]string, 0, 4)
	app := New(Options{
		Manifest: varyManifest(dimensions),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				cache.From(r.Context()).TTL(time.Minute)
				bucket, _ := BucketsFrom(r.Context()).Cookie("theme")
				seen = append(seen, bucket)
				return runtime.Empty{}, nil
			},
		},
	})
	return app, &calls, &seen
}

func withTheme(t *testing.T, handler http.Handler, value string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if value != "" {
		request.AddCookie(&http.Cookie{Name: "theme", Value: value})
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

var themes = []ir.Vary{{Kind: ir.VaryCookie, Name: "theme", Values: []string{"light", "dark"}}}

func TestEachDeclaredValueGetsItsOwnEntry(t *testing.T) {
	app, calls, _ := themed(t, themes)
	handler := app.Handler()
	for _, value := range []string{"light", "dark"} {
		if got := withTheme(t, handler, value).Header().Get(CacheHeader); got != "miss" {
			t.Errorf("first %s = %q", value, got)
		}
		if got := withTheme(t, handler, value).Header().Get(CacheHeader); got != "hit" {
			t.Errorf("second %s = %q", value, got)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("loader ran %d times, want one render per declared value", calls.Load())
	}
}

func TestAValueOutsideTheListSharesOneEntry(t *testing.T) {
	app, calls, seen := themed(t, themes)
	handler := app.Handler()
	withTheme(t, handler, "chartreuse")
	withTheme(t, handler, "vermilion")
	if got := withTheme(t, handler, "").Header().Get(CacheHeader); got != "hit" {
		t.Errorf("no cookie = %q, want the same entry as an unlisted value", got)
	}
	if calls.Load() != 1 {
		t.Errorf("loader ran %d times, want one render for every value outside the list", calls.Load())
	}
	if len(*seen) != 1 || (*seen)[0] != "" {
		t.Errorf("loader saw %v, want the empty bucket rather than the raw value", *seen)
	}
}

func TestADeclaredCookieKeepsTheResponseShareable(t *testing.T) {
	app, _, _ := themed(t, themes)
	answer := withTheme(t, app.Handler(), "dark")
	if got := answer.Header().Get("Cache-Control"); !strings.HasPrefix(got, "public") {
		t.Errorf("cache-control = %q, want a shared response", got)
	}
	if got := answer.Header().Get("Vary"); !strings.Contains(got, "Cookie") {
		t.Errorf("vary = %q, want the cookie named", got)
	}
}

func TestAHeaderDimensionNamesItselfInVary(t *testing.T) {
	app, _, _ := themed(t, []ir.Vary{{Kind: ir.VaryHeader, Name: "CF-IPCountry", Values: []string{"PL"}}})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("CF-IPCountry", "PL")
	first := httptest.NewRecorder()
	app.Handler().ServeHTTP(first, request)
	if got := first.Header().Get("Vary"); !strings.Contains(got, "CF-IPCountry") {
		t.Errorf("vary = %q", got)
	}
	second := httptest.NewRecorder()
	app.Handler().ServeHTTP(second, request)
	if got := second.Header().Get(CacheHeader); got != "hit" {
		t.Errorf("second = %q", got)
	}
	other := httptest.NewRequest(http.MethodGet, "/", nil)
	other.Header.Set("CF-IPCountry", "DE")
	third := httptest.NewRecorder()
	app.Handler().ServeHTTP(third, other)
	if got := third.Header().Get(CacheHeader); got != "miss" {
		t.Errorf("another country = %q, want its own entry", got)
	}
}

func TestARouteWithoutDimensionsCarriesNoBuckets(t *testing.T) {
	app, _, _ := themed(t, nil)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if got := recorder.Header().Get("Vary"); strings.Contains(got, "Cookie") {
		t.Errorf("vary = %q, want no cookie dimension", got)
	}
	if BucketsFrom(request.Context()) != nil {
		t.Error("a route that declares nothing must carry no buckets")
	}
}

func TestBucketsAnswerSafelyWhenAbsent(t *testing.T) {
	var absent *Buckets
	if got := absent.Variant(); got != "" {
		t.Errorf("variant = %q", got)
	}
	if _, ok := absent.Cookie("theme"); ok {
		t.Error("nothing is declared")
	}
	if got := absent.Headers(); got != nil {
		t.Errorf("headers = %v", got)
	}
	held := &Buckets{dimensions: themes, values: []string{"dark"}}
	if _, ok := held.Cookie("other"); ok {
		t.Error("only the declared cookie is bucketed")
	}
}
