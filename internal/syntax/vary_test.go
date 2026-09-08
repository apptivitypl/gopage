package syntax

import (
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
)

func varied(t *testing.T, source string) (*Document, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	return Parse("page.gopage", source, &bag), &bag
}

func TestVaryReadsACookieAndItsValues(t *testing.T) {
	doc, bag := varied(t, `{% vary cookie="theme" values="light, dark" %}<p>hi</p>`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	if len(doc.Varies) != 1 {
		t.Fatalf("varies = %+v", doc.Varies)
	}
	entry := doc.Varies[0]
	if entry.Kind != VaryCookie || entry.Name != "theme" {
		t.Errorf("entry = %+v", entry)
	}
	if len(entry.Values) != 2 || entry.Values[0] != "light" || entry.Values[1] != "dark" {
		t.Errorf("values = %v", entry.Values)
	}
	bare, _ := varied(t, `{% vary cookie="theme" values="light" %}`)
	if len(bare.Nodes) != 0 {
		t.Errorf("nodes = %+v, want the directive to emit nothing", bare.Nodes)
	}
}

func TestVaryReadsAHeader(t *testing.T) {
	doc, bag := varied(t, `{% vary header="CF-IPCountry" values="PL,DE" %}`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	if doc.Varies[0].Kind != VaryHeader || doc.Varies[0].Name != "CF-IPCountry" {
		t.Errorf("entry = %+v", doc.Varies[0])
	}
}

func TestVaryRejectsWhatItCannotUse(t *testing.T) {
	for _, source := range []string{
		`{% vary cookie="theme" %}`,
		`{% vary values="light" %}`,
		`{% vary cookie="theme" header="X" values="a" %}`,
		`{% vary cookie="theme" values="" %}`,
		`{% vary cookie %}`,
		`{% vary cookie=theme values="a" %}`,
		`{% vary colour="theme" values="a" %}`,
		`{% vary %}`,
	} {
		if _, bag := varied(t, source); !bag.HasErrors() {
			t.Errorf("%s was accepted", source)
		}
	}
}
