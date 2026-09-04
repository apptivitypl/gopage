package compile

import (
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
)

func formType(t *testing.T, frontmatter string) (string, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	template := Template{File: "app/page.gopage", Frontmatter: frontmatter}
	return template.FormType(&bag), &bag
}

func TestFormTypeComesFromTheSubmitSignature(t *testing.T) {
	name, bag := formType(t, `
type ContactForm struct{}

func Submit(ctx *gopage.Ctx, params gopage.Params, form ContactForm) (gopage.Action, error) {
	return nil, nil
}
`)
	if bag.HasErrors() || name != "ContactForm" {
		t.Errorf("name = %q, diagnostics = %v", name, bag.Sorted())
	}
}

func TestNoSubmitMeansNoFormType(t *testing.T) {
	name, bag := formType(t, "type Props struct{}\n")
	if name != "" || bag.HasErrors() {
		t.Errorf("name = %q, diagnostics = %v", name, bag.Sorted())
	}
}

func TestAMalformedSubmitIsReported(t *testing.T) {
	sources := []string{
		"func Submit() {}\n",
		"func Submit(ctx *gopage.Ctx, params gopage.Params) (gopage.Action, error) { return nil, nil }\n",
		"func Submit(ctx gopage.Ctx, params gopage.Params, form F) (gopage.Action, error) { return nil, nil }\n",
		"func Submit(ctx *gopage.Ctx, params string, form F) (gopage.Action, error) { return nil, nil }\n",
		"func Submit(ctx *gopage.Ctx, params gopage.Params, form F) (string, error) { return \"\", nil }\n",
		"func Submit(ctx *gopage.Ctx, params gopage.Params, form F) (gopage.Action, string) { return nil, \"\" }\n",
		"func Submit(ctx *gopage.Ctx, params gopage.Params, form F) gopage.Action { return nil }\n",
		"func Submit(ctx *gopage.Ctx, params gopage.Params, form *F) (gopage.Action, error) { return nil, nil }\n",
	}
	for _, source := range sources {
		name, bag := formType(t, source)
		if name != "" || !hasCode(bag, diag.C312) {
			t.Errorf("%q produced %q, %v", source, name, bag.Sorted())
		}
	}
}

func TestAnUnparsableFrontmatterYieldsNoFormType(t *testing.T) {
	name, bag := formType(t, "func Submit( {\n")
	if name != "" || bag.HasErrors() {
		t.Errorf("name = %q, diagnostics = %v", name, bag.Sorted())
	}
}

func TestSubmitOnAReceiverIsIgnored(t *testing.T) {
	name, bag := formType(t, "type p struct{}\n\nfunc (p) Submit(a, b, c int) (int, error) { return 0, nil }\n")
	if name != "" || bag.HasErrors() {
		t.Errorf("name = %q, diagnostics = %v", name, bag.Sorted())
	}
}

func TestRootPathsAreRecognised(t *testing.T) {
	valid := [][]string{
		{"flash"},
		{"meta", "Title"},
		{"form", "Failed"},
		{"form", "Token"},
		{"form", "Values", "Email"},
		{"form", "Errors", "Email"},
	}
	for _, path := range valid {
		if !RootPath(path) {
			t.Errorf("%v was rejected", path)
		}
	}
	invalid := [][]string{
		nil,
		{"flash", "Message"},
		{"meta"},
		{"meta", "Wobble"},
		{"form"},
		{"form", "Wobble"},
		{"form", "Values"},
		{"form", "Values", "Email", "Extra"},
		{"props"},
	}
	for _, path := range invalid {
		if RootPath(path) {
			t.Errorf("%v was accepted", path)
		}
	}
	if len(RootNames()) != 4 {
		t.Errorf("roots = %v", RootNames())
	}
	for _, path := range [][]string{{"locale", "Tag"}, {"locale", "Dir"}, {"locale", "Prefix"}, {"locale", "Default"}} {
		if !RootPath(path) {
			t.Errorf("%v was rejected", path)
		}
	}
	if RootPath([]string{"locale", "Unknown"}) || RootPath([]string{"locale"}) {
		t.Error("the locale root takes exactly one known field")
	}
}

func TestLoaderArity(t *testing.T) {
	cases := map[string]bool{
		"func Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }":                       false,
		"func Load(ctx *gopage.Ctx, params gopage.Params) (Props, error) { return Props{}, nil }": true,
		"func Load(ctx, other *gopage.Ctx) (Props, error) { return Props{}, nil }":                true,
		"type Props struct{}": false,
		"func Load( {":        false,
	}
	for source, want := range cases {
		template := Template{File: "app/page.gopage", Frontmatter: source}
		if got := template.LoaderTakesParams(); got != want {
			t.Errorf("%q = %v, want %v", source, got, want)
		}
	}
}

func TestALoaderOnAReceiverIsIgnored(t *testing.T) {
	template := Template{File: "app/page.gopage", Frontmatter: "type p struct{}\n\nfunc (p) Load(a, b int) int { return 0 }\n"}
	if template.LoaderTakesParams() {
		t.Error("a method is not the page loader")
	}
}
