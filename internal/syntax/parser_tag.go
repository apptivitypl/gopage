package syntax

import (
	"fmt"

	"github.com/sonquer/rill/internal/diag"
)

const templateTag = "Template"

func (p *parser) component(name string, span diag.Span) Node {
	node := &Component{Span: span, Name: name}
	slotName := ""

	for {
		switch p.tok.Kind {
		case KindTagSelfClose:
			node.Span = diag.Span{Start: span.Start, End: p.tok.Span.End}
			p.advance()
			return node
		case KindTagEnd:
			p.advance()
			return p.componentBody(node, slotName)
		case KindHash:
			p.advance()
			if p.tok.Kind != KindIdent {
				p.report(diag.C306, p.tok.Span,
					fmt.Sprintf("expected a slot name after #, found %s", p.tok.Kind),
					"the form is <Template #footer>")
				p.skipTag()
				return nil
			}
			slotName = p.tok.Text
			p.advance()
		case KindColon:
			p.advance()
			if !p.attribute(node, true) {
				return nil
			}
		case KindIdent:
			if !p.attribute(node, false) {
				return nil
			}
		case KindEOF:
			p.report(diag.C306, span, fmt.Sprintf("<%s is never closed", name), "close the tag with > or />")
			return nil
		default:
			p.report(diag.C306, p.tok.Span,
				fmt.Sprintf("unexpected %s inside <%s", p.tok.Kind, name), "")
			p.skipTag()
			return nil
		}
	}
}

func (p *parser) attribute(node *Component, bound bool) bool {
	if p.tok.Kind != KindIdent {
		p.report(diag.C306, p.tok.Span,
			fmt.Sprintf("expected an attribute name, found %s", p.tok.Kind), "")
		p.skipTag()
		return false
	}
	attribute := Attribute{Span: p.tok.Span, Name: p.tok.Text, Bound: bound}
	p.advance()

	if p.tok.Kind != KindAssign {
		if bound {
			p.report(diag.C306, attribute.Span,
				fmt.Sprintf("bound attribute :%s needs a value", attribute.Name),
				`the form is :name="expression"`)
			p.skipTag()
			return false
		}
		node.Attributes = append(node.Attributes, attribute)
		return true
	}
	p.advance()

	if p.tok.Kind != KindString {
		p.report(diag.C306, p.tok.Span,
			fmt.Sprintf("expected a quoted value, found %s", p.tok.Kind), "")
		p.skipTag()
		return false
	}
	attribute.Span = diag.Span{Start: attribute.Span.Start, End: p.tok.Span.End}
	attribute.Text = p.tok.Value
	if bound {
		value, ok := ParseExpr(p.file, p.tok.Value, p.tok.Span.Start+1, p.bag)
		if !ok {
			p.skipTag()
			return false
		}
		attribute.Value = value
	}
	node.Attributes = append(node.Attributes, attribute)
	p.advance()
	return true
}

func (p *parser) componentBody(node *Component, slotName string) Node {
	nodes, closed := p.untilClose(node.Name)
	if !closed {
		p.report(diag.C306, node.Span,
			fmt.Sprintf("<%s> is never closed", node.Name),
			fmt.Sprintf("close it with </%s>", node.Name))
		return nil
	}
	if node.Name == templateTag {
		if slotName == "" {
			p.report(diag.C306, node.Span, "<Template> needs a slot name", "the form is <Template #footer>")
			return nil
		}
		return &slotFill{name: slotName, nodes: nodes}
	}
	for _, child := range nodes {
		if fill, ok := child.(*slotFill); ok {
			node.Slots = append(node.Slots, Slot{Name: fill.name, Nodes: fill.nodes})
			continue
		}
		node.Children = append(node.Children, child)
	}
	return node
}

type slotFill struct {
	name  string
	nodes []Node
}

func (s *slotFill) NodeSpan() diag.Span { return diag.Span{} }

func (p *parser) untilClose(name string) ([]Node, bool) {
	var nodes []Node
	for {
		switch p.tok.Kind {
		case KindEOF:
			return nodes, false
		case KindComponentClose:
			if p.tok.Text != name {
				p.report(diag.C306, p.tok.Span,
					fmt.Sprintf("</%s> closes a tag that is not open here", p.tok.Text),
					fmt.Sprintf("the open tag is <%s>", name))
				p.advance()
				continue
			}
			p.advance()
			return nodes, true
		default:
			node, stop := p.bodyNode()
			if stop {
				return nodes, false
			}
			if node != nil {
				nodes = append(nodes, node)
			}
		}
	}
}

func (p *parser) skipTag() {
	for {
		switch p.tok.Kind {
		case KindEOF, KindText, KindComponentOpen:
			return
		case KindTagEnd, KindTagSelfClose:
			p.advance()
			return
		default:
			p.advance()
		}
	}
}

func ParseExpr(file, source string, offset uint32, bag *diag.Bag) (Expr, bool) {
	inner := &diag.Bag{}
	p := &parser{lexer: NewLexer(source), file: file, bag: inner}
	p.lexer.mode = modeInterp
	p.advance()
	value, ok := p.expr()
	if ok && p.tok.Kind != KindEOF {
		ok = false
		p.report(diag.C201, p.tok.Span, fmt.Sprintf("unexpected %s after the expression", p.tok.Kind), "")
	}
	for _, d := range inner.Items() {
		d.Span = diag.Span{Start: d.Span.Start + offset, End: d.Span.End + offset}
		bag.Add(d)
	}
	if !ok {
		return nil, false
	}
	return shift(value, offset), true
}

func shift(node Expr, offset uint32) Expr {
	switch n := node.(type) {
	case *Path:
		n.Span = shiftSpan(n.Span, offset)
	case *StringLit:
		n.Span = shiftSpan(n.Span, offset)
	case *IntLit:
		n.Span = shiftSpan(n.Span, offset)
	case *FloatLit:
		n.Span = shiftSpan(n.Span, offset)
	case *BoolLit:
		n.Span = shiftSpan(n.Span, offset)
	case *Unary:
		n.Span = shiftSpan(n.Span, offset)
		shift(n.Operand, offset)
	case *Binary:
		n.Span = shiftSpan(n.Span, offset)
		shift(n.Left, offset)
		shift(n.Right, offset)
	case *Index:
		n.Span = shiftSpan(n.Span, offset)
		shift(n.Base, offset)
		shift(n.Index, offset)
	}
	return node
}

func shiftSpan(span diag.Span, offset uint32) diag.Span {
	return diag.Span{Start: span.Start + offset, End: span.End + offset}
}
