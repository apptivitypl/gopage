package runtime

import "testing"

func TestMetaFields(t *testing.T) {
	meta := Meta{
		Title:       "t",
		Description: "d",
		Canonical:   "c",
		Image:       "i",
		Robots:      "r",
	}
	cases := map[string]string{
		"Title": "t", "Description": "d", "Canonical": "c", "Image": "i", "Robots": "r",
	}
	for field, want := range cases {
		value, ok := meta.Get([]string{field})
		if !ok || value.Str != want {
			t.Errorf("%s = %+v, %v", field, value, ok)
		}
	}
}

func TestMetaRejectsUnknownFields(t *testing.T) {
	if _, ok := (Meta{}).Get([]string{"Nope"}); ok {
		t.Error("Meta must not invent fields")
	}
	if _, ok := (Meta{}).Get(nil); ok {
		t.Error("an empty path resolves to nothing")
	}
	if _, ok := (Meta{}).Get([]string{"Title", "Deeper"}); ok {
		t.Error("meta fields are scalars")
	}
}

func TestWithMetaRoutesByPrefix(t *testing.T) {
	props := Map{"Title": String("from props")}
	wrapped := WithMeta(props, Meta{Title: "from meta"})

	if value, _ := wrapped.Get([]string{"Title"}); value.Str != "from props" {
		t.Errorf("props path = %q", value.Str)
	}
	if value, _ := wrapped.Get([]string{MetaRoot, "Title"}); value.Str != "from meta" {
		t.Errorf("meta path = %q", value.Str)
	}
}

func TestWithMetaToleratesMissingProps(t *testing.T) {
	wrapped := WithMeta(nil, Meta{Title: "t"})
	if _, ok := wrapped.Get([]string{"Anything"}); ok {
		t.Error("without props nothing resolves")
	}
	if value, _ := wrapped.Get([]string{MetaRoot, "Title"}); value.Str != "t" {
		t.Errorf("meta = %q", value.Str)
	}
}

func TestAlternateReadsItsFields(t *testing.T) {
	alternate := Alternate{Lang: "pl", Href: "https://example.com/pl"}
	lang, ok := alternate.Get([]string{"Lang"})
	if !ok || lang.Str != "pl" {
		t.Errorf("lang = %+v", lang)
	}
	href, ok := alternate.Get([]string{"Href"})
	if !ok || href.Str != "https://example.com/pl" {
		t.Errorf("href = %+v", href)
	}
	for _, path := range [][]string{nil, {"Wobble"}, {"Lang", "Extra"}} {
		if _, ok := alternate.Get(path); ok {
			t.Errorf("%v was accepted", path)
		}
	}
}

func TestAlternatesReadAsASequence(t *testing.T) {
	list := Alternates{{Lang: "en"}, {Lang: "pl"}}
	if list.Len() != 2 {
		t.Fatalf("len = %d", list.Len())
	}
	entry := list.At(1).Object()
	if entry == nil {
		t.Fatal("an entry must read as an object")
	}
	lang, _ := entry.Get([]string{"Lang"})
	if lang.Str != "pl" {
		t.Errorf("lang = %+v", lang)
	}
	for _, index := range []int{-1, 2} {
		if got := list.At(index); got.Kind != KindNil {
			t.Errorf("At(%d) = %+v", index, got)
		}
	}
}

func TestMetaExposesItsAlternates(t *testing.T) {
	meta := Meta{Alternates: Alternates{{Lang: "en"}}}
	value, ok := meta.Get([]string{AlternatesField})
	if !ok || value.Sequence().Len() != 1 {
		t.Errorf("value = %+v, ok = %v", value, ok)
	}
}
