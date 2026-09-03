package schema

import "strings"

type Kind uint8

const (
	KindInvalid Kind = iota
	KindBool
	KindInt
	KindFloat
	KindString
	KindSlice
	KindOptional
	KindStruct
	KindEnum
)

var kindNames = map[Kind]string{
	KindInvalid:  "unsupported",
	KindBool:     "bool",
	KindInt:      "whole number",
	KindFloat:    "number",
	KindString:   "string",
	KindSlice:    "slice",
	KindOptional: "optional",
	KindStruct:   "struct",
	KindEnum:     "enum",
}

func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return "unknown kind"
}

type Type struct {
	Kind Kind
	Name string
	Elem *Type
}

func (t Type) GoString() string {
	switch t.Kind {
	case KindSlice:
		return "[]" + t.Elem.GoString()
	case KindOptional:
		return "*" + t.Elem.GoString()
	case KindStruct:
		return t.Name
	default:
		return t.Name
	}
}

func (t Type) Scalar() bool {
	switch t.Kind {
	case KindBool, KindInt, KindFloat, KindString:
		return true
	default:
		return false
	}
}

type Field struct {
	Name     string
	Type     Type
	Default  string
	Slot     bool
	Rest     bool
	Computed bool
	Deferred bool
	Private  bool
}

type Struct struct {
	Name   string
	Fields []Field
}

func (s Struct) Field(name string) (Field, bool) {
	for _, field := range s.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}

func (s Struct) FieldNames() []string {
	names := make([]string, 0, len(s.Fields))
	for _, field := range s.Fields {
		names = append(names, field.Name)
	}
	return names
}

type Enum struct {
	Name       string
	Underlying Kind
	Members    []string
}

type Schema struct {
	Structs map[string]Struct
	Enums   map[string]Enum
	Order   []string
}

func (s *Schema) Enum(name string) (Enum, bool) {
	value, ok := s.Enums[name]
	return value, ok
}

func (s *Schema) Get(name string) (Struct, bool) {
	value, ok := s.Structs[name]
	return value, ok
}

func (s *Schema) Has(name string) bool {
	_, ok := s.Structs[name]
	return ok
}

const PropsName = "Props"

func (s *Schema) Props() (Struct, bool) {
	return s.Get(PropsName)
}

func (s *Schema) Private(root string, path []string) bool {
	current, ok := s.Get(root)
	if !ok {
		return false
	}
	for i, segment := range path {
		field, ok := current.Field(segment)
		if !ok {
			return false
		}
		if field.Private {
			return true
		}
		if i == len(path)-1 {
			return false
		}
		next := field.Type
		for next.Kind == KindOptional || next.Kind == KindSlice {
			next = *next.Elem
		}
		if next.Kind != KindStruct {
			return false
		}
		if current, ok = s.Get(next.Name); !ok {
			return false
		}
	}
	return false
}

func (s *Schema) Resolve(root string, path []string) (Type, bool) {
	current, ok := s.Get(root)
	if !ok {
		return Type{}, false
	}
	var last Type
	for i, segment := range path {
		field, ok := current.Field(segment)
		if !ok {
			return Type{}, false
		}
		last = field.Type
		if i == len(path)-1 {
			return last, true
		}
		next := last
		for next.Kind == KindOptional || next.Kind == KindSlice {
			next = *next.Elem
		}
		if next.Kind != KindStruct {
			return Type{}, false
		}
		if current, ok = s.Get(next.Name); !ok {
			return Type{}, false
		}
	}
	return last, false
}

func Suggest(name string, candidates []string) string {
	best, bestDistance := "", len(name)/2+2
	for _, candidate := range candidates {
		if distance := editDistance(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

func editDistance(a, b string) int {
	if strings.EqualFold(a, b) {
		return 1
	}
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}
