package compile

import (
	"strings"

	"github.com/sonquer/rill/internal/ir"
	"github.com/sonquer/rill/internal/syntax"
)

func (b *builder) element(node *syntax.Element) {
	b.static("<" + node.Name)
	for _, attribute := range node.Attributes {
		b.attribute(attribute)
	}
	if node.SelfClosing {
		b.static("/>")
		return
	}
	b.static(">")
}

func (b *builder) attribute(attribute syntax.Attribute) {
	if strings.EqualFold(attribute.Name, ClassAttribute) {
		b.classes(attribute)
	}
	switch {
	case len(attribute.Classes) > 0:
		b.classMap(attribute)
	case attribute.Conditional:
		b.conditional(attribute)
	case attribute.Bound:
		b.static(" " + attribute.Name + `="`)
		b.emit(ir.Op{Kind: valueOp(attribute.Name), A: b.expr(attribute.Value)})
		b.static(`"`)
	case attribute.Parts != nil:
		b.static(" " + attribute.Name + `="`)
		b.attributeParts(attribute)
		b.static(`"`)
	case attribute.Text == "" && !strings.Contains(attribute.Name, "="):
		b.static(" " + attribute.Name)
	default:
		b.static(" " + attribute.Name + `="` + escapeAttribute(attribute.Text) + `"`)
	}
}

var urlAttributes = map[string]bool{
	"href":       true,
	"src":        true,
	"srcset":     true,
	"action":     true,
	"formaction": true,
	"poster":     true,
	"cite":       true,
	"ping":       true,
	"manifest":   true,
	"data":       true,
	"background": true,
	"xlink:href": true,
}

func urlAttribute(name string) bool {
	return urlAttributes[strings.ToLower(name)]
}

func valueOp(name string) ir.OpKind {
	if urlAttribute(name) {
		return ir.OpURL
	}
	return ir.OpText
}

func (b *builder) attributeParts(attribute syntax.Attribute) {
	if !urlAttribute(attribute.Name) {
		b.nodes(attribute.Parts)
		return
	}
	for i, part := range attribute.Parts {
		node, ok := part.(*syntax.Interpolation)
		if ok && i == 0 {
			b.emit(ir.Op{Kind: ir.OpURL, A: b.expr(node.Expr)})
			continue
		}
		b.node(part)
	}
}

func (b *builder) conditional(attribute syntax.Attribute) {
	test := b.emit(ir.Op{Kind: ir.OpJumpIfFalse, A: b.expr(attribute.Value)})
	b.static(" " + attribute.Name)
	b.patch(test, b.here())
}

func (b *builder) classMap(attribute syntax.Attribute) {
	b.static(` class="`)
	for _, entry := range attribute.Classes {
		test := b.emit(ir.Op{Kind: ir.OpJumpIfFalse, A: b.expr(entry.Cond)})
		b.static(escapeAttribute(entry.Name) + " ")
		b.patch(test, b.here())
	}
	b.static(`"`)
}

var attributeEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&#34;",
	"'", "&#39;",
)

func escapeAttribute(value string) string {
	return attributeEscaper.Replace(value)
}
