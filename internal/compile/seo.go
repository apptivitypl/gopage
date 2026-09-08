package compile

import (
	"fmt"
	"strings"

	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/seo"
)

func seoPhase(s *state) error {
	reportClaims(s)
	reportRules(s)
	return nil
}

func reportClaims(s *state) {
	for _, route := range s.routes {
		if builtin(route.Pattern, s.config) {
			s.bag.Add(diag.New(diag.C112, route.File, diag.Span{},
				fmt.Sprintf("%s answers %s, which gopage serves itself", route.File, route.Pattern)).
				WithHelp(yield(route.Pattern)))
		}
	}
	public, err := assets.Public(s.fsys)
	if err != nil {
		return
	}
	for _, asset := range public {
		if builtin(asset.Path, s.config) {
			s.bag.Add(diag.New(diag.C112, asset.Source, diag.Span{},
				fmt.Sprintf("%s is served at %s, which gopage serves itself", asset.Source, asset.Path)).
				WithHelp(yield(asset.Path)))
		}
	}
}

func builtin(path string, settings config.Config) bool {
	if path == seo.RobotsPath {
		return settings.SEO.Robots.Enabled()
	}
	if path == seo.SitemapPath || strings.HasPrefix(path, seo.SitemapPrefix) {
		return settings.SEO.Sitemap.Enabled()
	}
	return false
}

func yield(path string) string {
	if path == seo.RobotsPath {
		return `set "seo": {"robots": {"mode": "off"}} to answer /robots.txt yourself`
	}
	return `set "seo": {"sitemap": {"mode": "off"}} to answer /sitemap.xml yourself`
}

func reportRules(s *state) {
	rules := map[string][]string{"seo.sitemap.exclude": s.config.SEO.Sitemap.Exclude}
	for _, group := range s.config.SEO.Robots.Groups {
		rules["seo.robots allow"] = append(rules["seo.robots allow"], group.Allow...)
		rules["seo.robots disallow"] = append(rules["seo.robots disallow"], group.Disallow...)
	}
	for _, field := range []string{"seo.sitemap.exclude", "seo.robots allow", "seo.robots disallow"} {
		for _, rule := range rules[field] {
			if wildcarded(rule) || served(rule, s.routes, s.config) {
				continue
			}
			s.bag.Add(diag.Warn(diag.W704, config.FileName, diag.Span{},
				fmt.Sprintf("%s names %s, which no route answers", field, rule)).
				WithHelp("write the path a route actually serves, or drop the rule"))
		}
	}
}

func wildcarded(rule string) bool {
	return strings.ContainsAny(rule, "*$?")
}

func served(rule string, routes []Route, settings config.Config) bool {
	if rule == "/" || settings.Reserves(rule) || strings.HasPrefix(rule, assets.Prefix) {
		return true
	}
	trimmed := unprefixed(rule, settings)
	for _, route := range routes {
		if route.Pattern == trimmed || strings.HasPrefix(route.Pattern, trimmed+"/") {
			return true
		}
	}
	return false
}

func unprefixed(path string, settings config.Config) string {
	for _, locale := range settings.I18n.Locales {
		prefix := "/" + locale
		if path == prefix {
			return "/"
		}
		if rest, found := strings.CutPrefix(path, prefix+"/"); found {
			return "/" + rest
		}
	}
	return path
}
