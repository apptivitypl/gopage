package compile

import (
	"fmt"
	"strings"

	"github.com/apptivitypl/rill/internal/csrf"
	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/form"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/syntax"
)

const (
	FormComponent  = "Form"
	FieldComponent = "Field"
	ImageComponent = "Image"
	nameAttribute  = "name"
	labelAttribute = "label"
	typeAttribute  = "type"
	asAttribute    = "as"
	textareaTag    = "textarea"
)

var builtins = map[string]bool{FormComponent: true, FieldComponent: true, ImageComponent: true}

func Builtin(name string) bool {
	return builtins[name]
}

func BuiltinNames() []string {
	return []string{FieldComponent, FormComponent, ImageComponent}
}

func (b *builder) builtin(node *syntax.Component) {
	switch node.Name {
	case FormComponent:
		b.formComponent(node)
	case ImageComponent:
		b.imageComponent(node)
	default:
		b.fieldComponent(node)
	}
}

func (b *builder) formComponent(node *syntax.Component) {
	b.static("<form")
	if !hasAttribute(node.Attributes, "method") {
		b.static(` method="post"`)
	}
	for _, attribute := range node.Attributes {
		b.attribute(attribute)
	}
	b.static(`><input type="hidden" name="` + csrf.Field + `" value="`)
	b.emit(ir.Op{Kind: ir.OpText, A: b.rootPath(form.Root, "Token")})
	b.static(`">`)
	b.nodes(node.Children)
	b.static("</form>")
}

func (b *builder) fieldComponent(node *syntax.Component) {
	name, ok := literal(node.Attributes, nameAttribute)
	if !ok || !fieldName(name) {
		b.report(diag.C311, node.Span, "a field needs a literal name attribute",
			fmt.Sprintf(`write <%s name="Email" /> naming the form field it edits`, FieldComponent))
		return
	}
	b.recordClasses("field")
	b.static(`<div class="field"><label for="` + escapeAttribute(name) + `">`)
	b.label(node, name)
	b.static("</label>")
	b.control(node, name)
	b.fieldError(name)
	b.static("</div>")
}

func (b *builder) label(node *syntax.Component, name string) {
	if bound, ok := boundAttribute(node.Attributes, labelAttribute); ok {
		b.emit(ir.Op{Kind: ir.OpText, A: b.expr(bound)})
		return
	}
	if text, ok := literal(node.Attributes, labelAttribute); ok {
		b.static(escapeAttribute(text))
		return
	}
	b.static(escapeAttribute(name))
}

func boundAttribute(attributes []syntax.Attribute, name string) (syntax.Expr, bool) {
	for _, attribute := range attributes {
		if strings.EqualFold(attribute.Name, name) && attribute.Bound {
			return attribute.Value, true
		}
	}
	return nil, false
}

func (b *builder) control(node *syntax.Component, name string) {
	kind, _ := literal(node.Attributes, typeAttribute)
	if kind == "" {
		kind = "text"
	}
	if as, _ := literal(node.Attributes, asAttribute); as == textareaTag {
		b.static(`<textarea id="` + escapeAttribute(name) + `" name="` + escapeAttribute(name) + `"`)
		b.passthrough(node)
		b.static(">")
		b.emit(ir.Op{Kind: ir.OpText, A: b.rootPath(form.Root, "Values", name)})
		b.static("</textarea>")
		return
	}
	b.static(`<input id="` + escapeAttribute(name) + `" name="` + escapeAttribute(name) +
		`" type="` + escapeAttribute(kind) + `"`)
	b.passthrough(node)
	if kind == "checkbox" {
		b.static(` value="on"`)
		test := b.emit(ir.Op{Kind: ir.OpJumpIfFalse, A: b.rootPath(form.Root, "Values", name)})
		b.static(" checked")
		b.patch(test, b.here())
		b.static(">")
		return
	}
	b.static(` value="`)
	b.emit(ir.Op{Kind: ir.OpText, A: b.rootPath(form.Root, "Values", name)})
	b.static(`">`)
}

func (b *builder) passthrough(node *syntax.Component) {
	for _, attribute := range node.Attributes {
		if reserved(attribute.Name) {
			continue
		}
		b.attribute(attribute)
	}
}

func (b *builder) fieldError(name string) {
	message := b.rootPath(form.Root, "Errors", name)
	test := b.emit(ir.Op{Kind: ir.OpJumpIfFalse, A: message})
	b.recordClasses("field-error")
	b.static(`<p class="field-error">`)
	b.emit(ir.Op{Kind: ir.OpText, A: message})
	b.static("</p>")
	b.patch(test, b.here())
}

func (b *builder) rootPath(segments ...string) uint32 {
	return b.emitExpr(ir.ExprNode{Kind: ir.ExprPath, A: b.pathOf(segments)})
}

func fieldName(name string) bool {
	for i := range len(name) {
		c := name[i]
		letter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if letter || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return name != ""
}

func reserved(name string) bool {
	switch strings.ToLower(name) {
	case nameAttribute, labelAttribute, typeAttribute, asAttribute:
		return true
	default:
		return false
	}
}

func hasAttribute(attributes []syntax.Attribute, name string) bool {
	for _, attribute := range attributes {
		if strings.EqualFold(attribute.Name, name) {
			return true
		}
	}
	return false
}

func literal(attributes []syntax.Attribute, name string) (string, bool) {
	for _, attribute := range attributes {
		if !strings.EqualFold(attribute.Name, name) || attribute.Bound || attribute.Conditional {
			continue
		}
		if len(attribute.Parts) > 0 || len(attribute.Classes) > 0 {
			return "", false
		}
		return attribute.Text, attribute.Text != ""
	}
	return "", false
}
