package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/apptivitypl/rill/internal/config"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
	"github.com/apptivitypl/rill/internal/seo"
)

const defaultHreflang = "x-default"

func (a *App) seo(meta runtime.Meta, r *http.Request, route ir.Route) runtime.Meta {
	if a.config.Reserves(r.URL.Path) {
		return meta
	}
	locales := a.config.I18n.Locales
	if len(locales) < 2 && meta.Canonical != "" {
		return meta
	}
	origin := a.origin(r)
	if meta.Canonical == "" {
		meta.Canonical = origin + a.localised(r.URL.Path, LocaleOf(r))
	}
	if len(locales) < 2 || len(meta.Alternates) > 0 {
		return meta
	}
	meta.Alternates = a.alternates(r.URL.Path, route, origin, a.scheme(r))
	return meta
}

func (a *App) alternates(path string, route ir.Route, origin, scheme string) runtime.Alternates {
	if len(ParamsOf(route.Pattern)) > 0 {
		return nil
	}
	list := make(runtime.Alternates, 0, len(a.config.I18n.Locales)+1)
	for _, locale := range a.config.I18n.Locales {
		list = append(list, runtime.Alternate{Lang: locale, Href: a.hrefFor(path, locale, origin, scheme)})
	}
	list = append(list, runtime.Alternate{
		Lang: defaultHreflang,
		Href: a.hrefFor(path, a.config.I18n.DefaultLocale, origin, scheme),
	})
	return list
}

func (a *App) hrefFor(path, locale, origin, scheme string) string {
	if a.config.I18n.Mode == config.ModeSubdomain {
		return a.hostFor(locale, origin, scheme) + path
	}
	return origin + a.localised(path, locale)
}

func (a *App) localised(path, locale string) string {
	if a.config.I18n.Mode != config.ModePath {
		return path
	}
	if locale == a.config.I18n.DefaultLocale && !a.config.I18n.PrefixDefault {
		return path
	}
	if path == "/" {
		return "/" + locale
	}
	return "/" + locale + path
}

func (a *App) hostFor(locale, origin, scheme string) string {
	for _, host := range a.config.Hosts {
		if host.Locale == locale {
			return scheme + "://" + host.Pattern
		}
	}
	return origin
}

func (a *App) scheme(r *http.Request) string {
	if a.https(r) {
		return "https"
	}
	return "http"
}

func (a *App) origin(r *http.Request) string {
	if host := a.canonicalHost(); host != "" {
		return a.scheme(r) + "://" + host
	}
	host := hostName(r.Host)
	if host == "" {
		return ""
	}
	return a.scheme(r) + "://" + host
}

func (a *App) canonicalHost() string {
	for _, host := range a.config.Hosts {
		if host.Default {
			return host.Pattern
		}
	}
	return a.config.App.CanonicalHost
}

func hostName(host string) string {
	host = strings.TrimSpace(host)
	for index := range len(host) {
		letter := host[index]
		switch {
		case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z':
		case letter >= '0' && letter <= '9':
		case letter == '.', letter == '-', letter == ':', letter == '[', letter == ']':
		default:
			return ""
		}
	}
	return host
}

func ParamsOf(pattern string) []string {
	var names []string
	for segment := range strings.SplitSeq(strings.Trim(pattern, "/"), "/") {
		if strings.HasPrefix(segment, "[") {
			names = append(names, strings.Trim(segment, "[.]"))
		}
	}
	return names
}

func (a *App) sitemap(w http.ResponseWriter, r *http.Request) {
	pages := seo.Pages(a.manifest.Routes, a.config, a.origin(r))
	a.writeText(w, r, seo.SitemapType, seo.Sitemap(pages))
}

func (a *App) robots(w http.ResponseWriter, r *http.Request) {
	a.writeText(w, r, seo.RobotsType, seo.Robots(a.origin(r)))
}

func (a *App) writeText(w http.ResponseWriter, r *http.Request, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		a.logger.Error("write failed", "path", r.URL.Path, "error", err)
	}
}
