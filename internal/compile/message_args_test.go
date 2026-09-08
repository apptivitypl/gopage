package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
)

func spoke(t *testing.T, template, catalog string) (string, *diag.Bag) {
	t.Helper()
	files := app(map[string]string{"app/page.gopage": template})
	files["locales/en.json"] = &fstest.MapFile{Data: []byte(catalog)}
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(result.Manifest.Catalogs) == 0 {
		return "", &bag
	}
	return result.Manifest.Catalogs[0].Texts[0][0], &bag
}

func TestAMessageTakesNamedArguments(t *testing.T) {
	text, bag := spoke(t,
		`{{ t("jobs.in_city", city = "Warszawa") }}`,
		`{"jobs": {"in_city": "Praca w {city}"}}`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if text != "Praca w {city}" {
		t.Errorf("catalog text = %q", text)
	}
}

func TestAMissingArgumentIsReported(t *testing.T) {
	_, bag := spoke(t, `{{ t("jobs.in_city") }}`, `{"jobs": {"in_city": "Praca w {city}"}}`)
	item, ok := found(bag, diag.C604)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Help, "city = …") {
		t.Errorf("help = %q", item.Help)
	}
}

func TestAnArgumentTheMessageDoesNotUseIsReported(t *testing.T) {
	_, bag := spoke(t, `{{ t("jobs.all", city = "Warszawa") }}`, `{"jobs": {"all": "Wszystkie oferty"}}`)
	if _, ok := found(bag, diag.C604); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestCountKeepsItsOwnPlaceholder(t *testing.T) {
	_, bag := spoke(t,
		`{{ t("jobs.count", count = 3) }}`,
		`{"jobs": {"count": {"one": "{count} oferta", "other": "{count} ofert"}}}`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}
