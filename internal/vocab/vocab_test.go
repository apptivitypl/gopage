package vocab

import (
	"context"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/config"
)

func table(t *testing.T, text string) Table {
	t.Helper()
	settings, err := config.Parse(text)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return New(settings)
}

const twoLocales = `{
	"i18n": {"locales": ["en", "pl", "de"]},
	"routing": {"aliases": {
		"pl": {"jobs": "praca", "offer": "oferta"},
		"de": {"jobs": "arbeit"}
	}}
}`

func TestAnAppWithoutAliasesTranslatesNothing(t *testing.T) {
	empty := table(t, "")
	if empty.Speaks("pl") {
		t.Error("no locale speaks its own vocabulary")
	}
	if got := empty.Public("pl", "/jobs/offer"); got != "/jobs/offer" {
		t.Errorf("path = %q", got)
	}
}

func TestSegmentsTranslateBothWays(t *testing.T) {
	words := table(t, twoLocales)
	cases := map[string]string{
		"/jobs":             "/praca",
		"/jobs/offer":       "/praca/oferta",
		"/jobs/warszawa":    "/praca/warszawa",
		"/about/jobs/offer": "/about/praca/oferta",
		"/":                 "/",
		"":                  "",
		"/unknown":          "/unknown",
	}
	for canonical, public := range cases {
		if got := words.Public("pl", canonical); got != public {
			t.Errorf("public %q = %q, want %q", canonical, got, public)
		}
		if got := words.Canonical("pl", public); got != canonical {
			t.Errorf("canonical %q = %q, want %q", public, got, canonical)
		}
	}
}

func TestEachLocaleKeepsItsOwnWords(t *testing.T) {
	words := table(t, twoLocales)
	if got := words.Public("de", "/jobs/offer"); got != "/arbeit/offer" {
		t.Errorf("path = %q", got)
	}
	if got := words.Public("en", "/jobs"); got != "/jobs" {
		t.Errorf("a locale without aliases keeps the canonical path: %q", got)
	}
	if !words.Speaks("pl") || words.Speaks("en") {
		t.Error("only a locale with aliases speaks")
	}
}

func TestATrailingSegmentTranslatesToo(t *testing.T) {
	words := table(t, twoLocales)
	if got := words.Public("pl", "/offer"); got != "/oferta" {
		t.Errorf("path = %q", got)
	}
	if got := words.Public("pl", "/x/jobs"); got != "/x/praca" {
		t.Errorf("path = %q", got)
	}
}

func TestLocalisePrefixesAndTranslates(t *testing.T) {
	words := table(t, twoLocales)
	cases := map[[2]string]string{
		{"en", "/jobs"}:  "/jobs",
		{"pl", "/jobs"}:  "/pl/praca",
		{"de", "/jobs"}:  "/de/arbeit",
		{"pl", "/"}:      "/pl",
		{"en", "/"}:      "/",
		{"pl", "/about"}: "/pl/about",
	}
	for input, want := range cases {
		if got := words.Localise(input[0], input[1]); got != want {
			t.Errorf("Localise(%q, %q) = %q, want %q", input[0], input[1], got, want)
		}
	}
}

func TestThePrefixFollowsTheConfiguredMode(t *testing.T) {
	prefixed := table(t, `{"i18n": {"locales": ["en", "pl"], "prefixDefault": true}}`)
	if got := prefixed.Localise("en", "/jobs"); got != "/en/jobs" {
		t.Errorf("path = %q", got)
	}
	if got := prefixed.Localise("en", "/"); got != "/en" {
		t.Errorf("path = %q", got)
	}
	hosted := table(t, `{"i18n": {"mode": "subdomain", "locales": ["en", "pl"]}, "hosts": [{"pattern": "x.com", "locale": "en"}]}`)
	if got := hosted.Localise("pl", "/jobs"); got != "/jobs" {
		t.Errorf("subdomain mode adds no prefix: %q", got)
	}
}

func TestOnlyAMultilingualAppLocalises(t *testing.T) {
	if table(t, "").Localises() {
		t.Error("a single locale localises nothing")
	}
	if !table(t, `{"i18n": {"locales": ["en", "pl"]}}`).Localises() {
		t.Error("two locales localise")
	}
	if !table(t, twoLocales).Localises() {
		t.Error("aliases alone are enough to localise")
	}
	if !table(t, twoLocales).Any() || table(t, "").Any() {
		t.Error("Any reports the alias tables")
	}
}

func TestTheTableTravelsInTheContext(t *testing.T) {
	words := table(t, twoLocales)
	if got := From(With(context.Background(), words)).Public("pl", "/jobs"); got != "/praca" {
		t.Errorf("path = %q", got)
	}
	if got := From(context.Background()).Public("pl", "/jobs"); got != "/jobs" {
		t.Errorf("an absent table translates nothing: %q", got)
	}
}

func TestAliasesAreValidated(t *testing.T) {
	cases := map[string]string{
		`{"routing": {"aliases": {"pl": {"jobs": "praca"}}}}`:                                              "not a configured locale",
		`{"i18n": {"locales": ["en", "pl"]}, "routing": {"aliases": {"pl": {"jobs": "a/b"}}}}`:             "not a path segment",
		`{"i18n": {"locales": ["en", "pl"]}, "routing": {"aliases": {"pl": {"jobs": ""}}}}`:                "not a path segment",
		`{"i18n": {"locales": ["en", "pl"]}, "routing": {"aliases": {"pl": {"jobs": "x", "offer": "x"}}}}`: "same segment",
	}
	for text, want := range cases {
		if _, err := config.Parse(text); err == nil {
			t.Errorf("%s was accepted", text)
		} else if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want %q", text, err, want)
		}
	}
}
