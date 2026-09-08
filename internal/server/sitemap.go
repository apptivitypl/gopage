package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
	"github.com/apptivitypl/gopage/internal/seo"
)

var errNoShard = errors.New("no such sitemap shard")

func (a *App) sitemap(w http.ResponseWriter, r *http.Request) {
	ttl, stale := a.config.SEO.Sitemap.Freshness()
	a.document(w, r, seo.SitemapType, ttl, stale, func(r *http.Request, policy *seo.Policy) ([]byte, error) {
		return seo.Root(a.entries(r, policy), a.origin(r), a.config.SEO.Sitemap.Entries())
	})
}

func (a *App) sitemapShard(w http.ResponseWriter, r *http.Request) {
	shard, ok := seo.ShardIndex(r.PathValue("shard"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	ttl, stale := a.config.SEO.Sitemap.Freshness()
	a.document(w, r, seo.SitemapType, ttl, stale, func(r *http.Request, policy *seo.Policy) ([]byte, error) {
		body, found, err := seo.Shard(a.entries(r, policy), a.origin(r), a.config.SEO.Sitemap.Entries(), shard)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errNoShard
		}
		return body, nil
	})
}

func (a *App) robots(w http.ResponseWriter, r *http.Request) {
	ttl, stale := a.config.SEO.Robots.Freshness()
	a.document(w, r, seo.RobotsType, ttl, stale, func(r *http.Request, _ *seo.Policy) ([]byte, error) {
		return seo.Robots(a.config, a.origin(r)), nil
	})
}

type render func(*http.Request, *seo.Policy) ([]byte, error)

func (a *App) document(w http.ResponseWriter, r *http.Request, contentType string, ttl, stale time.Duration, build render) {
	load := func(background bool) (cache.Value, cache.Policy, error) {
		request := r
		if background {
			request = r.Clone(context.WithoutCancel(r.Context()))
		}
		policy := &seo.Policy{}
		body, err := build(request, policy)
		if err != nil {
			return cache.Value{}, cache.Policy{}, err
		}
		return cache.Value{Body: body, Tags: policy.Tags()}, policy.Resolve(ttl, stale), nil
	}
	if a.cache == nil {
		value, policy, err := load(false)
		if err != nil {
			a.failDocument(w, r, err)
			return
		}
		a.writeText(w, r, contentType, value.Body, cache.StatusBypass, policy)
		return
	}
	value, status, err := a.cache.Do(a.key(r).String(), load)
	if err != nil {
		a.failDocument(w, r, err)
		return
	}
	a.writeText(w, r, contentType, value.Body, status, value.Policy)
}

func (a *App) failDocument(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errNoShard) {
		http.NotFound(w, r)
		return
	}
	a.logger.Error("sitemap failed", "path", r.URL.Path, "error", err)
	http.Error(w, "sitemap unavailable", http.StatusInternalServerError)
}

func (a *App) entries(r *http.Request, policy *seo.Policy) seo.Seq {
	return func(yield func(seo.Entry, error) bool) {
		for _, derived := range seo.FromRoutes(a.manifest.Routes, a.vocab, a.config, a.overridden()) {
			entry, keep := a.probe(r, derived, policy)
			if !keep {
				continue
			}
			if !yield(entry, nil) {
				return
			}
		}
		for _, route := range a.manifest.Routes {
			provider, ok := a.sitemaps[route.Name]
			if !ok || seo.Excluded(route.Pattern, a.config) {
				continue
			}
			if !a.provided(r, route, provider, policy, yield) {
				return
			}
		}
	}
}

func (a *App) provided(r *http.Request, route ir.Route, provider SitemapProvider, policy *seo.Policy,
	yield func(seo.Entry, error) bool,
) bool {
	recorder := cache.NewRecorder()
	sequence, err := provider(r.Clone(cache.WithRecorder(r.Context(), recorder)))
	policy.Add(recorder)
	if err != nil {
		a.logger.Error("sitemap route failed", "route", route.Name, "error", err)
		return yield(seo.Entry{}, err)
	}
	if sequence == nil {
		return true
	}
	for entry, err := range sequence {
		if !yield(entry, err) {
			return false
		}
	}
	return true
}

func (a *App) overridden() map[string]bool {
	if len(a.sitemaps) == 0 {
		return nil
	}
	names := make(map[string]bool, len(a.sitemaps))
	for name := range a.sitemaps {
		names[name] = true
	}
	return names
}

func (a *App) probe(r *http.Request, derived seo.Derived, policy *seo.Policy) (seo.Entry, bool) {
	provider, ok := a.meta[derived.Route]
	if !ok || !a.config.SEO.Sitemap.Probes() {
		return derived.Entry, true
	}
	recorder := cache.NewRecorder()
	request := a.probeRequest(r, derived, recorder)
	props, err := a.probeProps(request, derived)
	if err == nil {
		var meta runtime.Meta
		if meta, err = provider(request, Params{}, props); err == nil {
			policy.Observe(recorder)
			return seo.FromMeta(derived.Entry, meta)
		}
	}
	policy.Observe(recorder)
	a.logger.Warn("sitemap probe failed", "route", derived.Route, "locale", derived.Locale, "error", err)
	return derived.Entry, true
}

func (a *App) probeProps(r *http.Request, derived seo.Derived) (runtime.Accessible, error) {
	provider, ok := a.props[derived.Route]
	if !ok {
		return runtime.Empty{}, nil
	}
	return provider(r, Params{})
}

func (a *App) probeRequest(r *http.Request, derived seo.Derived, recorder *cache.Recorder) *http.Request {
	request := r.Clone(cache.WithRecorder(r.Context(), recorder))
	target := *r.URL
	target.Path = derived.Entry.Path
	target.RawQuery = ""
	request.URL = &target
	return withLocale(request, derived.Locale)
}
