package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/cookie"
	"github.com/apptivitypl/gopage/internal/logs"
	"github.com/apptivitypl/gopage/internal/redirect"
	"github.com/apptivitypl/gopage/internal/vocab"
)

const (
	LocaleHeader   = "GOPAGE-Locale"
	AssetsHeader   = "GOPAGE-Assets"
	ForwardedProto = "X-Forwarded-Proto"
)

type routingKey struct{}

type routing struct {
	locale string
	path   string
	prefix string
}

type cut struct {
	locale     string
	rest       string
	prefix     string
	misspelled bool
	exempt     bool
}

type routed struct {
	locale string
	rest   string
	spoken string
	prefix string
}

func routingOf(r *http.Request) routing {
	switch held := r.Context().Value(routingKey{}).(type) {
	case routing:
		return held
	case string:
		return routing{locale: held, path: r.URL.Path}
	}
	return routing{}
}

func LocaleOf(r *http.Request) string {
	return routingOf(r).locale
}

func AskedPath(r *http.Request) string {
	return routingOf(r).path
}

func AskedPrefix(r *http.Request) string {
	return routingOf(r).prefix
}

func (a *App) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.config.KnownHost(r.Host) {
			a.logger.Warn("host refused", "host", logs.Line(r.Host), "path", logs.Line(r.URL.Path))
			http.Error(w, "misdirected request", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) reroute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := ""
		if a.hostRules {
			host = config.BareHost(r.Host)
		}
		for _, rule := range a.config.Redirects {
			if rule.Host != "" && rule.Host != host {
				continue
			}
			if target, ok := applyRule(rule.From, rule.To, r.URL.Path); ok {
				a.sendRedirect(w, r, target, rule.Status)
				return
			}
		}
		if target, ok := a.defaultLocaleRedirect(r.URL.Path); ok {
			a.sendRedirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		for _, rewrite := range a.config.Rewrites {
			target, ok := applyRule(rewrite.From, rewrite.To, r.URL.Path)
			if !ok {
				continue
			}
			inside, ok := redirect.Path(target)
			if !ok {
				a.logger.Warn("rewrite refused", "path", logs.Line(r.URL.Path), "target", logs.Line(target))
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			r = withPath(r.Context(), r, inside)
			break
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) sendRedirect(w http.ResponseWriter, r *http.Request, target string, status int) {
	if r.URL.RawQuery != "" && !strings.Contains(target, "?") {
		target += "?" + r.URL.RawQuery
	}
	safe, ok := redirect.Location(target)
	if !ok {
		a.logger.Warn("redirect refused", "path", logs.Line(r.URL.Path), "target", logs.Line(target))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, safe, status)
}

func (a *App) locale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		found := a.route(r.URL.Path)
		if found.spoken != "" {
			a.sendRedirect(w, r, found.spoken, http.StatusMovedPermanently)
			return
		}
		moved := found.rest != r.URL.Path
		ctx := r.Context()
		if moved {
			ctx = context.WithValue(ctx, routingKey{}, routing{locale: found.locale, path: r.URL.Path, prefix: found.prefix})
		} else {
			ctx = context.WithValue(ctx, routingKey{}, found.locale)
		}
		if a.vocab.Localises() {
			ctx = vocab.With(ctx, a.vocab)
		}
		if moved {
			r = withPath(ctx, r, found.rest)
		} else {
			r = r.WithContext(ctx)
		}
		w.Header().Set(LocaleHeader, found.locale)
		next.ServeHTTP(w, r)
	})
}

func (a *App) route(path string) routed {
	found := a.split(path)
	if found.exempt {
		return routed{locale: found.locale, rest: found.rest}
	}
	normalised := vocab.Normalise(found.rest, a.config.Routing.Normalize)
	if normalised != found.rest || found.misspelled {
		spoken := a.vocab.Localise(found.locale, normalised)
		return routed{locale: found.locale, rest: found.rest, spoken: spoken, prefix: found.prefix}
	}
	if !a.vocab.Speaks(found.locale) || a.config.Reserves(found.rest) {
		return routed{locale: found.locale, rest: found.rest, prefix: found.prefix}
	}
	canonical := a.vocab.Canonical(found.locale, found.rest)
	if spoken := a.vocab.Public(found.locale, canonical); spoken != found.rest {
		spoken = a.vocab.Localise(found.locale, canonical)
		return routed{locale: found.locale, rest: found.rest, spoken: spoken, prefix: found.prefix}
	}
	return routed{locale: found.locale, rest: canonical, prefix: found.prefix}
}

func (a *App) split(path string) cut {
	fallback := cut{locale: a.config.I18n.DefaultLocale, rest: path}
	if strings.HasPrefix(path, assets.Prefix) || a.config.Reserves(path) {
		fallback.exempt = true
		return fallback
	}
	if a.config.Routing.Normalize.Any() && a.claims(path) {
		fallback.exempt = true
		return fallback
	}
	if a.config.I18n.Mode != config.ModePath || path == "" || path[0] != '/' {
		return fallback
	}
	for _, locale := range a.config.I18n.Locales {
		if rest, ok := behind(path, locale, false); ok {
			return cut{locale: locale, rest: rest, prefix: path[:len(locale)+1]}
		}
	}
	if !shouted(path) && !a.mixedLocales {
		return fallback
	}
	for _, locale := range a.config.I18n.Locales {
		if rest, ok := behind(path, locale, true); ok {
			return cut{locale: locale, rest: rest, prefix: path[:len(locale)+1], misspelled: true}
		}
	}
	return fallback
}

func behind(path, locale string, fold bool) (string, bool) {
	if len(path) < len(locale)+1 {
		return "", false
	}
	head := path[1 : len(locale)+1]
	spelled := head == locale || (fold && folded(head, locale))
	if !spelled {
		return "", false
	}
	if len(path) == len(locale)+1 {
		return "/", true
	}
	if path[len(locale)+1] != '/' {
		return "", false
	}
	return path[len(locale)+1:], true
}

func shouted(path string) bool {
	for index := 1; index < len(path); index++ {
		if path[index] == '/' {
			return false
		}
		if path[index] >= 'A' && path[index] <= 'Z' {
			return true
		}
	}
	return false
}

func folded(spelled, locale string) bool {
	for index := range len(spelled) {
		if lowered(spelled[index]) != lowered(locale[index]) {
			return false
		}
	}
	return true
}

func lowered(letter byte) byte {
	if letter >= 'A' && letter <= 'Z' {
		return letter + 'a' - 'A'
	}
	return letter
}

func (a *App) defaultLocaleRedirect(path string) (string, bool) {
	if a.config.I18n.Mode != config.ModePath || a.config.I18n.PrefixDefault {
		return "", false
	}
	prefix := "/" + a.config.I18n.DefaultLocale
	switch {
	case path == prefix:
		return "/", true
	case strings.HasPrefix(path, prefix+"/"):
		return strings.TrimPrefix(path, prefix), true
	default:
		return "", false
	}
}

func (a *App) hostLocale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale, _ := a.config.HostLocale(r.Host)
		w.Header().Set(LocaleHeader, locale)
		next.ServeHTTP(w, withLocale(r, locale))
	})
}

func applyRule(from, to, path string) (string, bool) {
	const wildcard = "/*"
	if !strings.HasSuffix(from, wildcard) {
		if path == from {
			return to, true
		}
		return "", false
	}
	prefix := strings.TrimSuffix(from, wildcard)
	if path != prefix && !strings.HasPrefix(path, prefix+"/") {
		return "", false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(path, prefix), "/")
	if !strings.HasSuffix(to, wildcard) {
		return to, true
	}
	target := strings.TrimSuffix(to, wildcard)
	if rest == "" {
		if target == "" {
			return "/", true
		}
		return target, true
	}
	return target + "/" + rest, true
}

func withPath(ctx context.Context, r *http.Request, path string) *http.Request {
	clone := r.Clone(ctx)
	clone.URL.Path = path
	return clone
}

func withLocale(r *http.Request, locale string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), routingKey{}, locale))
}

func withRouting(r *http.Request, held routing) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), routingKey{}, held))
}

type Header struct {
	Name  string
	Value string
}

var securityHeaders = []Header{
	{Name: "X-Content-Type-Options", Value: "nosniff"},
	{Name: "Referrer-Policy", Value: "strict-origin-when-cross-origin"},
}

var securityValues = map[string][]string{
	"X-Content-Type-Options": {"nosniff"},
	"Referrer-Policy":        {"strict-origin-when-cross-origin"},
}

func SecurityHeaders() []Header {
	return securityHeaders
}

func (a *App) limited(next http.Handler) http.Handler {
	limit := a.config.Security.MaxBody()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.ContentLength != 0 {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		for name, value := range securityValues {
			header[name] = value
		}
		opts := cookie.Options{Secure: a.secureCookies()}
		next.ServeHTTP(w, r.WithContext(cookie.With(r.Context(), opts)))
	})
}

func (a *App) crossOrigin(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	for _, origin := range a.config.Security.TrustedOrigins {
		if err := protection.AddTrustedOrigin(origin); err != nil {
			a.logger.Error("trusted origin ignored", "origin", origin, "error", err)
		}
	}
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.logger.Warn("cross-origin request refused",
			"path", r.URL.Path, "method", r.Method, "origin", r.Header.Get("Origin"))
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
	}))
	return protection.Handler(next)
}

func (a *App) secureCookies() bool {
	return a.config.App.Scheme != config.SchemeHTTP
}

func (a *App) https(r *http.Request) bool {
	if scheme := a.config.App.Scheme; scheme != "" {
		return scheme == "https"
	}
	if a.config.Security.TrustedProxy {
		if proto := r.Header.Get(ForwardedProto); proto != "" {
			first, _, _ := strings.Cut(proto, ",")
			return strings.EqualFold(strings.TrimSpace(first), "https")
		}
	}
	return r.TLS != nil
}
