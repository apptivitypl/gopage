package seo

import (
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
)

type Policy struct {
	ttl     time.Duration
	stale   time.Duration
	tags    []string
	private bool
}

func (p *Policy) Add(recorder *cache.Recorder) {
	if !p.Observe(recorder) {
		return
	}
	policy := recorder.Policy()
	p.ttl = cache.Shorter(p.ttl, policy.TTL)
	p.stale = cache.Shorter(p.stale, policy.Stale)
}

func (p *Policy) Observe(recorder *cache.Recorder) bool {
	if !recorder.Shared() {
		p.private = true
		return false
	}
	p.tags = append(p.tags, recorder.Tags()...)
	return true
}

func (p *Policy) Tags() []string {
	return p.tags
}

func (p *Policy) Resolve(ttl, stale time.Duration) cache.Policy {
	if p.private {
		return cache.Policy{}
	}
	return cache.Policy{TTL: cache.Shorter(p.ttl, ttl), Stale: cache.Shorter(p.stale, stale)}
}
