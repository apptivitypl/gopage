package syntax

import (
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/diag"
)

func elementOf(t *testing.T, source string) *Element {
	t.Helper()
	doc := parseClean(t, source)
	element, ok := doc.Nodes[0].(*Element)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	return element
}

func TestElementShapes(t *testing.T) {
	element := elementOf(t, `<div id="a" class="b c" hidden>`)
	if element.Name != "div" || element.SelfClosing {
		t.Errorf("element = %+v", element)
	}
	if len(element.Attributes) != 3 {
		t.Fatalf("attributes = %+v", element.Attributes)
	}
	if element.Attributes[2].Name != "hidden" || element.Attributes[2].Text != "" {
		t.Errorf("bare attribute = %+v", element.Attributes[2])
	}
}

func TestSelfClosingElement(t *testing.T) {
	if !elementOf(t, "<br/>").SelfClosing {
		t.Error("<br/> is self closing")
	}
	if elementOf(t, "<br>").SelfClosing {
		t.Error("<br> is not self closing")
	}
}

func TestHyphenatedNamesAreKept(t *testing.T) {
	element := elementOf(t, `<my-widget data-id="1">`)
	if element.Name != "my-widget" || element.Attributes[0].Name != "data-id" {
		t.Errorf("element = %+v", element)
	}
}

func TestAttributeValueParts(t *testing.T) {
	element := elementOf(t, `<a href="/x/{{ ID }}/y">`)
	parts := element.Attributes[0].Parts
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want text, interpolation, text", len(parts))
	}
	if text, ok := parts[0].(*Text); !ok || text.Value != "/x/" {
		t.Errorf("first part = %#v", parts[0])
	}
	if _, ok := parts[1].(*Interpolation); !ok {
		t.Errorf("second part = %#v", parts[1])
	}
}

func TestPlainValuesHaveNoParts(t *testing.T) {
	if parts := elementOf(t, `<a href="/x">`).Attributes[0].Parts; parts != nil {
		t.Errorf("parts = %+v, want none for a literal", parts)
	}
}

func TestAttributePartSpansPointIntoTheSource(t *testing.T) {
	source := `<a title="a {{ Name }} b">`
	element := elementOf(t, source)
	interp := element.Attributes[0].Parts[1].(*Interpolation)
	if got := source[interp.Span.Start:interp.Span.End]; got != "{{ Name }}" {
		t.Errorf("span covers %q", got)
	}
}

func TestBoundAndConditionalAttributes(t *testing.T) {
	element := elementOf(t, `<button :data-x="A" disabled?="B">`)
	bound := element.Attributes[0]
	if !bound.Bound || bound.Conditional || render(bound.Value) != "A" {
		t.Errorf("bound = %+v", bound)
	}
	conditional := element.Attributes[1]
	if conditional.Bound || !conditional.Conditional || render(conditional.Value) != "B" {
		t.Errorf("conditional = %+v", conditional)
	}
}

func TestClassMapEntries(t *testing.T) {
	element := elementOf(t, `<div :class="{ 'is-active': A, 'is-wide': B > 1 }">`)
	classes := element.Attributes[0].Classes
	if len(classes) != 2 {
		t.Fatalf("classes = %+v", classes)
	}
	if classes[0].Name != "is-active" || render(classes[0].Cond) != "A" {
		t.Errorf("first class = %+v", classes[0])
	}
	if classes[1].Name != "is-wide" || render(classes[1].Cond) != "(B > 1)" {
		t.Errorf("second class = %+v", classes[1])
	}
}

func TestClassMapWithCommasInsideConditions(t *testing.T) {
	element := elementOf(t, `<div :class="{ 'a': X[0] == 1, 'b': Y }">`)
	if len(element.Attributes[0].Classes) != 2 {
		t.Errorf("classes = %+v", element.Attributes[0].Classes)
	}
}

func TestEmptyClassMap(t *testing.T) {
	element := elementOf(t, `<div :class="{}">`)
	if len(element.Attributes[0].Classes) != 0 {
		t.Errorf("classes = %+v", element.Attributes[0].Classes)
	}
}

func TestBoundClassThatIsNotAMap(t *testing.T) {
	element := elementOf(t, `<div :class="A ~ B">`)
	attribute := element.Attributes[0]
	if len(attribute.Classes) != 0 || render(attribute.Value) != "(A ~ B)" {
		t.Errorf("attribute = %+v", attribute)
	}
}

func TestMalformedElementsReportC310(t *testing.T) {
	cases := map[string]string{
		"unclosed tag":         "<div",
		"bound without value":  `<div :class>`,
		"conditional no value": "<div hidden?>",
		"unquoted value":       "<div class=x>",
		"junk":                 "<div @>",
		"no attribute name":    `<div "x">`,
		"class entry no colon": `<div :class="{ 'a' }">`,
		"class entry no name":  `<div :class="{ '': A }">`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := parse(t, source)
			if !hasCode(bag, diag.C310) {
				t.Errorf("%q produced %v, want C310", source, codesOf(bag))
			}
		})
	}
}

func TestBrokenExpressionInAnAttributeIsReported(t *testing.T) {
	for _, source := range []string{
		`<div :data-x="1 +">`,
		`<div hidden?="1 +">`,
		`<div :class="{ 'a': 1 + }">`,
		`<div title="{{ 1 + }}">`,
	} {
		_, bag := parse(t, source)
		if bag.Len() == 0 {
			t.Errorf("%q parsed without a diagnostic", source)
		}
	}
}

func TestSplitTopRespectsNestingAndQuotes(t *testing.T) {
	got := splitTop(`'a': f(1, 2), 'b': X[0, 1], 'c,d': Y`)
	if len(got) != 3 {
		t.Errorf("splitTop = %q", got)
	}
	if !strings.Contains(got[2], "c,d") {
		t.Errorf("quoted comma was split: %q", got)
	}
}

func TestElementSpanCoversTheWholeTag(t *testing.T) {
	source := `<div class="x">`
	element := elementOf(t, source)
	if element.Span.Start != 0 || int(element.Span.End) != len(source) {
		t.Errorf("span = %+v", element.Span)
	}
}
