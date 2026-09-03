package compile

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sonquer/rill/internal/diag"
)

func classesOf(t *testing.T, page string) (Result, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": &fstest.MapFile{Data: []byte(page)}}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return result, &bag
}

func TestLiteralClassesLandInTheInventory(t *testing.T) {
	result, bag := classesOf(t, `<div class="card  shadow">x</div><p class="lead">y</p>`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	if !slices.Equal(result.Classes, []string{"card", "lead", "shadow"}) {
		t.Errorf("classes = %v", result.Classes)
	}
}

func TestClassMapKeysLandInTheInventory(t *testing.T) {
	result, bag := classesOf(t, `---
type Props struct {
	Wide bool
}
---
<div :class="{ 'card': true, 'card-wide': Wide }">x</div>`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	if !slices.Equal(result.Classes, []string{"card", "card-wide"}) {
		t.Errorf("classes = %v", result.Classes)
	}
}

func TestBuiltinClassesAreInventoried(t *testing.T) {
	result, _ := classesOf(t, `<Form><Field name="Email" /></Form>`)
	for _, want := range []string{"field", "field-error"} {
		if !slices.Contains(result.Classes, want) {
			t.Errorf("classes = %v, want %q", result.Classes, want)
		}
	}
}

func TestARuntimeClassWarns(t *testing.T) {
	cases := map[string]string{
		"interpolated": `---
type Props struct {
	Tone string
}
---
<span class="badge badge-{{ Tone }}">x</span>`,
		"bound": `---
type Props struct {
	Tone string
}
---
<span :class="Tone">x</span>`,
	}
	for name, page := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := classesOf(t, page)
			if bag.HasErrors() {
				t.Fatalf("a runtime class is a warning, not an error: %v", bag.Sorted())
			}
			if !hasCode(bag, diag.W703) {
				t.Errorf("diagnostics = %v, want W703", bag.Sorted())
			}
			if bag.Sorted()[0].Severity != diag.Warning {
				t.Errorf("severity = %v", bag.Sorted()[0].Severity)
			}
		})
	}
}

func TestAWarningPointsAtTheComponentThatCausedIt(t *testing.T) {
	var bag diag.Bag
	_, err := Compile(fstest.MapFS{
		"components/Badge/props.go":      &fstest.MapFile{Data: []byte("package badge\n\ntype Props struct {\n\tTone string\n}\n")},
		"components/Badge/template.rill": &fstest.MapFile{Data: []byte(`<span class="badge badge-{{ Tone }}">x</span>`)},
		"app/page.rill":                  &fstest.MapFile{Data: []byte(`<Badge Tone="ok" /><Badge Tone="warn" />`)},
	}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	items := bag.Sorted()
	if len(items) != 1 {
		t.Fatalf("diagnostics = %v, want one warning for one bad line", items)
	}
	if items[0].File != "components/Badge/template.rill" {
		t.Errorf("file = %q, want the component that owns the line", items[0].File)
	}
}

func TestTheInventoryIsWrittenOnePerLine(t *testing.T) {
	if got := Inventory([]string{"card", "lead"}); got != "card\nlead\n" {
		t.Errorf("inventory = %q", got)
	}
	if got := Inventory(nil); got != "" {
		t.Errorf("inventory = %q", got)
	}
}

func TestAClassAttributeWithoutAValueIsNoted(t *testing.T) {
	result, _ := classesOf(t, `<div class>x</div>`)
	if len(result.Classes) != 0 {
		t.Errorf("classes = %v", result.Classes)
	}
	if !strings.Contains(Inventory(result.Classes), "") {
		t.Error("an empty inventory is still a string")
	}
}
