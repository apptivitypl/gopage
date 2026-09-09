package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func post(t *testing.T, handler http.Handler, target, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func taggedApp(t *testing.T, token string) *App {
	t.Helper()
	return New(Options{
		Manifest:   manifest(),
		Config:     settings(t, ""),
		Cache:      cache.New(cache.Options{Limit: 1 << 20}),
		Invalidate: token,
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				cache.From(r.Context()).TTL(time.Hour).Tag("listing")
				return runtime.Empty{}, nil
			},
		},
	})
}

func TestTaggedEntriesAreDroppedThroughTheEndpoint(t *testing.T) {
	app := taggedApp(t, "s3cret")
	handler := app.Handler()
	get(t, handler, "/")
	response := post(t, handler, InvalidatePath, "s3cret", `{"tags":["listing"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var answer struct {
		Dropped int `json:"dropped"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || answer.Dropped != 1 {
		t.Errorf("body = %q, err = %v", response.Body.String(), err)
	}
	if got := get(t, handler, "/").Header().Get(CacheHeader); got != "miss" {
		t.Errorf("cache = %q, want the entry gone", got)
	}
}

func TestInvalidationNeedsTheToken(t *testing.T) {
	handler := taggedApp(t, "s3cret").Handler()
	for _, token := range []string{"", "wrong"} {
		if code := post(t, handler, InvalidatePath, token, `{"tags":["listing"]}`).Code; code != http.StatusForbidden {
			t.Errorf("token %q answered %d", token, code)
		}
	}
	if code := post(t, handler, InvalidatePath, "s3cret", "not json").Code; code != http.StatusBadRequest {
		t.Errorf("broken body answered %d", code)
	}
	if code := get(t, handler, InvalidatePath).Code; code != http.StatusMethodNotAllowed {
		t.Errorf("GET answered %d", code)
	}
}

func TestWithoutATokenTheEndpointIsNotThere(t *testing.T) {
	handler := taggedApp(t, "").Handler()
	if code := post(t, handler, InvalidatePath, "", `{"tags":[]}`).Code; code != http.StatusNotFound {
		t.Errorf("status = %d", code)
	}
}

func TestEveryRequestIsReported(t *testing.T) {
	var seen []Trace
	app := New(Options{
		Manifest:  manifest(),
		Config:    settings(t, ""),
		OnRequest: func(trace Trace) { seen = append(seen, trace) },
	})
	get(t, app.Handler(), "/")
	if len(seen) != 1 {
		t.Fatalf("traces = %+v", seen)
	}
	trace := seen[0]
	if trace.Route != "index" || trace.Status != http.StatusOK || trace.Method != http.MethodGet {
		t.Errorf("trace = %+v", trace)
	}
	if trace.Cache == "" || trace.Duration < 0 {
		t.Errorf("trace = %+v", trace)
	}
}

func TestASlowRouteReportsTheTimeItTook(t *testing.T) {
	const wait = 5 * time.Millisecond
	var seen []Trace
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) {
				time.Sleep(wait)
				return runtime.Empty{}, nil
			},
		},
		OnRequest: func(trace Trace) { seen = append(seen, trace) },
	})
	get(t, app.Handler(), "/")
	if len(seen) != 1 || seen[0].Duration < wait {
		t.Errorf("traces = %+v, want at least %v", seen, wait)
	}
}

func TestAnUnmatchedPathIsReportedWithoutARoute(t *testing.T) {
	var seen Trace
	app := New(Options{
		Manifest:  manifest(),
		Config:    settings(t, ""),
		OnRequest: func(trace Trace) { seen = trace },
	})
	get(t, app.Handler(), "/nowhere/at/all")
	if seen.Route != "" || seen.Status != http.StatusNotFound {
		t.Errorf("trace = %+v", seen)
	}
}

func TestARouteRendersOnItsOwnForATest(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, ""),
		Props: map[string]PropsProvider{
			"listings.id": func(_ *http.Request, params Params) (runtime.Accessible, error) {
				return runtime.Map{"ID": runtime.String(params["id"])}, nil
			},
		},
	})
	body, err := app.RenderRoute(context.Background(), ir.Route{
		Pattern: "/listings/[id]", Name: "listings.id", Plan: 2, Class: ir.ClassDynamic,
	}, Params{"id": "7"})
	if err != nil {
		t.Fatalf("RenderRoute: %v", err)
	}
	if string(body) != "7" {
		t.Errorf("body = %q", body)
	}
}

func TestAnInvalidationWriteFailureIsLogged(t *testing.T) {
	app := taggedApp(t, "s3cret")
	request := httptest.NewRequest(http.MethodPost, InvalidatePath, strings.NewReader(`{"tags":[]}`))
	request.Header.Set("Authorization", "Bearer s3cret")
	app.Handler().ServeHTTP(&refusingWriter{}, request)
}

func TestRenderRouteSpeaksTheVocabulary(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config: settings(t, `{
			"i18n": {"locales": ["en", "pl"]},
			"routing": {"aliases": {"pl": {"listings": "oferty"}}}
		}`),
		Props: map[string]PropsProvider{
			"listings.id": func(r *http.Request, params Params) (runtime.Accessible, error) {
				return runtime.Map{"ID": runtime.String(r.URL.Path)}, nil
			},
		},
	})
	body, err := app.RenderRoute(context.Background(), ir.Route{
		Pattern: "/listings/[id]", Name: "listings.id", Plan: 2, Class: ir.ClassDynamic,
	}, Params{"id": "7"})
	if err != nil {
		t.Fatalf("RenderRoute: %v", err)
	}
	if string(body) != "/listings/7" {
		t.Errorf("body = %q", body)
	}
}

func TestRenderRouteReportsALoaderFailure(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, ""),
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) { return nil, ErrNotFound },
		},
	})
	if _, err := app.RenderRoute(context.Background(), ir.Route{
		Pattern: "/", Name: "index", Plan: 1, LayoutChain: []uint32{0},
	}, Params{}); err == nil {
		t.Error("the failure was swallowed")
	}
}

func TestTheTraceReportsTheLocaleAndTheRoute(t *testing.T) {
	var seen []Trace
	app := New(Options{
		Manifest:  metaChain(),
		Config:    settings(t, `{"i18n": {"locales": ["en", "pl"]}}`),
		OnRequest: func(trace Trace) { seen = append(seen, trace) },
	})
	get(t, app.Handler(), "/pl/docs/a")
	if len(seen) != 1 {
		t.Fatalf("traces = %+v", seen)
	}
	if seen[0].Locale != "pl" || seen[0].Route != "docs.slug" {
		t.Errorf("trace = %+v, want the locale and the route of the prefixed address", seen[0])
	}
	if seen[0].Path != "/pl/docs/a" {
		t.Errorf("path = %q, want the address the visitor asked for", seen[0].Path)
	}
}
