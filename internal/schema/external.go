package schema

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/apptivitypl/gopage/internal/diag"
)

const (
	wrapperPrefix = "ext"
	maxDepth      = 8
)

type Packages struct {
	FS     fs.FS
	Module string
}

func (p Packages) known() bool {
	return p.FS != nil && p.Module != ""
}

func (p Packages) dir(importPath string) (string, bool) {
	if importPath == p.Module {
		return ".", true
	}
	rest, inside := strings.CutPrefix(importPath, p.Module+"/")
	if !inside || rest == "" {
		return "", false
	}
	return rest, true
}

func (p Packages) sources(importPath string) []Source {
	dir, ok := p.dir(importPath)
	if !ok {
		return nil
	}
	entries, err := fs.ReadDir(p.FS, dir)
	if err != nil {
		return nil
	}
	var sources []Source
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := fs.ReadFile(p.FS, path.Join(dir, name))
		if err != nil {
			continue
		}
		sources = append(sources, Source{File: path.Join(dir, name), Code: string(data)})
	}
	return sources
}

func WrapperName(importPath, typeName string) string {
	return wrapperPrefix + title(path.Base(importPath)) + title(typeName)
}

func title(text string) string {
	letters := []rune(text)
	for index, letter := range letters {
		if !unicode.IsLetter(letter) && !unicode.IsDigit(letter) {
			letters[index] = '_'
		}
	}
	if len(letters) == 0 {
		return ""
	}
	letters[0] = unicode.ToUpper(letters[0])
	return strings.ReplaceAll(string(letters), "_", "")
}

func fileImports(file *ast.File) map[string]string {
	found := map[string]string{}
	for _, entry := range file.Imports {
		target, err := strconv.Unquote(entry.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(target)
		if entry.Name != nil {
			name = entry.Name.Name
		}
		found[name] = target
	}
	return found
}

type resolver struct {
	schema   *Schema
	packages Packages
	imports  map[string]string
	pkg      string
	alias    string
	source   Source
	bag      *diag.Bag
	depth    int
}

func (r resolver) selector(node *ast.SelectorExpr) (Type, bool) {
	pkg, ok := node.X.(*ast.Ident)
	if !ok {
		return Type{}, false
	}
	if pkg.Name+"."+node.Sel.Name == TimeType {
		return Type{Kind: KindTime, Name: TimeType}, true
	}
	target, known := r.imports[pkg.Name]
	if !known || !r.packages.known() || r.depth >= maxDepth {
		return Type{}, false
	}
	return r.adopt(target, pkg.Name, node.Sel.Name)
}

func (r resolver) sibling(name string) (Type, bool) {
	if r.pkg == "" || r.depth >= maxDepth {
		return Type{}, false
	}
	return r.adopt(r.pkg, r.alias, name)
}

func (r resolver) adopt(importPath, alias, name string) (Type, bool) {
	wrapper := WrapperName(importPath, name)
	if _, done := r.schema.Structs[wrapper]; done {
		return Type{Kind: KindStruct, Name: wrapper}, true
	}
	for _, source := range r.packages.sources(importPath) {
		file, err := parser.ParseFile(token.NewFileSet(), source.File, source.Code, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		node, found := declaration(file, name)
		if !found {
			continue
		}
		inner := resolver{
			schema:   r.schema,
			packages: r.packages,
			imports:  fileImports(file),
			pkg:      importPath,
			alias:    alias,
			source:   source,
			bag:      r.bag,
			depth:    r.depth + 1,
		}
		r.schema.add(Struct{Name: wrapper, External: true, Local: alias + "." + name})
		value := readStruct(inner, wrapper, node)
		value.External = true
		value.Local = alias + "." + name
		r.schema.add(value)
		return Type{Kind: KindStruct, Name: wrapper}, true
	}
	return Type{}, false
}

func declaration(file *ast.File, name string) (*ast.StructType, bool) {
	for _, decl := range file.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}
		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != name {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			return structType, ok
		}
	}
	return nil, false
}
