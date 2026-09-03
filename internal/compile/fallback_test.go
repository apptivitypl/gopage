package compile

import (
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/ir"
)

func fallbackApp() fstest.MapFS {
	return fstest.MapFS{
		"app/layout.rill":           &fstest.MapFile{Data: []byte("<main>{% outlet %}</main>")},
		"app/page.rill":             &fstest.MapFile{Data: []byte("<h1>home</h1>")},
		"app/not-found.rill":        &fstest.MapFile{Data: []byte("<p>gone</p>")},
		"app/error.rill":            &fstest.MapFile{Data: []byte("<p>broken</p>")},
		"app/docs/not-found.rill":   &fstest.MapFile{Data: []byte("<p>no such doc</p>")},
		"app/docs/[slug]/page.rill": &fstest.MapFile{Data: []byte("<h1>doc</h1>")},
	}
}

func TestDiscoverFallbacksNamesAndOrders(t *testing.T) {
	found := DiscoverFallbacks(fallbackApp())
	if len(found) != 3 {
		t.Fatalf("found %d fallbacks: %+v", len(found), found)
	}
	want := []struct{ prefix, kind, name string }{
		{"/", "error", "error"},
		{"/", "not-found", "not-found"},
		{"/docs", "not-found", "docs.not-found"},
	}
	for i, expected := range want {
		if found[i].Prefix != expected.prefix || found[i].Kind != expected.kind || found[i].Name != expected.name {
			t.Errorf("fallback %d = %+v, want %+v", i, found[i], expected)
		}
	}
	if len(found[2].Layouts) != 1 || found[2].Layouts[0] != "app/layout.rill" {
		t.Errorf("layouts = %v", found[2].Layouts)
	}
}

func TestFallbacksReachTheManifest(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fallbackApp(), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	if len(result.Manifest.Fallbacks) != 3 {
		t.Fatalf("fallbacks = %+v", result.Manifest.Fallbacks)
	}
	fallback, ok := result.Manifest.Fallback(ir.FallbackNotFound, "/docs/missing")
	if !ok || fallback.Name != "docs.not-found" {
		t.Fatalf("fallback = %+v, ok = %v", fallback, ok)
	}
	chain := result.Manifest.Chain(ir.Route{Plan: fallback.Plan, LayoutChain: fallback.LayoutChain})
	if len(chain) != 2 {
		t.Errorf("chain = %d plans, want the layout plus the page", len(chain))
	}
}

func TestFallbacksAreOptional(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": &fstest.MapFile{Data: []byte("<h1>home</h1>")}}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(result.Manifest.Fallbacks) != 0 {
		t.Errorf("fallbacks = %+v", result.Manifest.Fallbacks)
	}
}
