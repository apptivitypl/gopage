package seo

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
	"github.com/apptivitypl/gopage/internal/vocab"
)

func words(t *testing.T, settings config.Config) vocab.Table {
	t.Helper()
	return vocab.New(settings)
}

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

func paths(derived []Derived) []string {
	list := make([]string, 0, len(derived))
	for _, entry := range derived {
		list = append(list, entry.Entry.Path)
	}
	return list
}

func TestRoutesSkipDynamicAndReservedPatterns(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, "")), settings(t, ""), nil)
	if got := paths(derived); len(got) != 2 {
		t.Fatalf("paths = %v", got)
	}
	for _, entry := range derived {
		if strings.Contains(entry.Entry.Path, "[") || strings.Contains(entry.Entry.Path, "/api") {
			t.Errorf("entry = %+v", entry)
		}
	}
}

func TestEveryLocaleGetsAnEntryWithReciprocalAlternates(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, `{"i18n": {"locales": ["en", "pl"]}}`)), settings(t, `{"i18n": {"locales": ["en", "pl"]}}`), nil)
	if len(derived) != 4 {
		t.Fatalf("derived = %v", paths(derived))
	}
	for _, entry := range derived {
		if len(entry.Entry.Alternates) != 3 {
			t.Errorf("%s carries %d alternates", entry.Entry.Path, len(entry.Entry.Alternates))
		}
	}
	for _, want := range []string{"/", "/pl", "/features", "/pl/features"} {
		if !strings.Contains(strings.Join(paths(derived), " "), want) {
			t.Errorf("%s is missing from %v", want, paths(derived))
		}
	}
}

func TestAlternatesEndWithXDefault(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, `{"i18n": {"locales": ["en", "pl"]}}`)), settings(t, `{"i18n": {"locales": ["en", "pl"]}}`), nil)
	last := derived[0].Entry.Alternates[len(derived[0].Entry.Alternates)-1]
	if last.Lang != DefaultHreflang || last.Href != "/" {
		t.Errorf("x-default = %+v, want the default locale", last)
	}
}

func TestASingleLocaleNeedsNoAlternates(t *testing.T) {
	for _, entry := range FromRoutes(routes(), words(t, settings(t, "")), settings(t, ""), nil) {
		if len(entry.Entry.Alternates) != 0 {
			t.Errorf("entry = %+v", entry)
		}
	}
}

func TestPrefixDefaultMovesTheDefaultLocaleToo(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, `{"i18n": {"locales": ["en", "pl"], "prefixDefault": true}}`)), settings(t, `{"i18n": {"locales": ["en", "pl"], "prefixDefault": true}}`), nil)
	for _, entry := range derived {
		if entry.Entry.Path == "/" || entry.Entry.Path == "/features" {
			t.Errorf("entry = %+v, want every locale prefixed", entry)
		}
	}
}

func TestSubdomainModeLeavesThePathAlone(t *testing.T) {
	text := `{"i18n": {"mode": "subdomain", "locales": ["en", "pl"]}, "hosts": [{"pattern": "example.com", "locale": "en"}]}`
	for _, entry := range FromRoutes(routes(), words(t, settings(t, text)), settings(t, text), nil) {
		if strings.Contains(entry.Entry.Path, "/pl") {
			t.Errorf("entry = %+v", entry)
		}
	}
}

func TestAnOverriddenRouteLeavesTheDerivedList(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, "")), settings(t, ""), map[string]bool{"features": true})
	if got := paths(derived); len(got) != 1 || got[0] != "/" {
		t.Errorf("paths = %v, want the index alone", got)
	}
}

func TestExcludedPathsNeverReachTheSitemap(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, `{"seo": {"sitemap": {"exclude": ["/features"]}}}`)), settings(t, `{"seo": {"sitemap": {"exclude": ["/features"]}}}`), nil)
	if got := paths(derived); len(got) != 1 || got[0] != "/" {
		t.Errorf("paths = %v", got)
	}
	if !Excluded("/features/pricing", settings(t, `{"seo": {"sitemap": {"exclude": ["/features"]}}}`)) {
		t.Error("exclude matches prefixes")
	}
}

func TestDerivedEntriesCarryTheConfiguredDefaults(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, `{"seo": {"sitemap": {"changefreq": "daily", "priority": 0.7}}}`)), settings(t, `{"seo": {"sitemap": {"changefreq": "daily", "priority": 0.7}}}`), nil)
	if derived[0].Entry.ChangeFreq != "daily" || derived[0].Entry.Priority != 0.7 {
		t.Errorf("entry = %+v", derived[0].Entry)
	}
}

func TestMetaDecidesThePathAndTheIndexation(t *testing.T) {
	entry, keep := FromMeta(Entry{Path: "/jobs"}, runtime.Meta{Canonical: "https://x/de/arbeit"})
	if !keep || entry.Path != "https://x/de/arbeit" {
		t.Errorf("entry = %+v, keep = %v", entry, keep)
	}
	if _, keep := FromMeta(Entry{Path: "/map"}, runtime.Meta{Robots: "NOINDEX, follow"}); keep {
		t.Error("a noindex page stays out of the sitemap")
	}
	entry, _ = FromMeta(Entry{Path: "/"}, runtime.Meta{Alternates: runtime.Alternates{{Lang: "pl", Href: "/pl"}}})
	if len(entry.Alternates) != 1 || entry.Alternates[0].Href != "/pl" {
		t.Errorf("alternates = %+v", entry.Alternates)
	}
	entry, keep = FromMeta(Entry{Path: "/"}, runtime.Meta{})
	if !keep || entry.Path != "/" {
		t.Errorf("an empty meta leaves the entry alone: %+v", entry)
	}
}

func TestSitemapIsWellFormed(t *testing.T) {
	derived := FromRoutes(routes(), words(t, settings(t, `{"i18n": {"locales": ["en", "pl"]}}`)), settings(t, `{"i18n": {"locales": ["en", "pl"]}}`), nil)
	xml := render(t, entriesOf(derived), "https://example.com", 100)
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"`,
		`<loc>https://example.com/features</loc>`,
		`<xhtml:link rel="alternate" hreflang="pl" href="https://example.com/pl/features"/>`,
		`<xhtml:link rel="alternate" hreflang="x-default" href="https://example.com/features"/>`,
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

func TestOptionalFieldsAreEmittedOnlyWhenSet(t *testing.T) {
	moment := time.Date(2026, 9, 8, 10, 30, 0, 0, time.UTC)
	full := render(t, Of([]Entry{{Path: "/a", LastMod: moment, ChangeFreq: "weekly", Priority: 0.5}}), "https://x", 10)
	for _, want := range []string{"<lastmod>2026-09-08T10:30:00Z</lastmod>", "<changefreq>weekly</changefreq>", "<priority>0.5</priority>"} {
		if !strings.Contains(full, want) {
			t.Errorf("sitemap = %q, want %q", full, want)
		}
	}
	bare := render(t, Of([]Entry{{Path: "/a"}}), "https://x", 10)
	for _, unwanted := range []string{"lastmod", "changefreq", "priority"} {
		if strings.Contains(bare, unwanted) {
			t.Errorf("sitemap = %q, want no %s", bare, unwanted)
		}
	}
}

func TestAbsoluteEntriesKeepTheirOrigin(t *testing.T) {
	xml := render(t, Of([]Entry{{Path: "https://cdn.example.com/a"}}), "https://x", 10)
	if !strings.Contains(xml, "<loc>https://cdn.example.com/a</loc>") {
		t.Errorf("sitemap = %q", xml)
	}
}

func TestSitemapEscapesUrls(t *testing.T) {
	xml := render(t, Of([]Entry{{
		Path:       "/a&b",
		Alternates: []Alternate{{Lang: `x"y`, Href: "/<c>"}},
	}}), "https://example.com", 10)
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
	xml := render(t, Of(nil), "https://x", 10)
	if !strings.Contains(xml, "<urlset") || !strings.Contains(xml, "</urlset>") {
		t.Errorf("sitemap = %q", xml)
	}
}

func TestAFullSitemapBecomesAnIndex(t *testing.T) {
	xml := render(t, Of(many(5)), "https://x", 2)
	for _, want := range []string{
		"<sitemapindex", "<loc>https://x/sitemap/1.xml</loc>", "<loc>https://x/sitemap/3.xml</loc>", "</sitemapindex>",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("index = %q, want %q", xml, want)
		}
	}
	if strings.Contains(xml, "sitemap/4.xml") {
		t.Errorf("index = %q, want three shards for five entries", xml)
	}
}

func TestAShardHoldsItsOwnSlice(t *testing.T) {
	body, found, err := Shard(Of(many(5)), "https://x", 2, 2)
	if err != nil || !found {
		t.Fatalf("shard: found = %v, err = %v", found, err)
	}
	xml := string(body)
	if !strings.Contains(xml, "/p3") || !strings.Contains(xml, "/p4") || strings.Contains(xml, "/p2") {
		t.Errorf("shard = %q, want the third and fourth entry", xml)
	}
	last, found, err := Shard(Of(many(5)), "https://x", 2, 3)
	if err != nil || !found || strings.Count(string(last), "<url>") != 1 {
		t.Errorf("last shard = %q, found = %v, err = %v", last, found, err)
	}
	if _, found, _ := Shard(Of(many(5)), "https://x", 2, 9); found {
		t.Error("a shard past the end is missing")
	}
}

func TestAFailingSequenceStopsTheDocument(t *testing.T) {
	boom := errors.New("boom")
	failing := func(yield func(Entry, error) bool) {
		yield(Entry{Path: "/a"}, nil)
		yield(Entry{}, boom)
	}
	if _, err := Root(failing, "https://x", 10); !errors.Is(err, boom) {
		t.Errorf("root error = %v, want boom", err)
	}
	if _, _, err := Shard(failing, "https://x", 10, 1); !errors.Is(err, boom) {
		t.Errorf("shard error = %v, want boom", err)
	}
}

func TestSequencesJoinAndStopEarly(t *testing.T) {
	joined := Join(nil, Of(many(2)), Of([]Entry{{Path: "/z"}}))
	seen := 0
	for range joined {
		seen++
		if seen == 2 {
			break
		}
	}
	if seen != 2 {
		t.Errorf("seen = %d, want the range to stop at two", seen)
	}
	total, _, err := take(joined, 0, 10)
	if err != nil || len(total) != 3 {
		t.Errorf("joined = %d entries, err = %v", len(total), err)
	}
}

func TestShardNamesRoundTrip(t *testing.T) {
	index, ok := ShardIndex(strings.TrimPrefix(ShardPath(7), SitemapPrefix))
	if !ok || index != 7 {
		t.Errorf("index = %d, ok = %v", index, ok)
	}
	for _, name := range []string{"2.txt", "x.xml", "0.xml", "-1.xml"} {
		if _, ok := ShardIndex(name); ok {
			t.Errorf("%s is not a shard", name)
		}
	}
}

func TestRobotsPointsAtTheSitemap(t *testing.T) {
	got := string(Robots(settings(t, ""), "https://example.com"))
	if got != "User-agent: *\nAllow: /\n\nSitemap: https://example.com/sitemap.xml\n" {
		t.Errorf("robots = %q", got)
	}
	if bare := string(Robots(settings(t, ""), "")); bare != "User-agent: *\nAllow: /\n" {
		t.Errorf("robots without an origin names no sitemap: %q", bare)
	}
}

func TestRobotsCarriesEveryConfiguredRule(t *testing.T) {
	text := `{"seo": {"robots": {"groups": [
		{"userAgent": ["Googlebot", "Bingbot"], "allow": ["/"], "disallow": ["/api"], "crawlDelay": 10},
		{"userAgent": ["*"], "disallow": ["/"]}
	], "sitemaps": ["/sitemaps/hubs.xml", "https://cdn.example.com/s.xml"]}}}`
	got := string(Robots(settings(t, text), "https://example.com"))
	for _, want := range []string{
		"User-agent: Googlebot\nUser-agent: Bingbot\n",
		"Allow: /\n", "Disallow: /api\n", "Crawl-delay: 10\n",
		"Sitemap: https://example.com/sitemap.xml\n",
		"Sitemap: https://example.com/sitemaps/hubs.xml\n",
		"Sitemap: https://cdn.example.com/s.xml\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("robots = %q, want %q", got, want)
		}
	}
}

func TestRobotsSkipsTheSitemapWhenItIsOff(t *testing.T) {
	got := string(Robots(settings(t, `{"seo": {"sitemap": {"mode": "off"}}}`), "https://example.com"))
	if strings.Contains(got, "Sitemap:") {
		t.Errorf("robots = %q, want no sitemap line", got)
	}
	relative := string(Robots(settings(t, `{"seo": {"robots": {"sitemaps": ["/s.xml"]}}}`), ""))
	if strings.Contains(relative, "Sitemap:") {
		t.Errorf("robots = %q, want no relative sitemap without an origin", relative)
	}
}

func entriesOf(derived []Derived) Seq {
	entries := make([]Entry, 0, len(derived))
	for _, item := range derived {
		entries = append(entries, item.Entry)
	}
	return Of(entries)
}

func many(count int) []Entry {
	entries := make([]Entry, 0, count)
	for index := range count {
		entries = append(entries, Entry{Path: "/p" + string(rune('1'+index))})
	}
	return entries
}

func render(t *testing.T, entries Seq, origin string, limit int) string {
	t.Helper()
	body, err := Root(entries, origin, limit)
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	return string(body)
}
