package seo

import (
	"sort"
	"strings"

	"github.com/apptivitypl/rill/internal/config"
	"github.com/apptivitypl/rill/internal/ir"
)

const (
	SitemapPath = "/sitemap.xml"
	RobotsPath  = "/robots.txt"
	SitemapType = "application/xml"
	RobotsType  = "text/plain; charset=utf-8"
	namespace   = "http://www.sitemaps.org/schemas/sitemap/0.9"
	xhtml       = "http://www.w3.org/1999/xhtml"
)

type Page struct {
	Path       string
	Alternates []Alternate
}

type Alternate struct {
	Lang string
	Href string
}

func Pages(routes []ir.Route, settings config.Config, origin string) []Page {
	var pages []Page
	for _, route := range listed(routes, settings) {
		for _, locale := range locales(settings) {
			pages = append(pages, page(route.Pattern, locale, settings, origin))
		}
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Path < pages[j].Path })
	return pages
}

func listed(routes []ir.Route, settings config.Config) []ir.Route {
	var out []ir.Route
	for _, route := range routes {
		if settings.Reserves(route.Pattern) || strings.Contains(route.Pattern, "[") {
			continue
		}
		out = append(out, route)
	}
	return out
}

func locales(settings config.Config) []string {
	if settings.I18n.Mode != config.ModePath || len(settings.I18n.Locales) < 2 {
		return []string{settings.I18n.DefaultLocale}
	}
	return settings.I18n.Locales
}

func page(pattern, locale string, settings config.Config, origin string) Page {
	entry := Page{Path: origin + localised(pattern, locale, settings)}
	if len(locales(settings)) < 2 {
		return entry
	}
	for _, other := range settings.I18n.Locales {
		entry.Alternates = append(entry.Alternates, Alternate{
			Lang: other,
			Href: origin + localised(pattern, other, settings),
		})
	}
	return entry
}

func localised(pattern, locale string, settings config.Config) string {
	if settings.I18n.Mode != config.ModePath {
		return pattern
	}
	if locale == settings.I18n.DefaultLocale && !settings.I18n.PrefixDefault {
		return pattern
	}
	if pattern == "/" {
		return "/" + locale
	}
	return "/" + locale + pattern
}

func Sitemap(pages []Page) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="` + namespace + `" xmlns:xhtml="` + xhtml + `">` + "\n")
	for _, entry := range pages {
		b.WriteString("  <url>\n    <loc>")
		b.WriteString(escapeXML(entry.Path))
		b.WriteString("</loc>\n")
		for _, alternate := range entry.Alternates {
			b.WriteString(`    <xhtml:link rel="alternate" hreflang="`)
			b.WriteString(escapeXML(alternate.Lang))
			b.WriteString(`" href="`)
			b.WriteString(escapeXML(alternate.Href))
			b.WriteString("\"/>\n")
		}
		b.WriteString("  </url>\n")
	}
	b.WriteString("</urlset>\n")
	return []byte(b.String())
}

func Robots(origin string) []byte {
	var b strings.Builder
	b.WriteString("User-agent: *\nAllow: /\n")
	if origin != "" {
		b.WriteString("\nSitemap: " + origin + SitemapPath + "\n")
	}
	return []byte(b.String())
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
