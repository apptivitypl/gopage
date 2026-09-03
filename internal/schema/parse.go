package schema

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"

	"github.com/apptivitypl/rill/internal/diag"
)

const (
	tagKey     = "rill"
	tagDefault = "default"
	tagSlot    = "slot"
	tagRest    = "rest"
	tagPrivate = "private"
)

var scalars = map[string]Kind{
	"bool":    KindBool,
	"string":  KindString,
	"int":     KindInt,
	"int8":    KindInt,
	"int16":   KindInt,
	"int32":   KindInt,
	"int64":   KindInt,
	"uint":    KindInt,
	"uint8":   KindInt,
	"uint16":  KindInt,
	"uint32":  KindInt,
	"uint64":  KindInt,
	"rune":    KindInt,
	"byte":    KindInt,
	"float32": KindFloat,
	"float64": KindFloat,
}

type Source struct {
	File string
	Code string
	Line int
}

func Parse(sources []Source, bag *diag.Bag) *Schema {
	schema := &Schema{Structs: map[string]Struct{}, Enums: map[string]Enum{}}
	for _, source := range sources {
		parseOne(schema, source, bag)
	}
	markEnums(schema)
	validate(schema, sources, bag)
	return schema
}

func parseOne(schema *Schema, source Source, bag *diag.Bag) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, source.File, wrap(source.Code), parser.SkipObjectResolution)
	if err != nil {
		bag.Add(diag.New(diag.C301, source.File, diag.Span{},
			fmt.Sprintf("cannot read the Go block: %v", err)).
			WithHelp("the block between the fences must be valid Go: imports and declarations"))
		return
	}
	for _, decl := range file.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch generic.Tok {
		case token.TYPE:
			readTypes(schema, generic, source, bag)
		case token.CONST:
			readConsts(schema, generic)
		}
	}
	attach(schema, readMethods(file))
	attachDeferred(schema, readDeferred(file))
}

func readTypes(schema *Schema, decl *ast.GenDecl, source Source, bag *diag.Bag) {
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if structType, ok := typeSpec.Type.(*ast.StructType); ok {
			schema.add(readStruct(typeSpec.Name.Name, structType, source, bag))
			continue
		}
		if ident, ok := typeSpec.Type.(*ast.Ident); ok && !typeSpec.Assign.IsValid() {
			if kind, isScalar := scalars[ident.Name]; isScalar {
				schema.Enums[typeSpec.Name.Name] = Enum{Name: typeSpec.Name.Name, Underlying: kind}
			}
		}
	}
}

func readConsts(schema *Schema, decl *ast.GenDecl) {
	var current string
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if ident, ok := valueSpec.Type.(*ast.Ident); ok {
			current = ident.Name
		}
		enum, known := schema.Enums[current]
		if !known {
			continue
		}
		for _, name := range valueSpec.Names {
			if name.IsExported() {
				enum.Members = append(enum.Members, name.Name)
			}
		}
		schema.Enums[current] = enum
	}
}

func wrap(code string) string {
	return "package rillframe\n" + code
}

func (s *Schema) add(value Struct) {
	if _, exists := s.Structs[value.Name]; !exists {
		s.Order = append(s.Order, value.Name)
	}
	s.Structs[value.Name] = value
}

func readStruct(name string, node *ast.StructType, source Source, bag *diag.Bag) Struct {
	value := Struct{Name: name}
	for _, field := range node.Fields.List {
		if len(field.Names) == 0 {
			bag.Add(diag.New(diag.C302, source.File, diag.Span{},
				fmt.Sprintf("%s embeds a type", name)).
				WithHelp("props hold named fields only; give the field a name"))
			continue
		}
		fieldType, ok := readType(field.Type)
		if !ok {
			bag.Add(diag.New(diag.C302, source.File, diag.Span{},
				fmt.Sprintf("%s.%s uses %s, which props cannot carry", name, field.Names[0].Name, describe(field.Type))).
				WithHelp("props carry bools, numbers, strings, slices, pointers and structs declared in the same block; " +
					"convert anything else in Load and pass a simple type"))
			continue
		}
		for _, ident := range field.Names {
			if !ident.IsExported() {
				bag.Add(diag.New(diag.C303, source.File, diag.Span{},
					fmt.Sprintf("%s.%s is unexported", name, ident.Name)).
					WithHelp("templates read exported fields; start the name with a capital letter"))
				continue
			}
			value.Fields = append(value.Fields, withTag(Field{Name: ident.Name, Type: fieldType}, field.Tag))
		}
	}
	return value
}

func withTag(field Field, tag *ast.BasicLit) Field {
	if tag == nil {
		return field
	}
	text := strings.Trim(tag.Value, "`")
	options := reflect.StructTag(text).Get(tagKey)
	for option := range strings.SplitSeq(options, ",") {
		name, value, _ := strings.Cut(strings.TrimSpace(option), "=")
		switch name {
		case tagDefault:
			field.Default = value
		case tagSlot:
			field.Slot = true
		case tagRest:
			field.Rest = true
		case tagPrivate:
			field.Private = true
		}
	}
	return field
}

func readType(node ast.Expr) (Type, bool) {
	switch n := node.(type) {
	case *ast.Ident:
		if kind, ok := scalars[n.Name]; ok {
			return Type{Kind: kind, Name: n.Name}, true
		}
		return Type{Kind: KindStruct, Name: n.Name}, true
	case *ast.SelectorExpr:
		return Type{}, false
	case *ast.ArrayType:
		if n.Len != nil {
			return Type{}, false
		}
		elem, ok := readType(n.Elt)
		if !ok {
			return Type{}, false
		}
		return Type{Kind: KindSlice, Elem: &elem}, true
	case *ast.StarExpr:
		elem, ok := readType(n.X)
		if !ok {
			return Type{}, false
		}
		return Type{Kind: KindOptional, Elem: &elem}, true
	default:
		return Type{}, false
	}
}

func describe(node ast.Expr) string {
	switch n := node.(type) {
	case *ast.MapType:
		return "a map"
	case *ast.InterfaceType:
		return "an interface"
	case *ast.FuncType:
		return "a function"
	case *ast.ChanType:
		return "a channel"
	case *ast.SelectorExpr:
		return "a type from another package"
	case *ast.ArrayType:
		if n.Len != nil {
			return "a fixed-size array"
		}
		return "an unsupported slice element"
	default:
		return "an unsupported type"
	}
}

func markEnums(schema *Schema) {
	for name, value := range schema.Structs {
		for i := range value.Fields {
			markType(schema, &value.Fields[i].Type)
		}
		schema.Structs[name] = value
	}
}

func markType(schema *Schema, fieldType *Type) {
	switch fieldType.Kind {
	case KindSlice, KindOptional:
		markType(schema, fieldType.Elem)
	case KindStruct:
		enum, ok := schema.Enums[fieldType.Name]
		if !ok {
			return
		}
		if len(enum.Members) == 0 {
			fieldType.Kind = enum.Underlying
			return
		}
		fieldType.Kind = KindEnum
	}
}

func validate(schema *Schema, sources []Source, bag *diag.Bag) {
	file := ""
	if len(sources) > 0 {
		file = sources[0].File
	}
	for _, name := range schema.Order {
		for _, field := range schema.Structs[name].Fields {
			checkKnown(schema, name, field, field.Type, file, bag)
		}
	}
}

func checkKnown(schema *Schema, owner string, field Field, fieldType Type, file string, bag *diag.Bag) {
	switch fieldType.Kind {
	case KindSlice, KindOptional:
		checkKnown(schema, owner, field, *fieldType.Elem, file, bag)
	case KindStruct:
		if schema.Has(fieldType.Name) {
			return
		}
		message := fmt.Sprintf("%s.%s refers to %s, which is not declared here", owner, field.Name, fieldType.Name)
		help := "declare the struct in the same block, or convert the value in Load"
		if suggestion := Suggest(fieldType.Name, schema.Order); suggestion != "" {
			help = fmt.Sprintf("did you mean %s?", suggestion)
		}
		bag.Add(diag.New(diag.C304, file, diag.Span{}, message).WithHelp(help))
	}
}
