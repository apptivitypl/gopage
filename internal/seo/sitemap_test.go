package seo

import (
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/config"
	"github.com/apptivitypl/rill/internal/ir"
)

func settings(t *testing.T, text string) config.Config {
	t.Helper()
	parsed, err := config.Parse(text)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return parsed
}

func routes() []ir.Route {
	return []ir.Route{
		{Pattern: "/", Name: "index"},
		{Pattern: "/features", Name: "features"},
		{Pattern: "/listings/[id]", Name: "listings.id"},
		{Pattern: "/api/health", Name: "api.health"},
	}
}

func TestPagesSkipDynamicAndReservedRoutes(t *testing.T) {
	pages := Pages(routes(), settings(t, ""), "https://example.com")
	if len(pages) != 2 {
		t.Fatalf("pages = %+v", pages)
	}
	for _, page := range pages {
		if strings.Contains(page.Path, "[") || strings.Contains(page.Path, "/api") {
			t.Errorf("page = %+v", page)
		}
	}
}

func TestEveryLocaleGetsAnEntryWithReciprocalAlternates(t *testing.T) {
	pages := Pages(routes(), settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"), "https://example.com")
	if len(pages) != 4 {
		t.Fatalf("pages = %+v", pages)
	}
	paths := map[string]bool{}
	for _, page := range pages {
		paths[page.Path] = true
		if len(page.Alternates) != 2 {
			t.Errorf("%s carries %d alternates", page.Path, len(page.Alternates))
		}
	}
	for _, want := range []string{
		"https://example.com/", "https://example.com/pl",
		"https://example.com/features", "https://example.com/pl/features",
	} {
		if !paths[want] {
			t.Errorf("%s is missing from %v", want, paths)
		}
	}
}

func TestASingleLocaleNeedsNoAlternates(t *testing.T) {
	pages := Pages(routes(), settings(t, ""), "https://example.com")
	for _, page := range pages {
		if len(page.Alternates) != 0 {
			t.Errorf("page = %+v", page)
		}
	}
}

func TestPrefixDefaultMovesTheDefaultLocaleToo(t *testing.T) {
	pages := Pages(routes(), settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"], \"prefixDefault\": true}}"), "https://x")
	for _, page := range pages {
		if page.Path == "https://x/" || page.Path == "https://x/features" {
			t.Errorf("page = %+v, want every locale prefixed", page)
		}
	}
}

func TestSubdomainModeLeavesThePathAlone(t *testing.T) {
	text := "{\"i18n\": {\"mode\": \"subdomain\", \"locales\": [\"en\", \"pl\"]}, \"hosts\": [{\"pattern\": \"example.com\", \"locale\": \"en\"}]}"
	pages := Pages(routes(), settings(t, text), "https://example.com")
	for _, page := range pages {
		if strings.Contains(page.Path, "/pl") {
			t.Errorf("page = %+v", page)
		}
	}
}

func TestSitemapIsWellFormed(t *testing.T) {
	pages := Pages(routes(), settings(t, "{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"), "https://example.com")
	xml := string(Sitemap(pages))
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"`,
		`<loc>https://example.com/features</loc>`,
		`<xhtml:link rel="alternate" hreflang="pl" href="https://example.com/pl/features"/>`,
		"</urlset>",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("sitemap = %q, want %q", xml, want)
		}
	}
	if strings.Count(xml, "<url>") != strings.Count(xml, "</url>") {
		t.Errorf("sitemap = %q, want balanced entries", xml)
	}
}

func TestSitemapEscapesUrls(t *testing.T) {
	xml := string(Sitemap([]Page{{
		Path:       "https://example.com/a&b",
		Alternates: []Alternate{{Lang: `x"y`, Href: "https://example.com/<c>"}},
	}}))
	for _, forbidden := range []string{"/a&b<", `x"y"`, "<c>"} {
		if strings.Contains(xml, forbidden) {
			t.Errorf("sitemap = %q, want %q escaped", xml, forbidden)
		}
	}
	if !strings.Contains(xml, "a&amp;b") || !strings.Contains(xml, "&lt;c&gt;") {
		t.Errorf("sitemap = %q", xml)
	}
}

func TestAnEmptySitemapIsStillValid(t *testing.T) {
	xml := string(Sitemap(nil))
	if !strings.Contains(xml, "<urlset") || !strings.Contains(xml, "</urlset>") {
		t.Errorf("sitemap = %q", xml)
	}
}

func TestRobotsPointsAtTheSitemap(t *testing.T) {
	got := string(Robots("https://example.com"))
	if !strings.Contains(got, "User-agent: *") || !strings.Contains(got, "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("robots = %q", got)
	}
	if strings.Contains(string(Robots("")), "Sitemap:") {
		t.Errorf("robots without an origin names no sitemap: %q", Robots(""))
	}
}
