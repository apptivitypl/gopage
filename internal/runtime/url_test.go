package runtime

import (
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/ir"
)

func TestSafeURLAcceptsWhatALinkNeeds(t *testing.T) {
	for _, raw := range []string{
		"", "/", "/listings/7", "listings/7", "./a", "../a", "#anchor", "?q=1",
		"http://example.test/x", "HTTPS://example.test/x", "//example.test/x",
		"mailto:someone@example.test", "tel:+48123456789",
		"/a:b", "a.png 1x, b.png 2x", "5:00",
	} {
		if !SafeURL(raw) {
			t.Errorf("SafeURL(%q) = false, want true", raw)
		}
	}
}

func TestSafeURLRejectsASchemeThatRuns(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		"  javascript:alert(1)",
		"java\tscript:alert(1)",
		"java\nscript:alert(1)",
		"java\rscript:alert(1)",
		"jav\x00ascript:alert(1)",
		"vbscript:msgbox(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"data:image/svg+xml,<svg onload=alert(1)>",
	} {
		if SafeURL(raw) {
			t.Errorf("SafeURL(%q) = true, want false", raw)
		}
	}
}

func TestWriteURLDropsARejectedValue(t *testing.T) {
	var b Buffer
	b.WriteURL("javascript:alert(1)")
	if got := string(b.Bytes()); got != "" {
		t.Errorf("buffer = %q, want nothing written", got)
	}
	b.WriteURL("/a?x=1&y=2")
	if got := string(b.Bytes()); got != "/a?x=1&amp;y=2" {
		t.Errorf("buffer = %q, want the accepted url escaped", got)
	}
}

func TestALoneColonIsNotAScheme(t *testing.T) {
	for _, raw := range []string{":", ":relative", "://example.test"} {
		if !SafeURL(raw) {
			t.Errorf("SafeURL(%q) = false, want true", raw)
		}
	}
}

func TestRenderFiltersAURLOp(t *testing.T) {
	plan := planOf(`<a href="">`, []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 9},
		{Kind: ir.OpURL, A: 0},
		{Kind: ir.OpStatic, A: 9, B: 2},
	}, [][]string{{"Href"}})

	safe := render(t, []*ir.Plan{plan}, Map{"Href": String("/listings/7")})
	if safe != `<a href="/listings/7">` {
		t.Errorf("render = %q", safe)
	}
	unsafe := render(t, []*ir.Plan{plan}, Map{"Href": String("javascript:alert(1)")})
	if unsafe != `<a href="">` {
		t.Errorf("render = %q, want the scheme dropped", unsafe)
	}
}

func TestRenderRejectsADanglingURLExpression(t *testing.T) {
	plan := planOf("", []ir.Op{{Kind: ir.OpURL, A: 7}}, nil)
	err := Render([]*ir.Plan{plan}, Map{}, NewBuffer(8))
	if err == nil || !strings.Contains(err.Error(), "not in the plan") {
		t.Errorf("err = %v", err)
	}
}
