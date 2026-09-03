package compile

import (
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/form"
)

func TestImageReservesItsBox(t *testing.T) {
	html := renderForm(t, `<Image src="/hero.avif" width="1200" height="800" alt="a view" class="hero" />`,
		form.Result{}, "")
	for _, want := range []string{
		`<img src="/hero.avif"`,
		`alt="a view"`,
		`width="1200" height="800"`,
		`loading="lazy" decoding="async"`,
		`class="hero"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html = %q, want %q", html, want)
		}
	}
}

func TestAnEagerImageSaysSo(t *testing.T) {
	html := renderForm(t, `<Image src="/hero.avif" width="10" height="10" alt="" eager />`, form.Result{}, "")
	if !strings.Contains(html, `loading="eager"`) || strings.Contains(html, "eager=") {
		t.Errorf("html = %q", html)
	}
}

func TestAnEmptyAltIsAllowed(t *testing.T) {
	html := renderForm(t, `<Image src="/line.svg" width="4" height="4" alt="" />`, form.Result{}, "")
	if !strings.Contains(html, `alt=""`) {
		t.Errorf("html = %q", html)
	}
}

func TestABoundSourceIsKept(t *testing.T) {
	html := renderForm(t, `<Image :src="form.Values.Cover" width="10" height="10" alt="cover" />`,
		form.Result{Values: map[string]string{"Cover": "/a.avif"}}, "")
	if !strings.Contains(html, `src="/a.avif"`) {
		t.Errorf("html = %q", html)
	}
}

func TestImageDiagnostics(t *testing.T) {
	cases := map[string]string{
		"no width":       `<Image src="/a.avif" height="10" alt="x" />`,
		"no height":      `<Image src="/a.avif" width="10" alt="x" />`,
		"no dimensions":  `<Image src="/a.avif" alt="x" />`,
		"zero width":     `<Image src="/a.avif" width="0" height="10" alt="x" />`,
		"negative":       `<Image src="/a.avif" width="-4" height="10" alt="x" />`,
		"not a number":   `<Image src="/a.avif" width="wide" height="10" alt="x" />`,
		"computed width": `<Image src="/a.avif" :width="form.Values.W" height="10" alt="x" />`,
		"no alt":         `<Image src="/a.avif" width="10" height="10" />`,
	}
	for name, page := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := compilePage(t, page)
			if !hasCode(bag, diag.C316) {
				t.Errorf("diagnostics = %v, want C316", bag.Sorted())
			}
		})
	}
}

func TestImageIsOfferedAsASuggestion(t *testing.T) {
	_, bag := compilePage(t, `<Imag src="/a.avif" />`)
	rendered := ""
	for _, item := range bag.Sorted() {
		rendered += item.Help
	}
	if !strings.Contains(rendered, "Image") {
		t.Errorf("help = %q", rendered)
	}
}
