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
	if !recorder.Shared() {
		p.private = true
		return
	}
	policy := recorder.Policy()
	p.ttl = shorter(p.ttl, policy.TTL)
	p.stale = shorter(p.stale, policy.Stale)
	p.tags = append(p.tags, recorder.Tags()...)
}

func (p *Policy) Tags() []string {
	return p.tags
}

func (p *Policy) Resolve(ttl, stale time.Duration) cache.Policy {
	if p.private {
		return cache.Policy{}
	}
	return cache.Policy{TTL: shorter(p.ttl, ttl), Stale: shorter(p.stale, stale)}
}

func shorter(current, candidate time.Duration) time.Duration {
	if candidate <= 0 {
		return current
	}
	if current <= 0 || candidate < current {
		return candidate
	}
	return current
}
