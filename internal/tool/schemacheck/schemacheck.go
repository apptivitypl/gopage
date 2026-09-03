package schemacheck

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const Path = "schema/rill.schema.json"

type IssueKind string

const (
	Undocumented IssueKind = "undocumented"
	Orphan       IssueKind = "orphan"
	Untyped      IssueKind = "untyped"
)

type Issue struct {
	Kind  IssueKind
	Field string
	Want  string
	Got   string
}

func (i Issue) Message() string {
	switch i.Kind {
	case Undocumented:
		return fmt.Sprintf("%s: in the config struct but not in %s", i.Field, Path)
	case Orphan:
		return fmt.Sprintf("%s: in %s but not in the config struct", i.Field, Path)
	default:
		return fmt.Sprintf("%s: declared as %q, want %q", i.Field, i.Got, i.Want)
	}
}

type node struct {
	Type       string          `json:"type"`
	Properties map[string]node `json:"properties"`
	Items      *node           `json:"items"`
}

func Check(schema []byte, config any) ([]Issue, error) {
	var root node
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", Path, err)
	}
	var issues []Issue
	walk("", reflect.TypeOf(config), root, &issues)
	sort.Slice(issues, func(a, b int) bool { return issues[a].Field < issues[b].Field })
	return issues, nil
}

func walk(prefix string, t reflect.Type, n node, issues *[]Issue) {
	seen := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		name, ok := jsonName(t.Field(i))
		if !ok {
			continue
		}
		seen[name] = true
		path := join(prefix, name)
		child, declared := n.Properties[name]
		if !declared {
			*issues = append(*issues, Issue{Kind: Undocumented, Field: path})
			continue
		}
		field := t.Field(i).Type
		if want := jsonType(field); want != "" && child.Type != want {
			*issues = append(*issues, Issue{Kind: Untyped, Field: path, Want: want, Got: child.Type})
			continue
		}
		switch field.Kind() {
		case reflect.Struct:
			walk(path, field, child, issues)
		case reflect.Slice:
			if field.Elem().Kind() == reflect.Struct && child.Items != nil {
				walk(path+"[]", field.Elem(), *child.Items, issues)
			}
		}
	}
	for name := range n.Properties {
		if !seen[name] {
			*issues = append(*issues, Issue{Kind: Orphan, Field: join(prefix, name)})
		}
	}
}

func jsonName(f reflect.StructField) (string, bool) {
	if f.PkgPath != "" {
		return "", false
	}
	tag, ok := f.Tag.Lookup("json")
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" || name == "-" {
		return "", false
	}
	return name, true
}

func jsonType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice:
		return "array"
	case reflect.Struct:
		return "object"
	default:
		return ""
	}
}

func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
