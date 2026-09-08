package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/redirect"
	"github.com/apptivitypl/gopage/internal/runtime"
)

const spoken = `{
	"i18n": {"locales": ["en", "pl"]},
	"routing": {"aliases": {"pl": {"docs": "dokumenty"}}}
}`

func spokenApp(t *testing.T) *App {
	t.Helper()
	return New(Options{Manifest: metaChain(), Config: settings(t, spoken)})
}

func TestAPublicPathReachesTheCanonicalRoute(t *testing.T) {
	response := get(t, spokenApp(t).Handler(), "/pl/dokumenty/a")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestTheCanonicalPathRedirectsToTheSpokenOne(t *testing.T) {
	response := get(t, spokenApp(t).Handler(), "/pl/docs/a")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/pl/dokumenty/a" {
		t.Errorf("location = %q", got)
	}
}

func TestALocaleWithoutAliasesKeepsItsPaths(t *testing.T) {
	response := get(t, spokenApp(t).Handler(), "/docs/a")
	if response.Code != http.StatusOK {
		t.Errorf("status = %d", response.Code)
	}
}

func TestCanonicalAndAlternatesSpeakThePublicVocabulary(t *testing.T) {
	meta := metaOf(t, spokenApp(t), "/pl/dokumenty/a")
	if meta.Canonical != "http://example.com/pl/dokumenty/a" {
		t.Errorf("canonical = %q", meta.Canonical)
	}
	seen := map[string]string{}
	for _, alternate := range meta.Alternates {
		seen[alternate.Lang] = alternate.Href
	}
	if seen["en"] != "http://example.com/docs/a" || seen["pl"] != "http://example.com/pl/dokumenty/a" {
		t.Errorf("alternates = %v", seen)
	}
}

func TestTheSitemapSpeaksThePublicVocabulary(t *testing.T) {
	app := New(Options{Manifest: metaChain(), Config: settings(t, `{
		"i18n": {"locales": ["en", "pl"]},
		"routing": {"aliases": {"pl": {"features": "funkcje"}}}
	}`)})
	body := get(t, app.Handler(), "/sitemap.xml").Body.String()
	if !strings.Contains(body, "<loc>http://example.com/pl</loc>") {
		t.Errorf("sitemap = %q", body)
	}
}

func TestAReservedPathIsNeverTranslated(t *testing.T) {
	if response := get(t, spokenApp(t).Handler(), "/robots.txt"); response.Code != http.StatusOK {
		t.Errorf("status = %d", response.Code)
	}
}

func TestALoaderCanRedirect(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, redirect.Fail(http.StatusMovedPermanently, "/listings/7")
			},
		},
	})
	response := get(t, app.Handler(), "/")
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/listings/7" {
		t.Errorf("location = %q", got)
	}
}

func TestARedirectToAnotherSiteIsRefused(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, redirect.Fail(http.StatusFound, "javascript:alert(1)")
			},
		},
	})
	if response := get(t, app.Handler(), "/"); response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want the location refused", response.Code)
	}
}

func TestAnOddRedirectStatusFallsBackToFound(t *testing.T) {
	var failure *redirect.Error
	if !errors.As(redirect.Fail(200, "/"), &failure) || failure.Status != http.StatusFound {
		t.Errorf("error = %v", failure)
	}
	if got := redirect.Fail(http.StatusSeeOther, "/x").Error(); !strings.Contains(got, "/x") {
		t.Errorf("message = %q", got)
	}
}

func TestLocalsReachTheLoader(t *testing.T) {
	type deps struct{ Name string }
	var seen deps
	app := New(Options{
		Manifest: manifest(),
		Locals:   deps{Name: "elastic"},
		Props: func() map[string]PropsProvider {
			return map[string]PropsProvider{
				"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
					seen, _ = LocalsFrom(r.Context()).(deps)
					return runtime.Empty{}, nil
				},
			}
		}(),
	})
	get(t, app.Handler(), "/")
	if seen.Name != "elastic" {
		t.Errorf("locals = %+v", seen)
	}
}

func TestAnAppWithoutLocalsCarriesNone(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if LocalsFrom(request.Context()) != nil {
		t.Error("nothing was injected")
	}
	get(t, app.Handler(), "/")
}

func TestAnAddressIsRedirectedToItsOneSpelling(t *testing.T) {
	app := New(Options{Manifest: metaChain(), Config: settings(t, `{
		"i18n": {"locales": ["en", "pl"]},
		"routing": {"normalize": {"trailingSlash": "strip", "case": "lower", "diacritics": "fold"}}
	}`)})
	handler := app.Handler()
	cases := map[string]string{
		"/docs/Kraków":  "/docs/krakow",
		"/docs/a/":      "/docs/a",
		"/pl/docs/Wien": "/pl/docs/wien",
	}
	for from, to := range cases {
		response := get(t, handler, from)
		if response.Code != http.StatusMovedPermanently {
			t.Errorf("%s answered %d", from, response.Code)
			continue
		}
		if got := response.Header().Get("Location"); got != to {
			t.Errorf("%s went to %q, want %q", from, got, to)
		}
	}
	if code := get(t, handler, "/docs/krakow").Code; code != http.StatusOK {
		t.Errorf("the normalised address answered %d", code)
	}
}
