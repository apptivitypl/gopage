package runtime

import (
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/ir"
)

func apply(t *testing.T, name string, value, argument Value) Value {
	t.Helper()
	id, _, ok := LookupFilter(name)
	if !ok {
		t.Fatalf("no filter named %q", name)
	}
	out, err := ApplyFilter(id, value, argument)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

func TestCaseFilters(t *testing.T) {
	if got := apply(t, "upper", String("ada"), Nil()); got.Str != "ADA" {
		t.Errorf("upper = %q", got.Str)
	}
	if got := apply(t, "lower", String("ADA"), Nil()); got.Str != "ada" {
		t.Errorf("lower = %q", got.Str)
	}
	if got := apply(t, "trim", String("  ada  "), Nil()); got.Str != "ada" {
		t.Errorf("trim = %q", got.Str)
	}
}

func TestDefaultFillsEmptyValues(t *testing.T) {
	if got := apply(t, "default", String(""), String("none")); got.Str != "none" {
		t.Errorf("default = %q", got.Str)
	}
	if got := apply(t, "default", Nil(), String("none")); got.Str != "none" {
		t.Errorf("default = %q", got.Str)
	}
	if got := apply(t, "default", String("ada"), String("none")); got.Str != "ada" {
		t.Errorf("default = %q", got.Str)
	}
	if got := apply(t, "default", Int(0), String("none")); got.Int() != 0 {
		t.Errorf("a zero number is a value, not a gap: %+v", got)
	}
}

func TestTruncateCountsRunes(t *testing.T) {
	if got := apply(t, "truncate", String("zolw"), Int(10)); got.Str != "zolw" {
		t.Errorf("truncate = %q", got.Str)
	}
	got := apply(t, "truncate", String("abcdef"), Int(3))
	if got.Str != "abc"+"…" {
		t.Errorf("truncate = %q", got.Str)
	}
	if got := apply(t, "truncate", String("żółw"), Int(2)); got.Str != "żó…" {
		t.Errorf("truncate = %q", got.Str)
	}
}

func TestTruncateNeedsAWholeNumber(t *testing.T) {
	id, _, _ := LookupFilter("truncate")
	for _, argument := range []Value{String("x"), Int(-1), Nil()} {
		if _, err := ApplyFilter(id, String("abc"), argument); err == nil {
			t.Errorf("%+v was accepted", argument)
		}
	}
}

func TestNumberGroupsDigits(t *testing.T) {
	cases := map[string]Value{
		"1 234 567": Int(1234567),
		"999":       Int(999),
		"-1 234":    Int(-1234),
		"1 234.5":   Float(1234.5),
		"2 345":     Float(2345),
		"ada":       String("ada"),
	}
	for want, value := range cases {
		if got := apply(t, "number", value, Nil()); got.Str != want {
			t.Errorf("number(%+v) = %q, want %q", value, got.Str, want)
		}
	}
}

func TestMoneyFormatsWithACode(t *testing.T) {
	if got := apply(t, "money", Int(1234567), String("PLN")); got.Str != "1 234 567.00 PLN" {
		t.Errorf("money = %q", got.Str)
	}
	if got := apply(t, "money", Float(12.5), String("")); got.Str != "12.50" {
		t.Errorf("money = %q", got.Str)
	}
}

func TestMoneyNeedsANumber(t *testing.T) {
	id, _, _ := LookupFilter("money")
	if _, err := ApplyFilter(id, String("ada"), String("PLN")); err == nil {
		t.Error("money must reject text")
	}
}

func TestUnknownFiltersAreReported(t *testing.T) {
	if _, _, ok := LookupFilter("wobble"); ok {
		t.Error("wobble is not a filter")
	}
	if _, err := ApplyFilter(9999, Nil(), Nil()); err == nil {
		t.Error("an index outside the registry must be reported")
	}
}

func TestFilterNamesCoverTheRegistry(t *testing.T) {
	names := strings.Join(FilterNames(), ",")
	for _, want := range []string{"upper", "lower", "trim", "default", "truncate", "number", "money"} {
		if !strings.Contains(names, want) {
			t.Errorf("names = %q, want %q", names, want)
		}
	}
}

func filterPlan(argument uint32) []*ir.Plan {
	upper, _, _ := LookupFilter("upper")
	plan := &ir.Plan{
		Ops: []ir.Op{{Kind: ir.OpText, A: 0}},
		Exprs: []ir.ExprNode{
			{Kind: ir.ExprFilter, Op: uint8(upper), A: 1, B: argument},
			{Kind: ir.ExprPath, A: 0},
			{Kind: ir.ExprPath, A: 1},
		},
		Paths:    [][]string{{"Name"}, {"Missing"}},
		Capacity: 32,
	}
	return []*ir.Plan{plan}
}

func renderFilter(t *testing.T, chain []*ir.Plan, props Accessible) (string, error) {
	t.Helper()
	out := Acquire(Capacity(chain))
	defer Release(out)
	if err := Render(chain, props, out); err != nil {
		return "", err
	}
	return out.String(), nil
}

func TestAFilterRunsInsideAPlan(t *testing.T) {
	got, err := renderFilter(t, filterPlan(NoArgument), Map{"Name": String("ada")})
	if err != nil || got != "ADA" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestAFilterReportsAFailingInput(t *testing.T) {
	chain := filterPlan(NoArgument)
	chain[0].Exprs[0].A = 2
	if _, err := renderFilter(t, chain, Map{"Name": String("ada")}); err == nil {
		t.Error("a missing path must be reported")
	}
}

func TestAFilterReportsAFailingArgument(t *testing.T) {
	chain := filterPlan(2)
	if _, err := renderFilter(t, chain, Map{"Name": String("ada")}); err == nil {
		t.Error("a missing argument path must be reported")
	}
}

func TestAFilterReportsAnUnknownRegistryEntry(t *testing.T) {
	chain := filterPlan(NoArgument)
	chain[0].Exprs[0].Op = 250
	if _, err := renderFilter(t, chain, Map{"Name": String("ada")}); err == nil {
		t.Error("an unknown filter must be reported")
	}
}

func TestLenCountsListsAndStrings(t *testing.T) {
	cases := []struct {
		value Value
		want  int64
	}{
		{Seq(Values{Int(1), Int(2)}), 2},
		{String("zolw"), 4},
		{String("żółw"), 4},
		{Nil(), 0},
	}
	for _, c := range cases {
		if got := apply(t, "len", c.value, Nil()); got.Int() != c.want {
			t.Errorf("len(%+v) = %d, want %d", c.value, got.Int(), c.want)
		}
	}
}

func TestLenRefusesWhatItCannotCount(t *testing.T) {
	id, _, _ := LookupFilter("len")
	if _, err := ApplyFilter(id, Int(7), Nil()); err == nil {
		t.Error("len must refuse a number")
	}
}
