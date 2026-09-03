package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/runtime"
)

func renderMeta(t *testing.T, meta runtime.Meta) string {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{
		"app/layout.rill": file("<head>{% meta %}</head>{% outlet %}"),
		"app/page.rill":   file("body"),
	}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	route, _ := result.Manifest.Lookup("/")
	out := runtime.NewBuffer(512)
	props := runtime.WithMeta(runtime.Map{}, meta)
	if err := runtime.Render(result.Manifest.Chain(route), props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestMetaRendersEveryTagItHas(t *testing.T) {
	got := renderMeta(t, runtime.Meta{
		Title:       "Chair",
		Description: "a chair",
		Canonical:   "https://example.com/chair",
		Image:       "https://example.com/chair.png",
		Robots:      "index",
	})
	for _, want := range []string{
		"<title>Chair</title>",
		`<meta name="description" content="a chair">`,
		`<link rel="canonical" href="https://example.com/chair">`,
		`<meta property="og:title" content="Chair">`,
		`<meta property="og:image" content="https://example.com/chair.png">`,
		`<meta name="robots" content="index">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render is missing %s:\n%s", want, got)
		}
	}
}

func TestEmptyMetaFieldsEmitNothing(t *testing.T) {
	got := renderMeta(t, runtime.Meta{Title: "only"})
	if !strings.Contains(got, "<title>only</title>") {
		t.Errorf("render = %q", got)
	}
	for _, unwanted := range []string{"description", "canonical", "og:image", "robots"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("render emits an empty %s:\n%s", unwanted, got)
		}
	}
}

func TestMetaWithNothingSetRendersNothing(t *testing.T) {
	got := renderMeta(t, runtime.Meta{})
	if got != "<head></head>body" {
		t.Errorf("render = %q", got)
	}
}

func TestMetaValuesAreEscaped(t *testing.T) {
	got := renderMeta(t, runtime.Meta{Title: `a "quote" & <tag>`})
	if strings.Contains(got, `<tag>`) || strings.Contains(got, `"quote"`) {
		t.Errorf("render = %q, want the value escaped", got)
	}
}

func TestMetaDirectiveNeedsNoArguments(t *testing.T) {
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.rill": file("{% meta x %}")}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !bag.HasErrors() {
		t.Error("meta takes no arguments")
	}
}
