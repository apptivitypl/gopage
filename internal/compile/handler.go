package compile

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/apptivitypl/rill/internal/diag"
)

const (
	CtxType      = "Ctx"
	ParamsType   = "Params"
	ResponseType = "Response"
)

var methods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

type Handler struct {
	Route   string
	Pattern string
	Package string
	Dir     string
	Methods []string
}

func LoadHandler(fsys fs.FS, route Route, bag *diag.Bag) (Handler, bool) {
	source, err := fs.ReadFile(fsys, route.File)
	if err != nil {
		bag.Add(diag.New(diag.C104, route.File, diag.Span{}, fmt.Sprintf("%s cannot be read", route.File)))
		return Handler{}, false
	}
	file, err := parser.ParseFile(token.NewFileSet(), route.File, source, parser.SkipObjectResolution)
	if err != nil {
		bag.Add(diag.New(diag.C105, route.File, diag.Span{}, fmt.Sprintf("%s does not parse: %v", route.File, err)))
		return Handler{}, false
	}
	handler := Handler{
		Route:   route.Name,
		Pattern: MuxPattern(route.Pattern),
		Package: file.Name.Name,
		Dir:     dirOf(route.File),
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !slices.Contains(methods, fn.Name.Name) {
			continue
		}
		if !validSignature(fn) {
			bag.Add(diag.New(diag.C105, route.File, diag.Span{},
				fmt.Sprintf("%s has the wrong signature", fn.Name.Name)).
				WithHelp("write func " + fn.Name.Name + "(ctx *rill.Ctx, params rill.Params) (rill.Response, error)"))
			continue
		}
		handler.Methods = append(handler.Methods, fn.Name.Name)
	}
	if len(handler.Methods) == 0 {
		bag.Add(diag.New(diag.C104, route.File, diag.Span{},
			fmt.Sprintf("%s declares no HTTP method", route.File)).
			WithHelp("name a function after the method it answers: " + strings.Join(methods, ", ")))
		return Handler{}, false
	}
	sort.Strings(handler.Methods)
	return handler, true
}

func validSignature(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || fn.Type.Results == nil {
		return false
	}
	if fields(fn.Type.Params) != 2 || fields(fn.Type.Results) != 2 {
		return false
	}
	params := flatten(fn.Type.Params)
	results := flatten(fn.Type.Results)
	return isPointer(params[0], CtxType) && isNamed(params[1], ParamsType) &&
		isNamed(results[0], ResponseType) && isNamed(results[1], "error")
}

func fields(list *ast.FieldList) int {
	total := 0
	for _, field := range list.List {
		total += max(len(field.Names), 1)
	}
	return total
}

func flatten(list *ast.FieldList) []ast.Expr {
	var out []ast.Expr
	for _, field := range list.List {
		for range max(len(field.Names), 1) {
			out = append(out, field.Type)
		}
	}
	return out
}

func isPointer(expr ast.Expr, name string) bool {
	star, ok := expr.(*ast.StarExpr)
	return ok && isNamed(star.X, name)
}

func isNamed(expr ast.Expr, name string) bool {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name == name
	case *ast.SelectorExpr:
		return node.Sel.Name == name
	default:
		return false
	}
}

func MuxPattern(pattern string) string {
	var parts []string
	for segment := range strings.SplitSeq(strings.Trim(pattern, "/"), "/") {
		switch {
		case segment == "":
		case strings.HasPrefix(segment, "[[..."):
			parts = append(parts, "{"+strings.Trim(segment, "[.]")+"...}")
		case strings.HasPrefix(segment, "[..."):
			parts = append(parts, "{"+strings.Trim(segment, "[.]")+"...}")
		case strings.HasPrefix(segment, "["):
			parts = append(parts, "{"+strings.Trim(segment, "[]")+"}")
		default:
			parts = append(parts, segment)
		}
	}
	if len(parts) == 0 {
		return "/"
	}
	return "/" + strings.Join(parts, "/")
}

func dirOf(file string) string {
	if cut := strings.LastIndex(file, "/"); cut >= 0 {
		return file[:cut]
	}
	return "."
}

func MuxParams(pattern string) []string {
	var names []string
	for segment := range strings.SplitSeq(strings.Trim(pattern, "/"), "/") {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		names = append(names, strings.TrimSuffix(strings.Trim(segment, "{}"), "..."))
	}
	return names
}
