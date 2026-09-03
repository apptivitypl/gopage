package cache

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestRecorderCollectsAPolicy(t *testing.T) {
	recorder := NewRecorder().TTL(time.Minute).Stale(time.Hour).Tag("a", "b").Tag("c")
	policy := recorder.Policy()
	if policy.TTL != time.Minute || policy.Stale != time.Hour {
		t.Errorf("policy = %+v", policy)
	}
	if got := recorder.Tags(); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("tags = %v", got)
	}
}

func TestAPrivateRecorderIsNeverCacheable(t *testing.T) {
	recorder := NewRecorder().TTL(time.Minute).Private()
	if recorder.Policy().cacheable() {
		t.Errorf("policy = %+v", recorder.Policy())
	}
}

func TestAnEmptyRecorderReportsNoTags(t *testing.T) {
	if got := NewRecorder().Tags(); got != nil {
		t.Errorf("tags = %v", got)
	}
}

func TestTagsAreCopiedOut(t *testing.T) {
	recorder := NewRecorder().Tag("a")
	tags := recorder.Tags()
	tags[0] = "changed"
	if recorder.Tags()[0] != "a" {
		t.Error("the caller must not be able to edit the recorded tags")
	}
}

func TestTheRecorderTravelsInTheContext(t *testing.T) {
	recorder := NewRecorder()
	ctx := WithRecorder(context.Background(), recorder)
	From(ctx).TTL(time.Minute)
	if recorder.Policy().TTL != time.Minute {
		t.Errorf("policy = %+v", recorder.Policy())
	}
}

func TestAContextWithoutARecorderStillAnswers(t *testing.T) {
	recorder := From(context.Background())
	if recorder == nil {
		t.Fatal("From must never return nil")
	}
	recorder.TTL(time.Minute).Tag("x")
	if !recorder.Policy().cacheable() {
		t.Errorf("policy = %+v", recorder.Policy())
	}
}

func TestKeyIsStableAndSeparated(t *testing.T) {
	key := Key{Host: "example.com", Locale: "pl", Path: "/listings/1", Query: "page=2", Variant: "v1"}
	if got := key.String(); got != "example.com|pl|/listings/1|page=2|v1" {
		t.Errorf("key = %q", got)
	}
	first := Key{Path: "/a|b"}.String()
	second := Key{Path: "/a", Query: "b"}.String()
	if first == second {
		t.Errorf("keys must not collide: %q", first)
	}
}

func TestAPrivateRecorderIsNoLongerShared(t *testing.T) {
	recorder := NewRecorder()
	if !recorder.Shared() {
		t.Error("a fresh recorder is shared")
	}
	recorder.Private()
	if recorder.Shared() {
		t.Error("a private recorder is not shared")
	}
}
