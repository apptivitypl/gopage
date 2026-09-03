package ir

import (
	"reflect"
	"testing"
)

func TestFallbackRoundTrip(t *testing.T) {
	original := &Manifest{
		Version: Version,
		Plans:   []Plan{{Ops: []Op{{Kind: OpStatic}}, Blob: []byte("x"), Capacity: 4}},
		Fallbacks: []Fallback{
			{Prefix: "/", Name: "not-found", Kind: FallbackNotFound, Plan: 0, LayoutChain: []uint32{0}},
			{Prefix: "/docs", Name: "docs.error", Kind: FallbackError, Plan: 0},
		},
	}
	decoded, err := Decode(Encode(original))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded.Fallbacks, original.Fallbacks) {
		t.Errorf("fallbacks = %+v", decoded.Fallbacks)
	}
}

func TestFallbackKindNames(t *testing.T) {
	if FallbackNotFound.String() != "not-found" || FallbackError.String() != "error" {
		t.Errorf("names = %q, %q", FallbackNotFound, FallbackError)
	}
}

func TestFallbackPrefixMatching(t *testing.T) {
	m := &Manifest{Fallbacks: []Fallback{
		{Prefix: "/docs", Name: "docs", Kind: FallbackNotFound},
		{Prefix: "", Name: "root", Kind: FallbackNotFound},
	}}
	cases := map[string]string{"/docs": "docs", "/docs/deep": "docs", "/docsx": "root", "/": "root"}
	for path, want := range cases {
		fallback, ok := m.Fallback(FallbackNotFound, path)
		if !ok || fallback.Name != want {
			t.Errorf("Fallback(%q) = %+v, ok = %v, want %q", path, fallback, ok, want)
		}
	}
}

func TestFallbackLookupWithoutAMatch(t *testing.T) {
	m := &Manifest{Fallbacks: []Fallback{{Prefix: "/docs", Name: "docs", Kind: FallbackNotFound}}}
	if _, ok := m.Fallback(FallbackNotFound, "/blog"); ok {
		t.Error("a fallback outside the path must not answer")
	}
	if _, ok := m.Fallback(FallbackError, "/docs"); ok {
		t.Error("a not-found page must not answer an error")
	}
}

func TestFragmentRoundTrip(t *testing.T) {
	original := &Manifest{
		Version: Version,
		Plans: []Plan{{
			Fragments: []Fragment{{Name: "reviews", TTL: 300, Stale: 60, Paths: []uint32{0}}},
			Ops:       []Op{{Kind: OpFragment, A: 0, B: 1}},
			Paths:     [][]string{{"Note"}},
			Capacity:  8,
		}},
	}
	decoded, err := Decode(Encode(original))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded.Plans[0].Fragments, original.Plans[0].Fragments) {
		t.Errorf("fragments = %+v", decoded.Plans[0].Fragments)
	}
}

func TestFragmentLookupAndCacheability(t *testing.T) {
	plan := &Plan{Fragments: []Fragment{{Name: "a", TTL: 1}, {Name: "b"}}}
	if fragment, ok := plan.Fragment(0); !ok || !fragment.Cacheable() {
		t.Errorf("fragment = %+v, ok = %v", fragment, ok)
	}
	if fragment, ok := plan.Fragment(1); !ok || fragment.Cacheable() {
		t.Errorf("fragment = %+v, ok = %v", fragment, ok)
	}
	if _, ok := plan.Fragment(9); ok {
		t.Error("an index outside the table must not resolve")
	}
}

func TestFragmentOpHasAName(t *testing.T) {
	if OpFragment.String() != "fragment" {
		t.Errorf("name = %q", OpFragment)
	}
}

func TestCatalogRoundTrip(t *testing.T) {
	original := &Manifest{
		Version:  Version,
		Messages: []string{"hello", "reviews"},
		Catalogs: []Catalog{
			{Locale: "en", Texts: [][PluralForms]string{{"hello"}, {"{count} reviews", "", "{count} review"}}},
			{Locale: "pl", Texts: [][PluralForms]string{{"czesc"}, {"", "", "{count} opinia", "", "{count} opinie"}}},
		},
		Plans: []Plan{{Messages: []string{"hello"}, Ops: []Op{{Kind: OpText}}, Capacity: 8}},
	}
	decoded, err := Decode(Encode(original))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded.Catalogs, original.Catalogs) {
		t.Errorf("catalogs = %+v", decoded.Catalogs)
	}
	if !reflect.DeepEqual(decoded.Messages, original.Messages) {
		t.Errorf("messages = %+v", decoded.Messages)
	}
	if !reflect.DeepEqual(decoded.Plans[0].Messages, original.Plans[0].Messages) {
		t.Errorf("plan messages = %+v", decoded.Plans[0].Messages)
	}
}

func TestCatalogLookup(t *testing.T) {
	m := &Manifest{Catalogs: []Catalog{
		{Locale: "en", Texts: [][PluralForms]string{{"other text", "", "one text"}}},
	}}
	catalog, ok := m.Catalog("en")
	if !ok {
		t.Fatal("en must resolve")
	}
	if _, ok := m.Catalog("pl"); ok {
		t.Error("an unlisted locale must not resolve")
	}
	if text, ok := catalog.Text(0, 2); !ok || text != "one text" {
		t.Errorf("text = %q, ok = %v", text, ok)
	}
	if text, ok := catalog.Text(0, 4); !ok || text != "other text" {
		t.Errorf("an undeclared form falls back to other: %q", text)
	}
	for _, bad := range [][2]int{{9, 0}, {0, -1}, {0, PluralForms}} {
		if _, ok := catalog.Text(uint32(bad[0]), bad[1]); ok {
			t.Errorf("Text(%d, %d) resolved", bad[0], bad[1])
		}
	}
	empty := &Catalog{Texts: [][PluralForms]string{{}}}
	if _, ok := empty.Text(0, 0); ok {
		t.Error("an empty entry resolves to nothing")
	}
}

func TestPlanMessageLookup(t *testing.T) {
	plan := &Plan{Messages: []string{"hello"}}
	if plan.Message(0) != "hello" || plan.Message(9) != "" {
		t.Errorf("message = %q, %q", plan.Message(0), plan.Message(9))
	}
}

func TestMessageExpressionHasAName(t *testing.T) {
	if ExprMessage.String() != "message" {
		t.Errorf("name = %q", ExprMessage)
	}
}

func TestDecodeRejectsTruncationInsideACatalog(t *testing.T) {
	original := &Manifest{
		Version:  Version,
		Messages: []string{"hello", "bye"},
		Catalogs: []Catalog{
			{Locale: "en", Texts: [][PluralForms]string{{"hi", "", "one"}, {"bye"}}},
			{Locale: "pl", Texts: [][PluralForms]string{{"czesc", "", "jedna"}, {"pa"}}},
		},
	}
	full := Encode(original)
	for cut := len(magic); cut < len(full); cut++ {
		if _, err := Decode(full[:cut]); err == nil {
			t.Fatalf("Decode accepted %d of %d bytes", cut, len(full))
		}
	}
	decoded, err := Decode(full)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded.Catalogs, original.Catalogs) {
		t.Errorf("catalogs = %+v", decoded.Catalogs)
	}
}
