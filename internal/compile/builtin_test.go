package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/form"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func renderForm(t *testing.T, source string, result form.Result, token string) string {
	t.Helper()
	var bag diag.Bag
	compiled, err := Compile(fstest.MapFS{"app/page.gopage": &fstest.MapFile{Data: []byte(source)}}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	chain := compiled.Manifest.Chain(compiled.Manifest.Routes[0])
	out := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(out)
	if err := runtime.Render(chain, form.With(runtime.Empty{}, result, token), out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestFormCarriesTheToken(t *testing.T) {
	html := renderForm(t, `<Form class="stack">send</Form>`, form.Result{}, "tok")
	if !strings.Contains(html, `<form method="post" class="stack">`) {
		t.Errorf("html = %q", html)
	}
	if !strings.Contains(html, `<input type="hidden" name="__csrf" value="tok">`) {
		t.Errorf("html = %q", html)
	}
	if !strings.HasSuffix(html, "send</form>") {
		t.Errorf("html = %q", html)
	}
}

func TestFormKeepsAnExplicitMethod(t *testing.T) {
	html := renderForm(t, `<Form method="dialog">x</Form>`, form.Result{}, "")
	if strings.Count(html, "method=") != 1 || !strings.Contains(html, `method="dialog"`) {
		t.Errorf("html = %q", html)
	}
}

func TestFieldRendersLabelInputAndError(t *testing.T) {
	result := form.Result{
		Values: map[string]string{"Email": "a@b"},
		Errors: map[string][]string{"Email": {"nope"}},
	}
	html := renderForm(t, `<Field name="Email" label="your email" type="email" required />`, result, "")
	for _, want := range []string{
		`<label for="Email">your email</label>`,
		`<input id="Email" name="Email" type="email" required value="a@b">`,
		`<p class="field-error">nope</p>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html = %q, want %q", html, want)
		}
	}
}

func TestFieldWithoutAnErrorSkipsTheParagraph(t *testing.T) {
	html := renderForm(t, `<Field name="Email" />`, form.Result{}, "")
	if strings.Contains(html, "field-error") {
		t.Errorf("html = %q", html)
	}
	if !strings.Contains(html, `<label for="Email">Email</label>`) {
		t.Errorf("a field without a label falls back to its name: %q", html)
	}
}

func TestFieldAsTextarea(t *testing.T) {
	result := form.Result{Values: map[string]string{"Bio": "hello"}}
	html := renderForm(t, `<Field name="Bio" as="textarea" rows="6" />`, result, "")
	if !strings.Contains(html, `<textarea id="Bio" name="Bio" rows="6">hello</textarea>`) {
		t.Errorf("html = %q", html)
	}
}

func TestCheckboxKeepsItsState(t *testing.T) {
	ticked := renderForm(t, `<Field name="Consent" type="checkbox" />`,
		form.Result{Values: map[string]string{"Consent": "on"}}, "")
	if !strings.Contains(ticked, `type="checkbox" value="on" checked>`) {
		t.Errorf("html = %q", ticked)
	}
	empty := renderForm(t, `<Field name="Consent" type="checkbox" />`, form.Result{}, "")
	if strings.Contains(empty, "checked") {
		t.Errorf("html = %q", empty)
	}
}

func TestFieldEscapesItsLiterals(t *testing.T) {
	html := renderForm(t, `<Field name="Email" label="a&quot;b" />`, form.Result{}, "")
	if strings.Contains(html, `label="a"b"`) {
		t.Errorf("html = %q", html)
	}
}

func TestFieldNeedsALiteralName(t *testing.T) {
	sources := []string{`<Field />`, `<Field :name="Email" />`, `<Field name="" />`}
	for _, source := range sources {
		var bag diag.Bag
		_, err := Compile(fstest.MapFS{"app/page.gopage": &fstest.MapFile{Data: []byte(source)}}, &bag)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		if !hasCode(&bag, diag.C311) {
			t.Errorf("%q produced %v, want C311", source, bag.Sorted())
		}
	}
}

func TestBuiltinsAreOfferedAsSuggestions(t *testing.T) {
	var bag diag.Bag
	_, err := Compile(fstest.MapFS{"app/page.gopage": &fstest.MapFile{Data: []byte(`<Fied name="x" />`)}}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	rendered := ""
	for _, item := range bag.Sorted() {
		rendered += item.Help
	}
	if !strings.Contains(rendered, "Field") {
		t.Errorf("help = %q", rendered)
	}
	if !Builtin("Form") || Builtin("Badge") {
		t.Error("Builtin answers for the built-in set only")
	}
}

func TestFieldRejectsAComputedName(t *testing.T) {
	var bag diag.Bag
	_, err := Compile(fstest.MapFS{
		"app/page.gopage": &fstest.MapFile{Data: []byte(`<Field name="a{{ 1 }}b" />`)},
	}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C311) {
		t.Errorf("diagnostics = %v, want C311", bag.Sorted())
	}
}

func TestAPageThatAcceptsASubmissionIsDynamic(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{
		"app/page.gopage": &fstest.MapFile{Data: []byte(`---
type ContactForm struct{}

func Submit(ctx *gopage.Ctx, params gopage.Params, form ContactForm) (gopage.Action, error) {
	return nil, nil
}
---
<Form>x</Form>
`)},
	}, &bag)
	if err != nil || bag.HasErrors() {
		t.Fatalf("err = %v, diagnostics = %v", err, bag.Sorted())
	}
	if result.Manifest.Routes[0].Class != ir.ClassStatic {
		return
	}
	t.Error("a page with a submit handler needs a token per request and cannot be prerendered")
}

func TestFiltersRenderThroughThePlan(t *testing.T) {
	html := renderForm(t, `<p>{{ form.Values.Name | upper }}</p>`,
		form.Result{Values: map[string]string{"Name": "ada"}}, "")
	if !strings.Contains(html, "<p>ADA</p>") {
		t.Errorf("html = %q", html)
	}
}

func TestFiltersChain(t *testing.T) {
	html := renderForm(t, `<p>{{ form.Values.Name | default("none") | upper }}</p>`, form.Result{}, "")
	if !strings.Contains(html, "<p>NONE</p>") {
		t.Errorf("html = %q", html)
	}
}

func TestFilterDiagnostics(t *testing.T) {
	cases := map[string]string{
		"unknown":      `{{ form.Token | uppercase }}`,
		"extra":        `{{ form.Token | upper("x") }}`,
		"missing":      `{{ form.Token | truncate }}`,
		"no name":      `{{ form.Token | }}`,
		"unclosed arg": `{{ form.Token | truncate(3 }}`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			var bag diag.Bag
			if _, err := Compile(fstest.MapFS{"app/page.gopage": &fstest.MapFile{Data: []byte(source)}}, &bag); err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if !bag.HasErrors() {
				t.Errorf("%q was accepted", source)
			}
		})
	}
}

func TestAnUnknownFilterSuggestsTheClosestName(t *testing.T) {
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{
		"app/page.gopage": &fstest.MapFile{Data: []byte(`{{ form.Token | uppr }}`)},
	}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C313) {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	if !strings.Contains(bag.Sorted()[0].Help, "upper") {
		t.Errorf("help = %q", bag.Sorted()[0].Help)
	}
}

func TestAFarOffFilterNameListsTheRegistry(t *testing.T) {
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{
		"app/page.gopage": &fstest.MapFile{Data: []byte(`{{ form.Token | wobblewobble }}`)},
	}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !strings.Contains(bag.Sorted()[0].Help, "filters:") {
		t.Errorf("help = %q", bag.Sorted()[0].Help)
	}
}

func TestFieldTakesABoundLabel(t *testing.T) {
	html := renderForm(t, `<Field name="Email" :label="form.Values.Label" />`,
		form.Result{Values: map[string]string{"Label": "your email"}}, "")
	if !strings.Contains(html, `<label for="Email">your email</label>`) {
		t.Errorf("html = %q", html)
	}
}

func TestABoundLabelIsEscaped(t *testing.T) {
	html := renderForm(t, `<Field name="Email" :label="form.Values.Label" />`,
		form.Result{Values: map[string]string{"Label": "<b>"}}, "")
	if strings.Contains(html, "<b>") {
		t.Errorf("html = %q", html)
	}
}
