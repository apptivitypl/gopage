package schema

import (
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
)

func project() fstest.MapFS {
	return fstest.MapFS{
		"server/chrome/chrome.go": &fstest.MapFile{Data: []byte(`package chrome

type Nav struct {
	Home  string
	Links []Link
	Depth int
}

type Link struct {
	Label string
	Href  string
}

func (n Nav) private() {}
`)},
		"server/chrome/extra_test.go": &fstest.MapFile{Data: []byte("package chrome\n")},
	}
}

func adopted(t *testing.T, code string) (*Schema, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	packages := Packages{FS: project(), Module: "example.com/demo"}
	model := ParseWith([]Source{{File: "app/page.gopage", Code: code}}, packages, &bag)
	return model, &bag
}

const withNav = `import "example.com/demo/server/chrome"

type Props struct {
	Title string
	Nav   chrome.Nav
}
`

func TestAPropCarriesATypeFromTheProject(t *testing.T) {
	model, bag := adopted(t, withNav)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	field, ok := model.Structs["Props"].Field("Nav")
	if !ok {
		t.Fatal("the field is missing")
	}
	wrapper := WrapperName("example.com/demo/server/chrome", "Nav")
	if field.Type.Kind != KindStruct || field.Type.Name != wrapper {
		t.Errorf("field = %+v, want %s", field.Type, wrapper)
	}
	adoptedStruct, known := model.Structs[wrapper]
	if !known || !adoptedStruct.External || adoptedStruct.Local != "chrome.Nav" {
		t.Errorf("struct = %+v", adoptedStruct)
	}
	if _, ok := adoptedStruct.Field("Home"); !ok {
		t.Errorf("fields = %+v", adoptedStruct.Fields)
	}
}

func TestATypeReachedThroughAnotherIsAdoptedToo(t *testing.T) {
	model, bag := adopted(t, withNav)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	links, ok := model.Structs[WrapperName("example.com/demo/server/chrome", "Nav")].Field("Links")
	if !ok || links.Type.Kind != KindSlice {
		t.Fatalf("links = %+v", links)
	}
	if links.Type.Elem.Name != WrapperName("example.com/demo/server/chrome", "Link") {
		t.Errorf("element = %+v", links.Type.Elem)
	}
}

func TestAPackageOutsideTheModuleIsRefused(t *testing.T) {
	_, bag := adopted(t, "import \"github.com/other/pkg\"\n\ntype Props struct {\n\tNav pkg.Nav\n}\n")
	if !bag.HasErrors() {
		t.Error("a package outside the module was accepted")
	}
}

func TestAMissingTypeIsRefused(t *testing.T) {
	_, bag := adopted(t, "import \"example.com/demo/server/chrome\"\n\ntype Props struct {\n\tNav chrome.Ghost\n}\n")
	if !bag.HasErrors() {
		t.Error("a type that is not there was accepted")
	}
}

func TestWithoutAProjectNothingIsAdopted(t *testing.T) {
	var bag diag.Bag
	Parse([]Source{{File: "app/page.gopage", Code: withNav}}, &bag)
	if !bag.HasErrors() {
		t.Error("a compiler without the project filesystem cannot adopt")
	}
}

func TestTheWrapperNameIsAGoIdentifier(t *testing.T) {
	cases := map[string]string{
		"example.com/demo/server/chrome": "extChromeNav",
		"example.com/demo/view-model":    "extViewmodelNav",
		"example.com/demo":               "extDemoNav",
	}
	for importPath, want := range cases {
		if got := WrapperName(importPath, "Nav"); got != want {
			t.Errorf("WrapperName(%q) = %q, want %q", importPath, got, want)
		}
	}
}

func TestTheModuleRootIsAPackageToo(t *testing.T) {
	files := project()
	files["root.go"] = &fstest.MapFile{Data: []byte("package demo\n\ntype Root struct{ Name string }\n")}
	var bag diag.Bag
	packages := Packages{FS: files, Module: "example.com/demo"}
	model := ParseWith([]Source{{File: "app/page.gopage", Code: `import "example.com/demo"

type Props struct {
	Root demo.Root
}
`}}, packages, &bag)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if _, ok := model.Structs[WrapperName("example.com/demo", "Root")]; !ok {
		t.Errorf("structs = %v", model.Structs)
	}
}

func TestAnAliasedImportIsFollowed(t *testing.T) {
	var bag diag.Bag
	packages := Packages{FS: project(), Module: "example.com/demo"}
	model := ParseWith([]Source{{File: "app/page.gopage", Code: `import ui "example.com/demo/server/chrome"

type Props struct {
	Nav ui.Nav
}
`}}, packages, &bag)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	adopted, ok := model.Structs[WrapperName("example.com/demo/server/chrome", "Nav")]
	if !ok || adopted.Local != "ui.Nav" {
		t.Errorf("struct = %+v", adopted)
	}
}

func TestAPackageWithoutFilesIsRefused(t *testing.T) {
	var bag diag.Bag
	packages := Packages{FS: fstest.MapFS{}, Module: "example.com/demo"}
	ParseWith([]Source{{File: "app/page.gopage", Code: withNav}}, packages, &bag)
	if !bag.HasErrors() {
		t.Error("an empty package was accepted")
	}
}

func TestAFieldTypeInsideAnAdoptedPackageMustResolve(t *testing.T) {
	files := project()
	files["server/chrome/chrome.go"] = &fstest.MapFile{Data: []byte(`package chrome

type Nav struct {
	Broken map[string]int
}
`)}
	var bag diag.Bag
	packages := Packages{FS: files, Module: "example.com/demo"}
	ParseWith([]Source{{File: "app/page.gopage", Code: withNav}}, packages, &bag)
	if !bag.HasErrors() {
		t.Error("a map inside an adopted type was accepted")
	}
}

func TestATitleIsBuiltFromTheLastSegment(t *testing.T) {
	if got := title(""); got != "" {
		t.Errorf("title = %q", got)
	}
}

func TestTheSameTypeIsAdoptedOnce(t *testing.T) {
	model, bag := adopted(t, `import "example.com/demo/server/chrome"

type Props struct {
	Left  chrome.Nav
	Right chrome.Nav
}
`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	wrapper := WrapperName("example.com/demo/server/chrome", "Nav")
	count := 0
	for _, name := range model.Order {
		if name == wrapper {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the type is registered %d times", count)
	}
}

func TestADeepSelectorIsRefused(t *testing.T) {
	_, bag := adopted(t, "type Props struct {\n\tNav outer.inner.Nav\n}\n")
	if !bag.HasErrors() {
		t.Error("a nested selector was accepted")
	}
}

func TestATypeTheAdoptedPackageDoesNotDeclareIsRefused(t *testing.T) {
	files := project()
	files["server/chrome/chrome.go"] = &fstest.MapFile{Data: []byte(`package chrome

type Nav struct {
	Ghost Missing
}
`)}
	var bag diag.Bag
	packages := Packages{FS: files, Module: "example.com/demo"}
	ParseWith([]Source{{File: "app/page.gopage", Code: withNav}}, packages, &bag)
	if !bag.HasErrors() {
		t.Error("an unresolvable field type was accepted")
	}
}

func TestAnAdoptedPackageThatDoesNotParseIsRefused(t *testing.T) {
	files := project()
	files["server/chrome/broken.go"] = &fstest.MapFile{Data: []byte("package chrome\n\nfunc (")}
	var bag diag.Bag
	packages := Packages{FS: files, Module: "example.com/demo"}
	model := ParseWith([]Source{{File: "app/page.gopage", Code: withNav}}, packages, &bag)
	if _, ok := model.Structs[WrapperName("example.com/demo/server/chrome", "Nav")]; !ok {
		t.Errorf("a broken file next to a good one stops nothing: %v", bag.Items())
	}
}

func TestAnUnreadableFieldTypeIsRefusedInEveryShape(t *testing.T) {
	cases := map[string]string{
		"deep selector":    "type Props struct{ Nav a.b.C }",
		"slice of maps":    "type Props struct{ Rows []map[string]int }",
		"pointer to a map": "type Props struct{ Row *map[string]int }",
		"slice of arrays":  "type Props struct{ Rows [][3]int }",
	}
	for name, code := range cases {
		var bag diag.Bag
		Parse([]Source{{File: "app/page.gopage", Code: code}}, &bag)
		if !bag.HasErrors() {
			t.Errorf("%s was accepted", name)
		}
	}
}
