package schema

import (
	"go/ast"
	"sort"
)

const contextType = "Ctx"

var reservedLoaders = map[string]bool{"Load": true, "Meta": true, "Submit": true}

func readDeferred(file *ast.File) []Field {
	var found []Field
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !fn.Name.IsExported() || reservedLoaders[fn.Name.Name] {
			continue
		}
		if !takesContext(fn.Type.Params) {
			continue
		}
		typ, ok := loaderResult(fn.Type.Results)
		if !ok {
			continue
		}
		found = append(found, Field{Name: fn.Name.Name, Type: typ, Deferred: true})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found
}

func takesContext(params *ast.FieldList) bool {
	if params == nil || len(params.List) != 1 {
		return false
	}
	pointer, ok := params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == contextType
}

func loaderResult(results *ast.FieldList) (Type, bool) {
	if results == nil || len(results.List) != 2 {
		return Type{}, false
	}
	if !isError(results.List[1].Type) {
		return Type{}, false
	}
	return readType(results.List[0].Type)
}

func isError(node ast.Expr) bool {
	ident, ok := node.(*ast.Ident)
	return ok && ident.Name == "error"
}

func attachDeferred(schema *Schema, fields []Field) {
	props, ok := schema.Structs[PropsName]
	if !ok {
		return
	}
	for _, field := range fields {
		if _, clash := props.Field(field.Name); clash {
			continue
		}
		props.Fields = append(props.Fields, field)
	}
	schema.Structs[PropsName] = props
}

func Deferred(model *Schema) []string {
	if model == nil {
		return nil
	}
	props, ok := model.Props()
	if !ok {
		return nil
	}
	var names []string
	for _, field := range props.Fields {
		if field.Deferred {
			names = append(names, field.Name)
		}
	}
	sort.Strings(names)
	return names
}
