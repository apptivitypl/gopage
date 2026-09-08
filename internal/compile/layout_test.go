package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/runtime"
)

const loadingLayout = "---\ntype Props struct{ Home string }\n\n" +
	"func Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }\n---\n" +
	"<nav>{{ layout.Home }}</nav>{% outlet %}"

func renderChain(t *testing.T, files fstest.MapFS, props runtime.Accessible, layouts []runtime.Accessible) string {
	t.Helper()
	result, bag := compiled(t, files)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	route, ok := result.Manifest.Lookup("/")
	if !ok {
		t.Fatal("no route answers /")
	}
	out := runtime.NewBuffer(256)
	if err := runtime.RenderOptions(result.Manifest.Chain(route), props, out, runtime.Options{Layouts: layouts}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestALayoutRendersItsOwnProps(t *testing.T) {
	tree := app(map[string]string{"app/page.gopage": "<p>{{ Title }}</p>"})
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte(loadingLayout)}
	got := renderChain(t, tree,
		runtime.Map{"Title": runtime.String("home")},
		[]runtime.Accessible{runtime.Map{"Home": runtime.String("/")}, nil})
	if got != "<nav>/</nav><p>home</p>" {
		t.Errorf("render = %q", got)
	}
}

func TestEachLayoutInTheChainReadsItsOwn(t *testing.T) {
	tree := app(map[string]string{"app/inner/page.gopage": "<p>page</p>"})
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte(
		"---\ntype Props struct{ Name string }\n\nfunc Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }\n---\n" +
			"<a>{{ layout.Name }}</a>{% outlet %}")}
	tree["app/inner/layout.gopage"] = &fstest.MapFile{Data: []byte(
		"---\ntype Props struct{ Name string }\n\nfunc Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }\n---\n" +
			"<b>{{ layout.Name }}</b>{% outlet %}")}
	result, bag := compiled(t, tree)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	route, ok := result.Manifest.Lookup("/inner")
	if !ok {
		t.Fatal("no route answers /inner")
	}
	out := runtime.NewBuffer(256)
	layouts := []runtime.Accessible{
		runtime.Map{"Name": runtime.String("outer")},
		runtime.Map{"Name": runtime.String("inner")},
		nil,
	}
	if err := runtime.RenderOptions(result.Manifest.Chain(route), runtime.Map{}, out,
		runtime.Options{Layouts: layouts}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := out.String(); got != "<a>outer</a><b>inner</b><p>page</p>" {
		t.Errorf("render = %q", got)
	}
}

func TestALayoutStillReadsThePageProps(t *testing.T) {
	tree := app(map[string]string{"app/page.gopage": "---\ntype Props struct{ Title string }\n---\n<p>body</p>"})
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte("<h1>{{ Title }}</h1>{% outlet %}")}
	got := renderChain(t, tree, runtime.Map{"Title": runtime.String("home")}, nil)
	if got != "<h1>home</h1><p>body</p>" {
		t.Errorf("render = %q", got)
	}
}

func TestALayoutFieldThatDoesNotExistIsRejected(t *testing.T) {
	tree := app(map[string]string{"app/page.gopage": "<p>home</p>"})
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte(
		"---\ntype Props struct{ Home string }\n\nfunc Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }\n---\n" +
			"<nav>{{ layout.Absent }}</nav>{% outlet %}")}
	_, bag := compiled(t, tree)
	item, ok := found(bag, diag.C305)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Message, "Absent") {
		t.Errorf("message = %q", item.Message)
	}
}

func TestALayoutRootInAPageIsRejected(t *testing.T) {
	_, bag := compiled(t, app(map[string]string{
		"app/page.gopage": "---\ntype Props struct{ Title string }\n---\n<p>{{ layout.Title }}</p>",
	}))
	item, ok := found(bag, diag.C305)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Message, "not a layout") {
		t.Errorf("message = %q", item.Message)
	}
}

func TestABareLayoutRootIsRejected(t *testing.T) {
	tree := app(map[string]string{"app/page.gopage": "<p>home</p>"})
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte(
		"---\ntype Props struct{ Home string }\n\nfunc Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }\n---\n" +
			"<nav>{{ layout }}</nav>{% outlet %}")}
	if _, bag := compiled(t, tree); !bag.HasErrors() {
		t.Error("a bare layout root names no value")
	}
}
