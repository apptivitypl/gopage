package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/cookie"
	"github.com/apptivitypl/gopage/internal/redirect"
)

const (
	LocaleHeader   = "GOPAGE-Locale"
	AssetsHeader   = "GOPAGE-Assets"
	ForwardedProto = "X-Forwarded-Proto"
)

type localeKey struct{}

func LocaleOf(r *http.Request) string {
	locale, _ := r.Context().Value(localeKey{}).(string)
	return locale
}

func (a *App) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.config.KnownHost(r.Host) {
			a.logger.Warn("host refused", "host", r.Host, "path", r.URL.Path)
			http.Error(w, "misdirected request", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) reroute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, rule := range a.config.Redirects {
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
				a.logger.Warn("rewrite refused", "path", r.URL.Path, "target", target)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			r = withPath(r, inside)
			break
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) sendRedirect(w http.ResponseWriter, r *http.Request, target string, status int) {
	safe, ok := redirect.Location(target)
	if !ok {
		a.logger.Warn("redirect refused", "path", r.URL.Path, "target", target)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, safe, status)
}

func (a *App) locale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale, rest := a.splitLocale(r.URL.Path)
		if rest != r.URL.Path {
			r = withPath(r, rest)
		}
		w.Header().Set(LocaleHeader, locale)
		next.ServeHTTP(w, withLocale(r, locale))
	})
}

func (a *App) splitLocale(path string) (string, string) {
	if strings.HasPrefix(path, assets.Prefix) || a.config.Reserves(path) {
		return a.config.I18n.DefaultLocale, path
	}
	if a.config.I18n.Mode != config.ModePath {
		return a.config.I18n.DefaultLocale, path
	}
	for _, locale := range a.config.I18n.Locales {
		prefix := "/" + locale
		if path == prefix {
			return locale, "/"
		}
		if strings.HasPrefix(path, prefix+"/") {
			return locale, strings.TrimPrefix(path, prefix)
		}
	}
	return a.config.I18n.DefaultLocale, path
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

func withPath(r *http.Request, path string) *http.Request {
	clone := r.Clone(r.Context())
	clone.URL.Path = path
	return clone
}

func withLocale(r *http.Request, locale string) *http.Request {
	if locale == "" {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), localeKey{}, locale))
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
