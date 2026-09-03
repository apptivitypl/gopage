package codegen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

type Segment struct {
	Line int
	Text string
}

func SplitImports(source string) ([]string, []Segment) {
	lines := strings.Split(source, "\n")
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "frontmatter.go", "package rillpage\n"+source, parser.SkipObjectResolution)
	if err != nil {
		return nil, []Segment{{Line: 1, Text: source}}
	}
	var imports []string
	skip := map[int]bool{}
	for _, decl := range parsed.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.IMPORT {
			continue
		}
		for _, spec := range generic.Specs {
			if imported, ok := spec.(*ast.ImportSpec); ok {
				imports = append(imports, importSpec(imported))
			}
		}
		from := fileSet.Position(generic.Pos()).Line - 1
		to := fileSet.Position(generic.End()).Line - 1
		for line := from; line <= to; line++ {
			skip[line] = true
		}
	}
	if len(imports) == 0 {
		return nil, []Segment{{Line: 1, Text: source}}
	}
	return imports, segments(lines, skip)
}

func segments(lines []string, skip map[int]bool) []Segment {
	var out []Segment
	var current []string
	start := 0
	for index, line := range lines {
		number := index + 1
		if skip[number] {
			if len(current) > 0 {
				out = append(out, Segment{Line: start, Text: strings.Join(current, "\n")})
				current = nil
			}
			continue
		}
		if len(current) == 0 {
			start = number
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		out = append(out, Segment{Line: start, Text: strings.Join(current, "\n")})
	}
	return out
}

func importSpec(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name + " " + spec.Path.Value
	}
	return spec.Path.Value
}
