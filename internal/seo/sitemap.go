package seo

import (
	"iter"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
	"github.com/apptivitypl/gopage/internal/vocab"
)

const (
	SitemapPath     = "/sitemap.xml"
	SitemapPrefix   = "/sitemap/"
	SitemapSuffix   = ".xml"
	RobotsPath      = "/robots.txt"
	SitemapType     = "application/xml"
	RobotsType      = "text/plain; charset=utf-8"
	DefaultHreflang = "x-default"
	namespace       = "http://www.sitemaps.org/schemas/sitemap/0.9"
	xhtml           = "http://www.w3.org/1999/xhtml"
)

type Entry struct {
	Path       string
	LastMod    time.Time
	ChangeFreq string
	Priority   float64
	Alternates []Alternate
}

type Alternate struct {
	Lang string
	Href string
}

type Seq = iter.Seq2[Entry, error]

func Of(entries []Entry) Seq {
	return func(yield func(Entry, error) bool) {
		for _, entry := range entries {
			if !yield(entry, nil) {
				return
			}
		}
	}
}

func Join(sequences ...Seq) Seq {
	return func(yield func(Entry, error) bool) {
		for _, sequence := range sequences {
			if sequence == nil {
				continue
			}
			for entry, err := range sequence {
				if !yield(entry, err) {
					return
				}
			}
		}
	}
}

type Derived struct {
	Route  string
	Locale string
	Entry  Entry
}

func FromRoutes(routes []ir.Route, words vocab.Table, settings config.Config, overridden map[string]bool) []Derived {
	var derived []Derived
	for _, route := range routes {
		if skipped(route, settings, overridden) {
			continue
		}
		for _, locale := range Locales(settings) {
			derived = append(derived, Derived{
				Route:  route.Name,
				Locale: locale,
				Entry:  derive(route.Pattern, locale, words, settings),
			})
		}
	}
	sort.Slice(derived, func(i, j int) bool { return derived[i].Entry.Path < derived[j].Entry.Path })
	return derived
}

func skipped(route ir.Route, settings config.Config, overridden map[string]bool) bool {
	if settings.Reserves(route.Pattern) || strings.Contains(route.Pattern, "[") {
		return true
	}
	return overridden[route.Name] || Excluded(route.Pattern, settings)
}

func FromMeta(entry Entry, meta runtime.Meta) (Entry, bool) {
	if Noindex(meta.Robots) {
		return Entry{}, false
	}
	if meta.Canonical != "" {
		entry.Path = meta.Canonical
	}
	if len(meta.Alternates) > 0 {
		entry.Alternates = make([]Alternate, 0, len(meta.Alternates))
		for _, alternate := range meta.Alternates {
			entry.Alternates = append(entry.Alternates, Alternate{Lang: alternate.Lang, Href: alternate.Href})
		}
	}
	return entry, true
}

func Noindex(robots string) bool {
	return strings.Contains(strings.ToLower(robots), "noindex")
}

func Excluded(pattern string, settings config.Config) bool {
	for _, prefix := range settings.SEO.Sitemap.Exclude {
		if pattern == prefix || strings.HasPrefix(pattern, prefix+"/") {
			return true
		}
	}
	return false
}

func Locales(settings config.Config) []string {
	if settings.I18n.Mode != config.ModePath || len(settings.I18n.Locales) < 2 {
		return []string{settings.I18n.DefaultLocale}
	}
	return settings.I18n.Locales
}

func derive(pattern, locale string, words vocab.Table, settings config.Config) Entry {
	entry := Entry{
		Path:       words.Localise(locale, pattern),
		ChangeFreq: settings.SEO.Sitemap.ChangeFreq,
		Priority:   settings.SEO.Sitemap.Priority,
	}
	if len(Locales(settings)) < 2 {
		return entry
	}
	entry.Alternates = Alternates(pattern, words, settings)
	return entry
}

func Alternates(pattern string, words vocab.Table, settings config.Config) []Alternate {
	locales := settings.I18n.Locales
	list := make([]Alternate, 0, len(locales)+1)
	for _, locale := range locales {
		list = append(list, Alternate{Lang: locale, Href: words.Localise(locale, pattern)})
	}
	return append(list, Alternate{
		Lang: DefaultHreflang,
		Href: words.Localise(settings.I18n.DefaultLocale, pattern),
	})
}

func ShardPath(index int) string {
	return SitemapPrefix + strconv.Itoa(index) + SitemapSuffix
}

func ShardIndex(name string) (int, bool) {
	trimmed, found := strings.CutSuffix(name, SitemapSuffix)
	if !found {
		return 0, false
	}
	index, err := strconv.Atoi(trimmed)
	if err != nil || index < 1 {
		return 0, false
	}
	return index, true
}

func Root(entries Seq, origin string, limit int) ([]byte, error) {
	var held []Entry
	total := 0
	for entry, err := range entries {
		if err != nil {
			return nil, err
		}
		total++
		if total <= limit {
			held = append(held, entry)
		}
	}
	if total <= limit {
		return urlset(held, origin), nil
	}
	return index(total, origin, limit), nil
}

func Shard(entries Seq, origin string, limit, shard int) ([]byte, bool, error) {
	held, total, err := take(entries, (shard-1)*limit, limit)
	if err != nil {
		return nil, false, err
	}
	if total == 0 {
		return nil, false, nil
	}
	return urlset(held, origin), true, nil
}

func take(entries Seq, skip, want int) ([]Entry, int, error) {
	var held []Entry
	seen, kept := 0, 0
	for entry, err := range entries {
		if err != nil {
			return nil, 0, err
		}
		seen++
		if seen <= skip {
			continue
		}
		held = append(held, entry)
		kept++
		if kept == want {
			break
		}
	}
	return held, kept, nil
}

func urlset(entries []Entry, origin string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="` + namespace + `" xmlns:xhtml="` + xhtml + `">` + "\n")
	for _, entry := range entries {
		b.WriteString("  <url>\n    <loc>")
		b.WriteString(escapeXML(Absolute(origin, entry.Path)))
		b.WriteString("</loc>\n")
		if !entry.LastMod.IsZero() {
			b.WriteString("    <lastmod>" + escapeXML(entry.LastMod.UTC().Format(time.RFC3339)) + "</lastmod>\n")
		}
		if entry.ChangeFreq != "" {
			b.WriteString("    <changefreq>" + escapeXML(entry.ChangeFreq) + "</changefreq>\n")
		}
		if entry.Priority > 0 {
			b.WriteString("    <priority>" + strconv.FormatFloat(entry.Priority, 'f', 1, 64) + "</priority>\n")
		}
		for _, alternate := range entry.Alternates {
			b.WriteString(`    <xhtml:link rel="alternate" hreflang="`)
			b.WriteString(escapeXML(alternate.Lang))
			b.WriteString(`" href="`)
			b.WriteString(escapeXML(Absolute(origin, alternate.Href)))
			b.WriteString("\"/>\n")
		}
		b.WriteString("  </url>\n")
	}
	b.WriteString("</urlset>\n")
	return []byte(b.String())
}

func index(total int, origin string, limit int) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<sitemapindex xmlns="` + namespace + `">` + "\n")
	for shard := 1; shard <= shards(total, limit); shard++ {
		b.WriteString("  <sitemap>\n    <loc>")
		b.WriteString(escapeXML(Absolute(origin, ShardPath(shard))))
		b.WriteString("</loc>\n  </sitemap>\n")
	}
	b.WriteString("</sitemapindex>\n")
	return []byte(b.String())
}

func shards(total, limit int) int {
	return (total + limit - 1) / limit
}

func Absolute(origin, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return origin + path
}

func Robots(settings config.Config, origin string) []byte {
	var b strings.Builder
	for _, group := range groups(settings) {
		for _, agent := range group.UserAgent {
			b.WriteString("User-agent: " + agent + "\n")
		}
		for _, allow := range group.Allow {
			b.WriteString("Allow: " + allow + "\n")
		}
		for _, disallow := range group.Disallow {
			b.WriteString("Disallow: " + disallow + "\n")
		}
		if group.CrawlDelay > 0 {
			b.WriteString("Crawl-delay: " + strconv.Itoa(group.CrawlDelay) + "\n")
		}
		b.WriteString("\n")
	}
	for _, sitemap := range sitemaps(settings, origin) {
		b.WriteString("Sitemap: " + sitemap + "\n")
	}
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}

func groups(settings config.Config) []config.RobotsGroup {
	if len(settings.SEO.Robots.Groups) > 0 {
		return settings.SEO.Robots.Groups
	}
	return []config.RobotsGroup{{UserAgent: []string{"*"}, Allow: []string{"/"}}}
}

func sitemaps(settings config.Config, origin string) []string {
	var list []string
	if origin != "" && settings.SEO.Sitemap.Enabled() {
		list = append(list, origin+SitemapPath)
	}
	for _, entry := range settings.SEO.Robots.Sitemaps {
		if origin == "" && strings.HasPrefix(entry, "/") {
			continue
		}
		list = append(list, Absolute(origin, entry))
	}
	return list
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

func escapeXML(value string) string {
	return xmlEscaper.Replace(value)
}
