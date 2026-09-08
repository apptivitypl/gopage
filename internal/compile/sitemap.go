package compile

import (
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/apptivitypl/gopage/internal/diag"
)

const (
	sitemapFunc = "Sitemap"
	sitemapType = "SitemapSeq"
)

func SitemapSignature(frontmatter, file string, bag *diag.Bag) bool {
	parsed, err := parser.ParseFile(token.NewFileSet(), file, "package pagesrc\n"+frontmatter, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != sitemapFunc {
			continue
		}
		if sitemapShape(fn) {
			return true
		}
		bag.Add(diag.New(diag.C325, file, diag.Span{}, "Sitemap has the wrong signature").
			WithHelp("write func Sitemap(ctx *gopage.Ctx) (gopage.SitemapSeq, error)"))
		return false
	}
	return false
}

func sitemapShape(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || fn.Type.Results == nil {
		return false
	}
	if fields(fn.Type.Params) != 1 || fields(fn.Type.Results) != 2 {
		return false
	}
	params := flatten(fn.Type.Params)
	results := flatten(fn.Type.Results)
	return isPointer(params[0], CtxType) && isNamed(results[0], sitemapType) && isNamed(results[1], "error")
}

func (t Template) Sitemap(bag *diag.Bag) bool {
	if !t.HasSitemap() {
		return false
	}
	return SitemapSignature(t.Frontmatter, t.File, bag)
}
