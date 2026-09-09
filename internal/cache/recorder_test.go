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
	if got := key.String(); got != "11:example.com2:pl11:/listings/16:page=22:v11:0" {
		t.Errorf("key = %q", got)
	}
	if key.String() != (Key{Host: "example.com", Locale: "pl", Path: "/listings/1",
		Query: "page=2", Variant: "v1"}).String() {
		t.Error("the same request must name the same entry")
	}
}

func TestTheLevelSeparatesKeys(t *testing.T) {
	document := Key{Host: "example.com", Path: "/docs/guide"}
	tail := Key{Host: "example.com", Path: "/docs/guide", Level: 1}
	deeper := Key{Host: "example.com", Path: "/docs/guide", Level: 2}
	seen := map[string]int{document.String(): 0, tail.String(): 1, deeper.String(): 2}
	if len(seen) != 3 {
		t.Errorf("keys = %v, want one entry per level", seen)
	}
	if tail.String() != (Key{Host: "example.com", Path: "/docs/guide", Level: 1}).String() {
		t.Error("the same level must name the same entry")
	}
}

func TestNoRequestCanNameAnotherRequestsEntry(t *testing.T) {
	victim := Key{Host: "h", Locale: "en", Path: "/search", Query: "tags=a|b"}
	for _, forged := range []Key{
		{Host: "h", Locale: "en", Path: "/search|tags=a", Query: "b"},
		{Host: "h", Locale: "en", Path: "/search", Query: "tags=a", Variant: "b"},
		{Host: "h", Locale: "en|/search", Path: "tags=a", Query: "b"},
		{Host: "h", Locale: "en", Path: "/a|b"},
		{Host: "h", Locale: "en", Path: "/a", Query: "b"},
	} {
		if forged.String() == victim.String() {
			t.Errorf("%+v names the entry of %+v: %q", forged, victim, victim.String())
		}
	}
	if (Key{Path: "/a|b"}).String() == (Key{Path: "/a", Query: "b"}).String() {
		t.Error("a separator inside a path must not shift the split")
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

func TestMergeTakesTheShorterFreshness(t *testing.T) {
	page := NewRecorder().TTL(time.Hour).Stale(2 * time.Hour).Tag("page")
	layout := NewRecorder().TTL(time.Minute).Stale(time.Minute).Tag("nav")
	page.Merge(layout)
	policy := page.Policy()
	if policy.TTL != time.Minute || policy.Stale != time.Minute {
		t.Errorf("policy = %+v, want the shorter of the two", policy)
	}
	if got := page.Tags(); len(got) != 2 || got[0] != "page" || got[1] != "nav" {
		t.Errorf("tags = %v, want both", got)
	}
}

func TestMergeKeepsTheLongerWhenTheOtherSaysNothing(t *testing.T) {
	page := NewRecorder().TTL(time.Minute)
	page.Merge(NewRecorder())
	if got := page.Policy().TTL; got != time.Minute {
		t.Errorf("ttl = %v", got)
	}
	quiet := NewRecorder()
	quiet.Merge(NewRecorder().TTL(time.Hour))
	if got := quiet.Policy().TTL; got != time.Hour {
		t.Errorf("ttl = %v, want the only freshness anybody named", got)
	}
	quiet.Merge(nil)
}

func TestAPrivateLayoutMakesThePagePrivate(t *testing.T) {
	page := NewRecorder().TTL(time.Hour)
	page.Merge(NewRecorder().Private())
	if page.Shared() {
		t.Error("a private part makes the whole response private")
	}
}

func TestShorterPrefersTheSmallerPositive(t *testing.T) {
	cases := []struct{ current, candidate, want time.Duration }{
		{0, 0, 0},
		{time.Minute, 0, time.Minute},
		{0, time.Minute, time.Minute},
		{time.Hour, time.Minute, time.Minute},
		{time.Minute, time.Hour, time.Minute},
	}
	for _, item := range cases {
		if got := Shorter(item.current, item.candidate); got != item.want {
			t.Errorf("Shorter(%v, %v) = %v, want %v", item.current, item.candidate, got, item.want)
		}
	}
}
