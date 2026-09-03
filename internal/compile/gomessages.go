package compile

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"github.com/sonquer/rill/internal/diag"
)

const (
	translateMethod = "T"
	countMethod     = "Count"
)

type goMessage struct {
	Key    string
	Plural bool
	Offset uint32
}

func GoMessages(code string) []goMessage {
	if code == "" {
		return nil
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "frontmatter.go", "package frontmatter\n"+code, 0)
	if err != nil {
		return nil
	}
	var found []goMessage
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		plural := selector.Sel.Name == countMethod
		if selector.Sel.Name != translateMethod && !plural {
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		key, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		found = append(found, goMessage{Key: key, Plural: plural})
		return true
	})
	return found
}

func frontmatterSpan(template Template) diag.Span {
	if template.Document != nil && template.Document.Frontmatter != nil {
		return template.Document.Frontmatter.Span
	}
	return diag.At(0)
}

func (s *state) goMessages(template Template) {
	if s.messages == nil {
		return
	}
	for _, message := range GoMessages(template.Frontmatter) {
		index := s.messages.intern(message.Key)
		s.messages.uses = append(s.messages.uses, messageUse{
			index:  index,
			plural: message.Plural,
			file:   template.File,
			span:   frontmatterSpan(template),
		})
	}
}
