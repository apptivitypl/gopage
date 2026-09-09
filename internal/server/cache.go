package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"golang.org/x/sync/singleflight"

	"github.com/apptivitypl/gopage/internal/action"
	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/logs"
	"github.com/apptivitypl/gopage/internal/redirect"
	"github.com/apptivitypl/gopage/internal/reply"
	"github.com/apptivitypl/gopage/internal/runtime"
)

const (
	CacheHeader      = "GOPAGE-Cache"
	PrivateFreshness = "private, no-cache"
)

var privateDirective = []string{PrivateFreshness}

func (a *App) cachedPage(w http.ResponseWriter, r *http.Request, route ir.Route, params Params) {
	a.cachedRender(w, r, route, params, 0)
}

func (a *App) cachedRender(w http.ResponseWriter, r *http.Request, route ir.Route, params Params, level int) {
	if a.cache == nil || !a.cacheable(r, route) {
		a.renderFresh(w, r, route, params, level, cache.StatusBypass)
		return
	}
	key := a.key(r, level).String()
	value, status, err := a.cache.Do(key, func(background bool) (cache.Value, cache.Policy, error) {
		ctx, sink := r.Context(), w
		if background {
			ctx, sink = context.WithoutCancel(ctx), discarded{header: http.Header{}}
		}
		recording, recorder, answer := recording(ctx)
		request := r.WithContext(recording)
		body, title, err := a.renderBody(sink, request, route, params, level)
		if err != nil {
			return cache.Value{}, cache.Policy{}, err
		}
		defer runtime.Release(body)
		answer.Deliver(sink, request, a.secureCookies())
		value := titled(a.valueOf(copyOf(body), recorder, answer), level, title)
		return value, recorder.Policy(), nil
	})
	if err != nil {
		a.failRender(w, r, route, err)
		return
	}
	a.writeBytes(w, r, value, status, level)
}

func titled(value cache.Value, level int, title string) cache.Value {
	if level == 0 {
		return value
	}
	if value.Header == nil {
		value.Header = http.Header{}
	}
	value.Header.Set(TitleHeader, url.QueryEscape(title))
	return value
}

func (a *App) renderFresh(w http.ResponseWriter, r *http.Request, route ir.Route, params Params,
	level int, status cache.Status) {
	recording, recorder, answer := recording(r.Context())
	request := r.WithContext(recording)
	body, title, err := a.renderBody(w, request, route, params, level)
	if err != nil {
		a.failRender(w, r, route, err)
		return
	}
	defer runtime.Release(body)
	answer.Deliver(w, request, a.secureCookies())
	value := titled(a.valueOf(body.Bytes(), recorder, answer), level, title)
	value.Policy = recorder.Policy()
	a.writeBytes(w, r, value, status, level)
}

type recorders struct {
	policy cache.Recorder
	answer reply.Recorder
	shared singleflight.Group
	slot   cache.Slot
}

func recording(ctx context.Context) (context.Context, *cache.Recorder, *reply.Recorder) {
	pair := &recorders{}
	pair.slot = cache.Slot{Policy: &pair.policy, Response: &pair.answer, Shared: &pair.shared}
	return cache.With(ctx, &pair.slot), &pair.policy, &pair.answer
}

func (a *App) valueOf(body []byte, recorder *cache.Recorder, answer *reply.Recorder) cache.Value {
	return cache.Value{
		Body:   body,
		Tags:   recorder.Tags(),
		Status: answer.Code(),
		Header: answer.Headers(),
	}
}

func (a *App) failRender(w http.ResponseWriter, r *http.Request, route ir.Route, err error) {
	var elsewhere *redirect.Error
	if errors.As(err, &elsewhere) {
		a.sendRedirect(w, r, elsewhere.Location, elsewhere.Status)
		return
	}
	if errors.Is(err, ErrNotFound) {
		a.fail(w, r, ir.FallbackNotFound, http.StatusNotFound)
		return
	}
	a.logger.Error("render failed", "route", route.Name, "error", err)
	a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
}

func (a *App) renderBody(w http.ResponseWriter, r *http.Request, route ir.Route,
	params Params, level int) (*runtime.Buffer, string, error) {
	props, layouts, err := a.loadChain(w, r, route, params, level)
	if err != nil {
		return nil, "", err
	}
	if level == 0 {
		body, err := a.renderResolved(route, props, layouts, a.fragmentHook(r), LocaleOf(r), a.resolved(r, params, route))
		return body, "", err
	}
	chain := a.chain(route)
	body := runtime.Acquire(runtime.Capacity(chain))
	opts := a.options(a.fragmentHook(r), LocaleOf(r))
	opts.Layouts = layouts
	opts.Deferred = a.resolved(r, params, route)
	opts.Depth = level
	if err := runtime.RenderOptions(chain[level:], props, body, opts); err != nil {
		runtime.Release(body)
		return nil, "", err
	}
	return body, titleOf(props), nil
}

func (a *App) writeBytes(w http.ResponseWriter, r *http.Request, value cache.Value, status cache.Status, level int) {
	reply.Apply(w, value.Header)
	vary(w)
	if names := BucketsFrom(r.Context()).Headers(); len(names) > 0 {
		reply.AddVary(w, names...)
	}
	personal := a.personal(r)
	if personal {
		reply.AddVary(w, reply.CookieVary)
	}
	w.Header().Set(CacheHeader, status.String())
	switch {
	case value.Policy.TTL <= 0 || personal:
		keepPrivate(w)
	case level > 0:
		w.Header().Set("Cache-Control", VisitorFreshness(value.Policy))
	default:
		w.Header().Set("Cache-Control", Freshness(value.Policy))
	}
	if level > 0 {
		w.Header().Set(LevelHeader, strconv.Itoa(level))
		w.Header().Set("Content-Type", PartialType)
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(value.Body)))
	w.WriteHeader(statusOr(value.Status))
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(value.Body); err != nil {
		a.logger.Error("write failed", "path", logs.Line(r.URL.Path), "error", err)
	}
}

func statusOr(code int) int {
	if code == 0 {
		return http.StatusOK
	}
	return code
}

func (a *App) cacheable(r *http.Request, route ir.Route) bool {
	if _, ok := a.submit[route.Name]; ok {
		return false
	}
	if action.HasFlash(r) {
		return false
	}
	return !a.personal(r)
}

func (a *App) personal(r *http.Request) bool {
	for _, name := range a.config.Security.PrivateCookies {
		if _, err := r.Cookie(name); err == nil {
			return true
		}
	}
	return false
}

func (a *App) key(r *http.Request, level int) cache.Key {
	key := cache.Key{
		Path:  r.URL.Path,
		Query: r.URL.RawQuery,
		Host:  a.origin(r),
		Level: level,
	}
	if !a.config.Reserves(r.URL.Path) {
		key.Locale = LocaleOf(r)
	}
	key.Variant = BucketsFrom(r.Context()).Variant()
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
	return freshness(policy, true)
}

func VisitorFreshness(policy cache.Policy) string {
	return freshness(policy, false)
}

func freshness(policy cache.Policy, shared bool) string {
	if policy.TTL <= 0 {
		return PrivateFreshness
	}
	answer := "private, max-age="
	if shared {
		answer = "public, max-age="
	}
	answer += strconv.Itoa(int(policy.TTL.Seconds()))
	if policy.Stale > 0 {
		answer += ", stale-while-revalidate=" + strconv.Itoa(int(policy.Stale.Seconds()))
	}
	return answer
}
