package seo

import (
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
)

func TestAnEmptyPolicyFallsBackToTheConfiguredCeiling(t *testing.T) {
	var policy Policy
	got := policy.Resolve(time.Hour, 24*time.Hour)
	if got.TTL != time.Hour || got.Stale != 24*time.Hour {
		t.Errorf("policy = %+v", got)
	}
	if policy.Tags() != nil {
		t.Errorf("tags = %v, want none", policy.Tags())
	}
}

func TestTheShortestProviderWins(t *testing.T) {
	var policy Policy
	policy.Add(cache.NewRecorder().TTL(30 * time.Minute).Stale(2 * time.Hour).Tag("listing"))
	policy.Add(cache.NewRecorder().TTL(5 * time.Minute).Tag("offers"))
	policy.Add(cache.NewRecorder())
	got := policy.Resolve(time.Hour, 24*time.Hour)
	if got.TTL != 5*time.Minute || got.Stale != 2*time.Hour {
		t.Errorf("policy = %+v, want the shortest of each", got)
	}
	if len(policy.Tags()) != 2 {
		t.Errorf("tags = %v, want both", policy.Tags())
	}
}

func TestACeilingShorterThanTheProviderStillWins(t *testing.T) {
	var policy Policy
	policy.Add(cache.NewRecorder().TTL(time.Hour))
	if got := policy.Resolve(time.Minute, 0); got.TTL != time.Minute {
		t.Errorf("policy = %+v", got)
	}
}

func TestAPrivateProviderMakesTheDocumentUncacheable(t *testing.T) {
	var policy Policy
	policy.Add(cache.NewRecorder().TTL(time.Hour).Tag("listing"))
	policy.Add(cache.NewRecorder().Private())
	got := policy.Resolve(time.Hour, time.Hour)
	if got.TTL != 0 || got.Stale != 0 {
		t.Errorf("policy = %+v, want nothing cached", got)
	}
}
