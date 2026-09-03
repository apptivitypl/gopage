package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/runtime"
)

func TestPlainElementsPassThrough(t *testing.T) {
	cases := map[string]string{
		"<p>text</p>":                           "<p>text</p>",
		`<div class="a b">x</div>`:              `<div class="a b">x</div>`,
		"<br/>":                                 "<br/>",
		"<input disabled>":                      "<input disabled>",
		"<!doctype html><html></html>":          "<!doctype html><html></html>",
		`<a href="!/x" title="y">go</a>`:        `<a href="/x" title="y">go</a>`,
		"<my-widget data-id=\"1\"></my-widget>": "<my-widget data-id=\"1\"></my-widget>",
	}
	for body, want := range cases {
		if got := mustRender(t, body, nil); got != want {
			t.Errorf("%s rendered %q, want %q", body, got, want)
		}
	}
}

func TestAttributeValuesInterpolate(t *testing.T) {
	props := runtime.Map{"ID": runtime.Int(7), "Kind": runtime.String("wide")}
	got := mustRender(t, `<a href="!/listings/{{ ID }}" class="card {{ Kind }}">x</a>`, props)
	if got != `<a href="/listings/7" class="card wide">x</a>` {
		t.Errorf("render = %q", got)
	}
}

func TestInterpolatedAttributeValuesAreEscaped(t *testing.T) {
	props := runtime.Map{"Title": runtime.String(`" onload="alert(1)`)}
	got := mustRender(t, `<div title="{{ Title }}">x</div>`, props)
	if strings.Contains(got, `onload="alert`) {
		t.Errorf("render = %q, want the quote escaped", got)
	}
	if !strings.Contains(got, "&#34;") {
		t.Errorf("render = %q", got)
	}
}

func TestLiteralAttributeValuesAreEscaped(t *testing.T) {
	got := mustRender(t, `<div title="a&b">x</div>`, nil)
	if got != `<div title="a&amp;b">x</div>` {
		t.Errorf("render = %q", got)
	}
}

func TestBoundAttributeRendersAnExpression(t *testing.T) {
	props := runtime.Map{"Count": runtime.Int(3)}
	got := mustRender(t, `<div :data-count="Count + 1">x</div>`, props)
	if got != `<div data-count="4">x</div>` {
		t.Errorf("render = %q", got)
	}
}

func TestConditionalAttributeAppearsOnlyWhenTrue(t *testing.T) {
	body := `<button disabled?="Locked">go</button>`
	cases := map[bool]string{
		true:  `<button disabled>go</button>`,
		false: `<button>go</button>`,
	}
	for value, want := range cases {
		props := runtime.Map{"Locked": runtime.Bool(value)}
		if got := mustRender(t, body, props); got != want {
			t.Errorf("Locked=%v rendered %q, want %q", value, got, want)
		}
	}
}

func TestClassMapEmitsOnlyMatchingClasses(t *testing.T) {
	body := `<div :class="{ 'is-active': Active, 'is-wide': Wide }">x</div>`
	props := runtime.Map{"Active": runtime.Bool(true), "Wide": runtime.Bool(false)}
	if got := mustRender(t, body, props); got != `<div class="is-active ">x</div>` {
		t.Errorf("render = %q", got)
	}
}

func TestClassMapWithEveryConditionFalse(t *testing.T) {
	body := `<div :class="{ 'a': Active }">x</div>`
	props := runtime.Map{"Active": runtime.Bool(false)}
	if got := mustRender(t, body, props); got != `<div class="">x</div>` {
		t.Errorf("render = %q", got)
	}
}

func TestClassMapReadsExpressions(t *testing.T) {
	body := `<div :class="{ 'big': Count > 2, 'small': Count <= 2 }">x</div>`
	props := runtime.Map{"Count": runtime.Int(5)}
	got := mustRender(t, body, props)
	if !strings.Contains(got, "big") || strings.Contains(got, "small") {
		t.Errorf("render = %q", got)
	}
}

func TestBoundClassWithAnExpressionIsNotAMap(t *testing.T) {
	props := runtime.Map{"Base": runtime.String("card"), "Extra": runtime.String("wide")}
	got := mustRender(t, `<div :class="Base ~ ' ' ~ Extra">x</div>`, props)
	if got != `<div class="card wide">x</div>` {
		t.Errorf("render = %q", got)
	}
}

func TestElementsInsideControlFlow(t *testing.T) {
	props := runtime.Map{"Items": numbers(1, 2)}
	got := mustRender(t, `{% for n in Items %}<li data-n="{{ n }}">{{ n }}</li>{% endfor %}`, props)
	if got != `<li data-n="1">1</li><li data-n="2">2</li>` {
		t.Errorf("render = %q", got)
	}
}

func TestAdjacentStaticsAreCoalesced(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": file("<div><span><b>deep</b></span></div>")}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	plan := result.Manifest.Plans[0]
	if len(plan.Ops) != 1 {
		t.Errorf("ops = %d, want one static block for markup with no dynamic parts", len(plan.Ops))
	}
}

func TestCoalescingStopsAtDynamicParts(t *testing.T) {
	var bag diag.Bag
	result, _ := Compile(fstest.MapFS{"app/page.rill": file("<p>{{ A }}</p>")}, &bag)
	plan := result.Manifest.Plans[0]
	if len(plan.Ops) != 3 {
		t.Errorf("ops = %d, want static, text, static", len(plan.Ops))
	}
}

func TestMalformedAttributesReportC310(t *testing.T) {
	cases := map[string]string{
		"unclosed tag":         "<div",
		"bound without value":  `<div :class>`,
		"conditional no value": `<div hidden?>`,
		"value not quoted":     "<div class=x>",
		"junk in tag":          "<div @>",
		"class entry no cond":  `<div :class="{ 'a' }">`,
		"class entry no name":  `<div :class="{ '': A }">`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var bag diag.Bag
			if _, err := Compile(fstest.MapFS{"app/page.rill": file(body)}, &bag); err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if !hasCode(&bag, diag.C310) {
				t.Errorf("diagnostics = %+v, want C310", bag.Items())
			}
		})
	}
}

func TestAttributeExpressionsAreTypeChecked(t *testing.T) {
	for _, body := range []string{
		`<div :class="Missing">x</div>`,
		`<div hidden?="Missing">x</div>`,
		`<div title="{{ Missing }}">x</div>`,
		`<div :class="{ 'a': Missing }">x</div>`,
	} {
		var bag diag.Bag
		source := propsBlock + body
		if _, err := Compile(fstest.MapFS{"app/page.rill": file(source)}, &bag); err != nil {
			t.Fatalf("Compile: %v", err)
		}
		if !hasCode(&bag, diag.C305) {
			t.Errorf("%q produced %+v, want C305", body, bag.Items())
		}
	}
}

func TestAttributeDiagnosticPointsIntoTheValue(t *testing.T) {
	source := propsBlock + `<div title="{{ Missing }}">x</div>`
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.rill": file(source)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, d := range bag.Items() {
		if d.Code != diag.C305 {
			continue
		}
		if got := source[d.Span.Start:d.Span.End]; got != "Missing" {
			t.Errorf("span covers %q, want the expression inside the value", got)
		}
		return
	}
	t.Fatal("no C305 reported")
}
