package runtime

import (
	"testing"

	"github.com/apptivitypl/gopage/internal/i18n"
	"github.com/apptivitypl/gopage/internal/ir"
)

func messageChain(count uint32) []*ir.Plan {
	return []*ir.Plan{{
		Messages: []string{"reviews"},
		Ops:      []ir.Op{{Kind: ir.OpText, A: 0}},
		Exprs: []ir.ExprNode{
			{Kind: ir.ExprMessage, A: 0, B: count},
			{Kind: ir.ExprPath, A: 0},
		},
		Paths:    [][]string{{"Count"}},
		Capacity: 32,
	}}
}

func renderMessage(t *testing.T, chain []*ir.Plan, props Accessible, opts Options) (string, error) {
	t.Helper()
	out := Acquire(Capacity(chain))
	defer Release(out)
	if err := RenderOptions(chain, props, out, opts); err != nil {
		return "", err
	}
	return out.String(), nil
}

func polishCatalog() *ir.Catalog {
	return &ir.Catalog{Locale: "pl", Texts: [][ir.PluralForms]string{
		{"", "", "{count} opinia", "", "{count} opinie", "{count} opinii"},
	}}
}

func TestAMessagePicksTheFormForItsCount(t *testing.T) {
	opts := Options{Catalog: polishCatalog(), Plural: i18n.RuleFor("pl")}
	cases := map[int64]string{1: "1 opinia", 3: "3 opinie", 7: "7 opinii"}
	for count, want := range cases {
		got, err := renderMessage(t, messageChain(1), Map{"Count": Int(count)}, opts)
		if err != nil || got != want {
			t.Errorf("count %d = %q, err = %v, want %q", count, got, err, want)
		}
	}
}

func TestAMessageWithoutACountUsesTheOtherForm(t *testing.T) {
	catalog := &ir.Catalog{Texts: [][ir.PluralForms]string{{"opinie"}}}
	got, err := renderMessage(t, messageChain(NoArgument), Empty{}, Options{Catalog: catalog})
	if err != nil || got != "opinie" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestAMessageWithoutACatalogShowsItsKey(t *testing.T) {
	got, err := renderMessage(t, messageChain(NoArgument), Empty{}, Options{})
	if err != nil || got != "reviews" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestAMessageWithoutAPluralRuleUsesTheOtherForm(t *testing.T) {
	catalog := &ir.Catalog{Texts: [][ir.PluralForms]string{{"{count} opinii", "", "{count} opinia"}}}
	got, err := renderMessage(t, messageChain(1), Map{"Count": Int(1)}, Options{Catalog: catalog})
	if err != nil || got != "1 opinii" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestACountThatIsNotANumberCountsAsZero(t *testing.T) {
	opts := Options{Catalog: polishCatalog(), Plural: i18n.RuleFor("pl")}
	got, err := renderMessage(t, messageChain(1), Map{"Count": String("many")}, opts)
	if err != nil || got != "many opinii" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestAFractionalCountIsPrintedInFull(t *testing.T) {
	catalog := &ir.Catalog{Texts: [][ir.PluralForms]string{{"{count} kg"}}}
	opts := Options{Catalog: catalog, Plural: i18n.RuleFor("en")}
	got, err := renderMessage(t, messageChain(1), Map{"Count": Float(1.5)}, opts)
	if err != nil || got != "1.5 kg" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestAFailingCountStopsTheRender(t *testing.T) {
	chain := messageChain(1)
	chain[0].Paths = [][]string{{"Missing"}}
	if _, err := renderMessage(t, chain, Empty{}, Options{}); err == nil {
		t.Error("a count that cannot be read must be reported")
	}
}

func TestAMessageOutsideTheTableIsEmpty(t *testing.T) {
	chain := messageChain(NoArgument)
	chain[0].Exprs[0].A = 9
	got, err := renderMessage(t, chain, Empty{}, Options{})
	if err != nil || got != "" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}
