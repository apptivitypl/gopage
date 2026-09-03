package syntax

import (
	"fmt"
	"strings"

	"github.com/sonquer/rill/internal/diag"
)

const ClassAttribute = "class"

func (p *parser) element(name string, span diag.Span) Node {
	node := &Element{Span: span, Name: name}
	for {
		switch p.tok.Kind {
		case KindTagSelfClose:
			node.SelfClosing = true
			node.Span = diag.Span{Start: span.Start, End: p.tok.Span.End}
			p.advance()
			return node
		case KindTagEnd:
			node.Span = diag.Span{Start: span.Start, End: p.tok.Span.End}
			p.advance()
			return node
		case KindColon:
			p.advance()
			if !p.elementAttribute(node, true) {
				return nil
			}
		case KindIdent:
			if !p.elementAttribute(node, false) {
				return nil
			}
		case KindEOF:
			p.report(diag.C310, span, fmt.Sprintf("<%s is never closed", name), "close the tag with > or />")
			return nil
		default:
			p.report(diag.C310, p.tok.Span,
				fmt.Sprintf("unexpected %s inside <%s", p.tok.Kind, name), "")
			p.skipTag()
			return nil
		}
	}
}

func (p *parser) elementAttribute(node *Element, bound bool) bool {
	if p.tok.Kind != KindIdent {
		p.report(diag.C310, p.tok.Span,
			fmt.Sprintf("expected an attribute name, found %s", p.tok.Kind), "")
		p.skipTag()
		return false
	}
	attribute := Attribute{Span: p.tok.Span, Name: p.tok.Text, Bound: bound}
	p.advance()

	if p.tok.Kind == KindQuestion {
		attribute.Conditional = true
		p.advance()
	}

	if p.tok.Kind != KindAssign {
		if bound || attribute.Conditional {
			p.report(diag.C310, attribute.Span,
				fmt.Sprintf("%s needs a value", attribute.Name),
				`the form is :name="expression" or name?="condition"`)
			p.skipTag()
			return false
		}
		node.Attributes = append(node.Attributes, attribute)
		return true
	}
	p.advance()

	if p.tok.Kind != KindString {
		p.report(diag.C310, p.tok.Span,
			fmt.Sprintf("expected a quoted value, found %s", p.tok.Kind), "")
		p.skipTag()
		return false
	}
	value := p.tok
	attribute.Span = diag.Span{Start: attribute.Span.Start, End: value.Span.End}
	attribute.Text = value.Value
	p.advance()

	switch {
	case bound && attribute.Name == ClassAttribute && strings.HasPrefix(strings.TrimSpace(value.Value), "{"):
		classes, ok := p.classMap(value)
		if !ok {
			return false
		}
		attribute.Classes = classes
	case bound || attribute.Conditional:
		expr, ok := ParseExpr(p.file, value.Value, value.Span.Start+1, p.bag)
		if !ok {
			return false
		}
		attribute.Value = expr
	default:
		attribute.Parts = p.valueParts(value)
	}
	node.Attributes = append(node.Attributes, attribute)
	return true
}

func (p *parser) valueParts(value Token) []Node {
	if !strings.Contains(value.Value, "{{") {
		return nil
	}
	inner := &diag.Bag{}
	sub := &parser{lexer: NewLexer(value.Value), file: p.file, bag: inner}
	sub.advance()
	nodes, _, _ := sub.block(nil)
	offset := value.Span.Start + 1
	for _, d := range inner.Items() {
		d.Span = shiftSpan(d.Span, offset)
		p.bag.Add(d)
	}
	return shiftNodes(nodes, offset)
}

func shiftNodes(nodes []Node, offset uint32) []Node {
	for _, node := range nodes {
		switch n := node.(type) {
		case *Text:
			n.Span = shiftSpan(n.Span, offset)
		case *Interpolation:
			n.Span = shiftSpan(n.Span, offset)
			shift(n.Expr, offset)
		}
	}
	return nodes
}

func (p *parser) classMap(value Token) ([]ClassEntry, bool) {
	body := strings.TrimSpace(value.Value)
	body = strings.TrimSuffix(strings.TrimPrefix(body, "{"), "}")
	offset := value.Span.Start + 1

	var entries []ClassEntry
	for _, item := range splitTop(body) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, condition, ok := strings.Cut(item, ":")
		if !ok {
			p.report(diag.C310, value.Span,
				fmt.Sprintf("class entry %q has no condition", item),
				`the form is :class="{ 'is-active': Selected }"`)
			return nil, false
		}
		name = strings.Trim(strings.TrimSpace(name), `'"`)
		if name == "" {
			p.report(diag.C310, value.Span, "a class entry needs a name", "")
			return nil, false
		}
		expr, ok := ParseExpr(p.file, condition, offset, p.bag)
		if !ok {
			return nil, false
		}
		entries = append(entries, ClassEntry{Name: name, Cond: expr})
	}
	return entries, true
}

func splitTop(body string) []string {
	var parts []string
	var depth int
	var quote byte
	start := 0
	for i := range len(body) {
		c := body[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, body[start:i])
			start = i + 1
		}
	}
	return append(parts, body[start:])
}
