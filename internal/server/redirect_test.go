package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestALocalePrefixNeverRedirectsOffSite(t *testing.T) {
	app := New(Options{Manifest: manifest(), Config: settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")})
	for _, target := range []string{"/en//evil.com", "/en/\\evil.com", "/en//attacker.test/path"} {
		recorder := get(t, app.Handler(), target)
		if location := recorder.Header().Get("Location"); location != "" {
			t.Errorf("%s: location = %q, want no redirect at all", target, location)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, recorder.Code)
		}
	}
}

func TestAWildcardRedirectNeverBuildsAnOffSiteTarget(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"redirects\": [{\"from\": \"/old/*\", \"to\": \"/*\"}]}"),
	})
	recorder := get(t, app.Handler(), "/old//evil.com")
	if location := recorder.Header().Get("Location"); strings.Contains(location, "evil.com") {
		t.Errorf("location = %q, want the host refused", location)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

func TestAConfiguredAbsoluteRedirectStillWorks(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"redirects\": [{\"from\": \"/docs\", \"to\": \"https://docs.example.com/\", \"status\": 302}]}"),
	})
	recorder := get(t, app.Handler(), "/docs")
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "https://docs.example.com/" {
		t.Errorf("status = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestARewriteNeverLeavesTheApplication(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, "{\"rewrites\": [{\"from\": \"/start/*\", \"to\": \"/*\"}]}"),
	})
	if recorder := get(t, app.Handler(), "/start//evil.com"); recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
	if recorder := get(t, app.Handler(), "/start/"); recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want a plain rewrite to still work", recorder.Code)
	}
	if recorder := get(t, app.Handler(), "/"); recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want a path no rewrite matches to pass through", recorder.Code)
	}
}
