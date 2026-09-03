package compile

import (
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/apptivitypl/rill/internal/diag"
)

const (
	submitFunc = "Submit"
	actionType = "Action"
)

func SubmitForm(frontmatter, file string, bag *diag.Bag) (string, bool) {
	parsed, err := parser.ParseFile(token.NewFileSet(), file, "package rillpage\n"+frontmatter, parser.SkipObjectResolution)
	if err != nil {
		return "", false
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != submitFunc {
			continue
		}
		name, valid := formParameter(fn)
		if !valid {
			bag.Add(diag.New(diag.C312, file, diag.Span{}, "Submit has the wrong signature").
				WithHelp("write func Submit(ctx *rill.Ctx, params rill.Params, form MyForm) (rill.Action, error)"))
			return "", false
		}
		return name, true
	}
	return "", false
}

func formParameter(fn *ast.FuncDecl) (string, bool) {
	if fn.Type.Params == nil || fn.Type.Results == nil {
		return "", false
	}
	if fields(fn.Type.Params) != 3 || fields(fn.Type.Results) != 2 {
		return "", false
	}
	params := flatten(fn.Type.Params)
	results := flatten(fn.Type.Results)
	if !isPointer(params[0], CtxType) || !isNamed(params[1], ParamsType) {
		return "", false
	}
	if !isNamed(results[0], actionType) || !isNamed(results[1], "error") {
		return "", false
	}
	name, ok := params[2].(*ast.Ident)
	if !ok {
		return "", false
	}
	return name.Name, true
}

func (t Template) FormType(bag *diag.Bag) string {
	if !t.HasSubmit() {
		return ""
	}
	name, ok := SubmitForm(t.Frontmatter, t.File, bag)
	if !ok {
		return ""
	}
	return name
}

const loaderFunc = "Load"

func LoaderTakesParams(frontmatter, file string) bool {
	parsed, err := parser.ParseFile(token.NewFileSet(), file, "package rillpage\n"+frontmatter, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != loaderFunc {
			continue
		}
		return fn.Type.Params != nil && fields(fn.Type.Params) == 2
	}
	return false
}
