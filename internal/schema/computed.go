package schema

import (
	"go/ast"
	"sort"
)

type method struct {
	Receiver string
	Field    Field
}

func readMethods(file *ast.File) []method {
	var found []method
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || !fn.Name.IsExported() {
			continue
		}
		receiver, ok := receiverName(fn.Recv)
		if !ok {
			continue
		}
		if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
			continue
		}
		if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 || len(fn.Type.Results.List[0].Names) > 1 {
			continue
		}
		typ, ok := readType(fn.Type.Results.List[0].Type)
		if !ok {
			continue
		}
		found = append(found, method{
			Receiver: receiver,
			Field:    Field{Name: fn.Name.Name, Type: typ, Computed: true},
		})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].Field.Name < found[j].Field.Name })
	return found
}

func receiverName(list *ast.FieldList) (string, bool) {
	if len(list.List) != 1 {
		return "", false
	}
	switch node := list.List[0].Type.(type) {
	case *ast.Ident:
		return node.Name, true
	case *ast.StarExpr:
		ident, ok := node.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		return ident.Name, true
	default:
		return "", false
	}
}

func attach(schema *Schema, methods []method) {
	for _, entry := range methods {
		owner, ok := schema.Structs[entry.Receiver]
		if !ok {
			continue
		}
		if _, clash := owner.Field(entry.Field.Name); clash {
			continue
		}
		owner.Fields = append(owner.Fields, entry.Field)
		schema.Structs[entry.Receiver] = owner
	}
}
