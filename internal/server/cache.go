package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/apptivitypl/rill/internal/action"
	"github.com/apptivitypl/rill/internal/cache"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

const (
	CacheHeader      = "RILL-Cache"
	PrivateFreshness = "private, no-cache"
)

var privateDirective = []string{PrivateFreshness}

func (a *App) cachedPage(w http.ResponseWriter, r *http.Request, route ir.Route, params Params) {
	if a.cache == nil || !a.cacheable(r, route) {
		a.renderFresh(w, r, route, params, cache.StatusBypass)
		return
	}
	key := a.key(r).String()
	value, status, err := a.cache.Do(key, func(background bool) (cache.Value, cache.Policy, error) {
		recorder := cache.NewRecorder()
		ctx, sink := r.Context(), w
		if background {
			ctx, sink = context.WithoutCancel(ctx), discarded{header: http.Header{}}
		}
		request := r.WithContext(cache.WithRecorder(ctx, recorder))
		body, err := a.renderPageBody(sink, request, route, params)
		if err != nil {
			return cache.Value{}, cache.Policy{}, err
		}
		defer runtime.Release(body)
		return cache.Value{Body: copyOf(body), Tags: recorder.Tags()}, recorder.Policy(), nil
	})
	if err != nil {
		a.failRender(w, r, route, err)
		return
	}
	a.writeBytes(w, r, value.Body, status, value.Policy)
}

func (a *App) renderFresh(w http.ResponseWriter, r *http.Request, route ir.Route, params Params, status cache.Status) {
	recorder := cache.NewRecorder()
	request := r.WithContext(cache.WithRecorder(r.Context(), recorder))
	body, err := a.renderPageBody(w, request, route, params)
	if err != nil {
		a.failRender(w, r, route, err)
		return
	}
	defer runtime.Release(body)
	a.writeBytes(w, r, body.Bytes(), status, recorder.Policy())
}

func (a *App) failRender(w http.ResponseWriter, r *http.Request, route ir.Route, err error) {
	if errors.Is(err, ErrNotFound) {
		a.fail(w, r, ir.FallbackNotFound, http.StatusNotFound)
		return
	}
	a.logger.Error("render failed", "route", route.Name, "error", err)
	a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
}

func (a *App) renderPageBody(w http.ResponseWriter, r *http.Request, route ir.Route, params Params) (*runtime.Buffer, error) {
	props, err := a.pageProps(w, r, route, params)
	if err != nil {
		return nil, err
	}
	return a.renderResolved(route, props, a.fragmentHook(r), LocaleOf(r), a.resolved(r, params, route))
}

func (a *App) writeBytes(w http.ResponseWriter, r *http.Request, body []byte, status cache.Status, policy cache.Policy) {
	vary(w)
	w.Header().Set(CacheHeader, status.String())
	if policy.TTL > 0 {
		w.Header().Set("Cache-Control", Freshness(policy))
	} else {
		keepPrivate(w)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		a.logger.Error("write failed", "path", r.URL.Path, "error", err)
	}
}

func (a *App) cacheable(r *http.Request, route ir.Route) bool {
	if _, ok := a.submit[route.Name]; ok {
		return false
	}
	if action.HasFlash(r) {
		return false
	}
	return true
}

func (a *App) key(r *http.Request) cache.Key {
	key := cache.Key{
		Path:  r.URL.Path,
		Query: r.URL.RawQuery,
		Host:  a.origin(r),
	}
	if !a.config.Reserves(r.URL.Path) {
		key.Locale = LocaleOf(r)
	}
	return key
}

func (a *App) Invalidate(tags ...string) int {
	if a.cache == nil {
		return 0
	}
	return a.cache.Invalidate(tags...)
}

func (a *App) CacheStats() cache.Stats {
	if a.cache == nil {
		return cache.Stats{}
	}
	return a.cache.Stats()
}

func copyOf(body *runtime.Buffer) []byte {
	out := make([]byte, body.Len())
	copy(out, body.Bytes())
	return out
}

type discarded struct {
	header http.Header
}

func (d discarded) Header() http.Header         { return d.header }
func (d discarded) Write(p []byte) (int, error) { return len(p), nil }
func (d discarded) WriteHeader(int)             {}

func Freshness(policy cache.Policy) string {
	if policy.TTL <= 0 {
		return PrivateFreshness
	}
	directive := "public, max-age=" + strconv.Itoa(int(policy.TTL.Seconds()))
	if policy.Stale > 0 {
		directive += ", stale-while-revalidate=" + strconv.Itoa(int(policy.Stale.Seconds()))
	}
	return directive
}
