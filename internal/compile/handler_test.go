package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
)

func handlerRoute(file string) Route {
	return Route{Pattern: "/api/health", Name: "api.health", Kind: RouteAPI, File: file}
}

func loadHandler(t *testing.T, source string) (Handler, *diag.Bag) {
	t.Helper()
	fsys := fstest.MapFS{"app/api/health/route.go": &fstest.MapFile{Data: []byte(source)}}
	var bag diag.Bag
	handler, _ := LoadHandler(fsys, handlerRoute("app/api/health/route.go"), &bag)
	return handler, &bag
}

func TestHandlerMethodsAreCollected(t *testing.T) {
	handler, bag := loadHandler(t, `package route

func POST(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) { return nil, nil }
func GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) { return nil, nil }
func helper() {}
`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	if strings.Join(handler.Methods, ",") != "GET,POST" {
		t.Errorf("methods = %v", handler.Methods)
	}
	if handler.Package != "route" || handler.Dir != "app/api/health" || handler.Pattern != "/api/health" {
		t.Errorf("handler = %+v", handler)
	}
}

func TestHandlerWithoutAMethodIsReported(t *testing.T) {
	_, bag := loadHandler(t, "package route\n\nfunc helper() {}\n")
	if !hasCode(bag, diag.C104) {
		t.Errorf("diagnostics = %v, want C104", bag.Sorted())
	}
}

func TestHandlerWithAWrongSignatureIsReported(t *testing.T) {
	sources := []string{
		"package route\n\nfunc GET() {}\n",
		"package route\n\nfunc GET(ctx *gopage.Ctx) (gopage.Response, error) { return nil, nil }\n",
		"package route\n\nfunc GET(ctx gopage.Ctx, params gopage.Params) (gopage.Response, error) { return nil, nil }\n",
		"package route\n\nfunc GET(ctx *gopage.Ctx, params string) (gopage.Response, error) { return nil, nil }\n",
		"package route\n\nfunc GET(ctx *gopage.Ctx, params gopage.Params) (string, error) { return \"\", nil }\n",
		"package route\n\nfunc GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, string) { return nil, \"\" }\n",
		"package route\n\nfunc GET(ctx *gopage.Ctx, params gopage.Params) gopage.Response { return nil }\n",
		"package route\n\nfunc GET(ctx, params *gopage.Ctx) (gopage.Response, error) { return nil, nil }\n",
		"package route\n\nfunc GET(ctx *gopage.Ctx, params gopage.Params) (func(), error) { return nil, nil }\n",
	}
	for _, source := range sources {
		_, bag := loadHandler(t, source)
		if !hasCode(bag, diag.C105) && !hasCode(bag, diag.C104) {
			t.Errorf("%q produced %v, want C105", source, bag.Sorted())
		}
	}
}

func TestHandlerWithAMethodOnAReceiverIsIgnored(t *testing.T) {
	_, bag := loadHandler(t, `package route

type api struct{}

func (api) GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) { return nil, nil }
`)
	if !hasCode(bag, diag.C104) {
		t.Errorf("diagnostics = %v, want C104", bag.Sorted())
	}
}

func TestUnparsableHandlerIsReported(t *testing.T) {
	_, bag := loadHandler(t, "package route\n\nfunc GET( {\n")
	if !hasCode(bag, diag.C105) {
		t.Errorf("diagnostics = %v, want C105", bag.Sorted())
	}
}

func TestUnreadableHandlerIsReported(t *testing.T) {
	var bag diag.Bag
	if _, ok := LoadHandler(fstest.MapFS{}, handlerRoute("app/api/health/route.go"), &bag); ok {
		t.Fatal("a missing file cannot produce a handler")
	}
	if !hasCode(&bag, diag.C104) {
		t.Errorf("diagnostics = %v, want C104", bag.Sorted())
	}
}

func TestMuxPatternTranslatesSegments(t *testing.T) {
	cases := map[string]string{
		"/":                      "/",
		"/api/health":            "/api/health",
		"/api/listings/[id]":     "/api/listings/{id}",
		"/api/files/[...path]":   "/api/files/{path...}",
		"/api/files/[[...rest]]": "/api/files/{rest...}",
	}
	for pattern, want := range cases {
		if got := MuxPattern(pattern); got != want {
			t.Errorf("MuxPattern(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func TestMuxParamsListsTheNames(t *testing.T) {
	if got := MuxParams("/api/files/{path...}"); len(got) != 1 || got[0] != "path" {
		t.Errorf("params = %v", got)
	}
	if got := MuxParams("/api/health"); got != nil {
		t.Errorf("params = %v, want none", got)
	}
}

func TestHandlersReachTheResult(t *testing.T) {
	fsys := fstest.MapFS{
		"app/page.gopage": &fstest.MapFile{Data: []byte("<h1>home</h1>")},
		"app/api/health/route.go": &fstest.MapFile{Data: []byte(
			"package route\n\nfunc GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) { return nil, nil }\n")},
	}
	var bag diag.Bag
	result, err := Compile(fsys, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	if len(result.Handlers) != 1 || result.Handlers[0].Route != "api.health" {
		t.Errorf("handlers = %+v", result.Handlers)
	}
}
