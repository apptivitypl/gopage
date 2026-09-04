package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
)

const propsBlock = `---
type Props struct {
	Title   string
	Count   int
	Tags    []string
	Badge   *string
	Listing Listing
	Cards   []Card
}

type Listing struct {
	Title string
	Owner Owner
}

type Owner struct {
	Name string
}

type Card struct {
	Title string
	Price int
}
---
`

func typed(t *testing.T, body string) *diag.Bag {
	t.Helper()
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.gopage": file(propsBlock + body)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return &bag
}

func accepts(t *testing.T, body string) {
	t.Helper()
	if bag := typed(t, body); bag.HasErrors() {
		t.Errorf("%q was rejected: %+v", body, bag.Items())
	}
}

func rejects(t *testing.T, body string) diag.Diagnostic {
	t.Helper()
	bag := typed(t, body)
	for _, d := range bag.Items() {
		if d.Code == diag.C305 {
			return d
		}
	}
	t.Fatalf("%q was accepted, diagnostics: %+v", body, bag.Items())
	return diag.Diagnostic{}
}

func TestKnownPathsAreAccepted(t *testing.T) {
	for _, body := range []string{
		"{{ Title }}",
		"{{ Count }}",
		"{{ Listing.Title }}",
		"{{ Listing.Owner.Name }}",
		"{{ Badge }}",
		"{{ Tags[0] }}",
		"{{ Count + 1 }}",
		"{% if Count > 0 %}x{% endif %}",
		"{% for c in Cards %}{{ c.Title }}{% endfor %}",
		"{% for c in Cards %}{{ c.Price + Count }}{% endfor %}",
		"{% for tag in Tags %}{{ tag }}{% endfor %}",
		"{% let n = Count %}{{ n }}",
	} {
		accepts(t, body)
	}
}

func TestUnknownPropsFieldIsRejected(t *testing.T) {
	d := rejects(t, "{{ Missing }}")
	if !strings.Contains(d.Message, "Props has no field Missing") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestUnknownNestedFieldIsRejected(t *testing.T) {
	d := rejects(t, "{{ Listing.Nope }}")
	if !strings.Contains(d.Message, "Listing has no field Nope") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestUnknownLoopFieldIsRejected(t *testing.T) {
	d := rejects(t, "{% for c in Cards %}{{ c.Nope }}{% endfor %}")
	if !strings.Contains(d.Message, "Card has no field Nope") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestATypoSuggestsTheRealField(t *testing.T) {
	d := rejects(t, "{{ Titel }}")
	if !strings.Contains(d.Help, "Title") {
		t.Errorf("help = %q, want a suggestion", d.Help)
	}
}

func TestAWildNameListsTheFields(t *testing.T) {
	d := rejects(t, "{{ Zzzzzzzzzz }}")
	if !strings.Contains(d.Help, "available fields") || !strings.Contains(d.Help, "Title") {
		t.Errorf("help = %q, want the field list", d.Help)
	}
}

func TestFieldAccessOnAScalarIsRejected(t *testing.T) {
	d := rejects(t, "{{ Title.Nope }}")
	if d.Code != diag.C305 {
		t.Errorf("code = %s", d.Code)
	}
}

func TestFieldAccessOnAScalarLoopValueIsRejected(t *testing.T) {
	d := rejects(t, "{% for tag in Tags %}{{ tag.Nope }}{% endfor %}")
	if !strings.Contains(d.Message, "has no fields") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestLoopingOverANonSliceIsRejected(t *testing.T) {
	d := rejects(t, "{% for c in Title %}x{% endfor %}")
	if !strings.Contains(d.Message, "not a list") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestDiagnosticPointsAtTheTemplateLine(t *testing.T) {
	body := "<h1>ok</h1>\n<p>{{ Missing }}</p>\n"
	var bag diag.Bag
	source := propsBlock + body
	if _, err := Compile(fstest.MapFS{"app/page.gopage": file(source)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	d := bag.Items()[0]
	position := diag.PositionOf(source, d.Span.Start)
	if position.Line != 26 {
		t.Errorf("line = %d, want the line inside the template body", position.Line)
	}
	if d.File != "app/page.gopage" {
		t.Errorf("file = %q", d.File)
	}
}

func TestPathsInsideEveryConstructAreChecked(t *testing.T) {
	for _, body := range []string{
		"{% if Missing %}x{% endif %}",
		"{% if Count > 0 %}{{ Missing }}{% endif %}",
		"{% if Count > 0 %}x{% else %}{{ Missing }}{% endif %}",
		"{% for c in Cards %}{{ Missing }}{% endfor %}",
		"{% for c in Cards %}x{% else %}{{ Missing }}{% endfor %}",
		"{% let n = Missing %}",
		"{{ -Missing }}",
		"{{ Count + Missing }}",
		"{{ Tags[Missing] }}",
		"{{ Missing[0] }}",
	} {
		if bag := typed(t, body); !bag.HasErrors() {
			t.Errorf("%q was accepted", body)
		}
	}
}

func TestLoopVariableLeavesScope(t *testing.T) {
	if bag := typed(t, "{% for c in Cards %}{{ c.Title }}{% endfor %}{{ c }}"); !bag.HasErrors() {
		t.Error("the loop variable must not be visible after the loop")
	}
}

func TestLoopVariableIsNotVisibleInTheEmptyBranch(t *testing.T) {
	if bag := typed(t, "{% for c in Cards %}x{% else %}{{ c }}{% endfor %}"); !bag.HasErrors() {
		t.Error("the loop variable must not be visible in the else branch")
	}
}

func TestLetIsVisibleAfterwards(t *testing.T) {
	accepts(t, "{% let n = Count %}{{ n }}{{ n + 1 }}")
}

func TestLetOfAStructKeepsItsFields(t *testing.T) {
	accepts(t, "{% let l = Listing %}{{ l.Title }}")
	if bag := typed(t, "{% let l = Listing %}{{ l.Nope }}"); !bag.HasErrors() {
		t.Error("a bound struct must still be checked")
	}
}

func TestLetOfAnExpressionIsNotChecked(t *testing.T) {
	accepts(t, "{% let n = Count + 1 %}{{ n }}")
}

func TestTemplateWithoutPropsIsNotChecked(t *testing.T) {
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.gopage": file("{{ Anything }}")}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Errorf("a template without a props block must not be checked: %+v", bag.Items())
	}
}

func TestBlockWithoutPropsStructIsNotChecked(t *testing.T) {
	var bag diag.Bag
	source := "---\ntype Card struct{ Title string }\n---\n{{ Anything }}"
	if _, err := Compile(fstest.MapFS{"app/page.gopage": file(source)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Errorf("without a Props struct there is nothing to check against: %+v", bag.Items())
	}
}

func TestSchemaIsReturnedForCodegen(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.gopage": file(propsBlock + "{{ Title }}")}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	model, ok := result.Schemas["app/page.gopage"]
	if !ok {
		t.Fatal("the schema was not returned")
	}
	if !model.Has("Card") {
		t.Errorf("structs = %v", model.Order)
	}
	template, ok := result.Templates["app/page.gopage"]
	if !ok {
		t.Fatal("the template was not returned")
	}
	if template.FirstLine != 2 {
		t.Errorf("frontmatter starts at line %d, want 2", template.FirstLine)
	}
	if !strings.Contains(template.Frontmatter, "type Props struct") {
		t.Error("the frontmatter was not captured")
	}
}

func TestLoopingOverAnExpressionIsNotChecked(t *testing.T) {
	accepts(t, "{% for x in Tags[0] %}{{ x }}{% endfor %}")
}

func TestPathThroughAScalarIsRejected(t *testing.T) {
	d := rejects(t, "{{ Listing.Title.Deeper }}")
	if d.Help != "" {
		t.Errorf("help = %q, want none when the path leaves the schema", d.Help)
	}
}

func TestLetBoundToAnExpressionAllowsAnyField(t *testing.T) {
	accepts(t, "{% let n = Count + 1 %}{{ n.Anything }}")
}

func TestNestedLoopsKeepTheirOwnScopes(t *testing.T) {
	accepts(t, "{% for c in Cards %}{% for t in Tags %}{{ c.Title }}{{ t }}{% endfor %}{% endfor %}")
	if bag := typed(t, "{% for c in Cards %}{% for t in Tags %}{{ t.Nope }}{% endfor %}{% endfor %}"); !bag.HasErrors() {
		t.Error("the inner loop variable must still be checked")
	}
}

func TestOptionalStructFieldsResolve(t *testing.T) {
	source := "---\ntype Props struct{ Owner *Owner }\ntype Owner struct{ Name string }\n---\n{{ Owner.Name }}"
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.gopage": file(source)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Errorf("an optional struct must resolve its fields: %+v", bag.Items())
	}
}
