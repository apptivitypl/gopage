package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
)

func site(body string) fstest.MapFS {
	return app(map[string]string{
		"app/page.gopage":                body,
		"app/about/page.gopage":          "<p>about</p>",
		"app/listings/page.gopage":       "<p>list</p>",
		"app/listings/[id]/page.gopage":  "<p>one</p>",
		"app/docs/[...slug]/page.gopage": "<p>docs</p>",
	})
}

func linkBag(t *testing.T, body string) *diag.Bag {
	t.Helper()
	var bag diag.Bag
	if _, err := Compile(site(body), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return &bag
}

func linkOK(t *testing.T, body string) {
	t.Helper()
	if bag := linkBag(t, body); bag.HasErrors() {
		t.Errorf("%q was rejected: %+v", body, bag.Items())
	}
}

func linkFails(t *testing.T, body string) diag.Diagnostic {
	t.Helper()
	bag := linkBag(t, body)
	for _, d := range bag.Items() {
		if d.Code == diag.C111 {
			return d
		}
	}
	t.Fatalf("%q was accepted, diagnostics: %+v", body, bag.Items())
	return diag.Diagnostic{}
}

func TestLinksToKnownRoutesAreAccepted(t *testing.T) {
	for _, body := range []string{
		`<a href="/">home</a>`,
		`<a href="/about">about</a>`,
		`<a href="/listings">list</a>`,
		`<a href="/listings/{{ A }}">one</a>`,
		`<a href="/listings/42">one</a>`,
		`<a href="/docs/intro">docs</a>`,
		`<a href="/docs/a/b/c">deep</a>`,
		`<a href="/listings?city=x">query</a>`,
		`<a href="/about#top">fragment</a>`,
	} {
		linkOK(t, body)
	}
}

func TestExternalAndSpecialLinksAreLeftAlone(t *testing.T) {
	for _, body := range []string{
		`<a href="https://example.org">out</a>`,
		`<a href="mailto:a@b.c">mail</a>`,
		`<a href="tel:123">call</a>`,
		`<a href="#top">anchor</a>`,
		`<a href="//cdn.example.com/x">protocol relative</a>`,
		`<a href="relative/path">relative</a>`,
		`<a href="">empty</a>`,
	} {
		linkOK(t, body)
	}
}

func TestUnknownLinkIsReported(t *testing.T) {
	d := linkFails(t, `<a href="/listing/9">typo</a>`)
	if !strings.Contains(d.Message, "no route answers /listing/9") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestUnknownLinkSuggestsTheNearestRoute(t *testing.T) {
	d := linkFails(t, `<a href="/abut">typo</a>`)
	if !strings.Contains(d.Help, "/about") {
		t.Errorf("help = %q", d.Help)
	}
}

func TestUnknownLinkMentionsTheEscapeHatch(t *testing.T) {
	d := linkFails(t, `<a href="/totally/unrelated/path">x</a>`)
	if !strings.Contains(d.Help, `href="!/`) {
		t.Errorf("help = %q", d.Help)
	}
}

func TestRawPrefixSkipsTheCheck(t *testing.T) {
	linkOK(t, `<a href="!/legacy/offer.php">old</a>`)
}

func TestRawPrefixIsStrippedFromTheOutput(t *testing.T) {
	got := mustRender(t, `<a href="!/legacy.php">x</a>`, nil)
	if got != `<a href="/legacy.php">x</a>` {
		t.Errorf("render = %q", got)
	}
}

func TestWrongArityIsReported(t *testing.T) {
	linkFails(t, `<a href="/listings/1/2">too deep</a>`)
	linkFails(t, `<a href="/about/extra">too deep</a>`)
}

func TestCatchAllNeedsAtLeastOneSegment(t *testing.T) {
	linkFails(t, `<a href="/docs">bare</a>`)
}

func TestBoundHrefIsNotChecked(t *testing.T) {
	linkOK(t, `<a :href="A">dynamic</a>`)
}

func TestLinksInsideControlFlowAreChecked(t *testing.T) {
	linkFails(t, `{% if A %}<a href="/nope">x</a>{% endif %}`)
	linkFails(t, `{% for x in A %}<a href="/nope">x</a>{% endfor %}`)
}

func TestLinksInsideComponentsAreChecked(t *testing.T) {
	var bag diag.Bag
	if _, err := Compile(app(map[string]string{
		"components/Card/template.gopage": `<a href="/nope">x</a>`,
		"app/page.gopage":                 "<Card />",
	}), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var found bool
	for _, d := range bag.Items() {
		if d.Code == diag.C111 && strings.Contains(d.File, "components/Card") {
			found = true
		}
	}
	if !found {
		t.Errorf("component links must be checked, diagnostics: %+v", bag.Items())
	}
}

func TestDiagnosticSpanCoversTheAttribute(t *testing.T) {
	source := `<a href="/nope">x</a>`
	var bag diag.Bag
	if _, err := Compile(site(source), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, d := range bag.Items() {
		if d.Code != diag.C111 {
			continue
		}
		if got := source[d.Span.Start:d.Span.End]; got != `href="/nope"` {
			t.Errorf("span covers %q", got)
		}
		return
	}
	t.Fatal("no C111 reported")
}

func TestPatternSegments(t *testing.T) {
	if got := patternSegments("/"); got != nil {
		t.Errorf("patternSegments(/) = %v, want none", got)
	}
	if got := patternSegments("/a/b"); len(got) != 2 {
		t.Errorf("patternSegments = %v", got)
	}
}
