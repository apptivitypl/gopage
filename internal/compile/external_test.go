package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func withChrome(files map[string]string) fstest.MapFS {
	tree := app(files)
	tree["go.mod"] = &fstest.MapFile{Data: []byte("module example.com/demo\n\ngo 1.26\n")}
	tree["server/chrome/chrome.go"] = &fstest.MapFile{Data: []byte(`package chrome

type Nav struct {
	Home  string
	Links []Link
}

type Link struct {
	Label string
	Href  string
}
`)}
	return tree
}

const navPage = `---
import "example.com/demo/server/chrome"

type Props struct {
	Nav chrome.Nav
}

func Load(ctx *gopage.Ctx) (Props, error) {
	return Props{}, nil
}
---
<p>{{ Nav.Home }}</p>
{% for link in Nav.Links %}<a href="/">{{ link.Label }}</a>{% endfor %}
`

func TestAPropFromAnotherPackageCompiles(t *testing.T) {
	var bag diag.Bag
	if _, err := Compile(withChrome(map[string]string{"app/page.gopage": navPage}), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestAFieldThatIsNotThereIsStillReported(t *testing.T) {
	page := strings.Replace(navPage, "{{ Nav.Home }}", "{{ Nav.Ghost }}", 1)
	var bag diag.Bag
	if _, err := Compile(withChrome(map[string]string{"app/page.gopage": page}), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, ok := found(&bag, diag.C305); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestATypeOutsideTheModuleIsRefused(t *testing.T) {
	page := strings.Replace(navPage, `import "example.com/demo/server/chrome"`, `import "github.com/other/chrome"`, 1)
	var bag diag.Bag
	if _, err := Compile(withChrome(map[string]string{"app/page.gopage": page}), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, ok := found(&bag, diag.C302); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestAComponentMayCarryAProjectType(t *testing.T) {
	files := withChrome(map[string]string{"app/page.gopage": `<Header :Nav="Nav" />`})
	files["components/Header/template.gopage"] = &fstest.MapFile{Data: []byte(`<nav>{{ Nav.Home }}</nav>`)}
	files["components/Header/props.go"] = &fstest.MapFile{Data: []byte(`package Header

import "example.com/demo/server/chrome"

type Props struct {
	Nav chrome.Nav
}
`)}
	files["app/page.gopage"] = &fstest.MapFile{Data: []byte(`---
import "example.com/demo/server/chrome"

type Props struct {
	Nav chrome.Nav
}

func Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }
---
<Header :Nav="Nav" />
`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

var _ = runtime.Empty{}
