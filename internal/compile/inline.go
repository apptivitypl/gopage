package compile

import (
	"fmt"
	"slices"
	"strings"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/schema"
	"github.com/apptivitypl/rill/internal/syntax"
)

const maxComponentDepth = 32

func (b *builder) component(node *syntax.Component) {
	if Builtin(node.Name) {
		b.builtin(node)
		return
	}
	component, ok := b.components[node.Name]
	if !ok {
		b.report(diag.C307, node.Span, fmt.Sprintf("no component named %s", node.Name),
			b.componentHelp(node.Name))
		return
	}
	if slices.Contains(b.active, node.Name) {
		b.report(diag.C307, node.Span,
			fmt.Sprintf("%s renders itself through %s", node.Name, strings.Join(b.active, " -> ")),
			"components cannot be recursive; move the repetition into a loop")
		return
	}
	if len(b.active) >= maxComponentDepth {
		b.report(diag.C307, node.Span,
			fmt.Sprintf("components nest more than %d deep", maxComponentDepth), "")
		return
	}
	if strategy, ok := literal(node.Attributes, clientAttribute); ok {
		b.island(node, component, strategy)
		return
	}
	b.inline(node, component)
}

func (b *builder) inline(node *syntax.Component, component Component) {
	arguments := b.arguments(node, component)
	callerFloor := b.floor
	base := len(b.locals)
	for _, argument := range arguments {
		slot := b.declare(argument.name)
		b.emit(ir.Op{Kind: ir.OpLet, A: slot, B: argument.expr})
	}

	b.floor = base
	b.frames = append(b.frames, frame{floor: callerFloor, children: node.Children, slots: node.Slots})
	b.active = append(b.active, node.Name)
	callerFile := b.file
	callerScript := b.clientSeen
	b.file = component.File
	b.clientSeen = false
	b.nested++

	b.nodes(component.Document.Nodes)

	b.nested--
	b.clientSeen = callerScript
	b.file = callerFile
	b.active = b.active[:len(b.active)-1]
	b.frames = b.frames[:len(b.frames)-1]
	b.floor = callerFloor
	b.locals = b.locals[:base]
}

type argument struct {
	name string
	expr uint32
}

func (b *builder) arguments(node *syntax.Component, component Component) []argument {
	fields := component.Fields()
	given := map[string]syntax.Attribute{}
	for _, attribute := range node.Attributes {
		given[attribute.Name] = attribute
	}
	for name, attribute := range given {
		if reservedIslandAttribute(name) {
			continue
		}
		if !hasField(fields, name) {
			b.report(diag.C308, attribute.Span,
				fmt.Sprintf("%s has no prop %s", node.Name, name),
				propHelp(name, fields))
		}
	}

	arguments := make([]argument, 0, len(fields))
	for _, field := range fields {
		attribute, ok := given[field.Name]
		switch {
		case ok:
			arguments = append(arguments, argument{name: field.Name, expr: b.attributeValue(attribute, field)})
		case field.Default != "":
			arguments = append(arguments, argument{name: field.Name, expr: b.defaultValue(field)})
		case field.Type.Kind == schema.KindBool:
			arguments = append(arguments, argument{name: field.Name, expr: b.boolean(false)})
		case field.Slot || field.Rest || field.Type.Kind == schema.KindOptional:
			arguments = append(arguments, argument{name: field.Name, expr: b.constant(ir.Const{Kind: ir.ConstString}, "s")})
		default:
			b.report(diag.C308, node.Span,
				fmt.Sprintf("%s needs the prop %s", node.Name, field.Name),
				fmt.Sprintf("pass it as %s=\"...\" or :%s=\"expression\"", field.Name, field.Name))
			arguments = append(arguments, argument{name: field.Name, expr: b.constant(ir.Const{Kind: ir.ConstString}, "s")})
		}
	}
	return arguments
}

func (b *builder) attributeValue(attribute syntax.Attribute, field schema.Field) uint32 {
	if attribute.Bound {
		return b.expr(attribute.Value)
	}
	if attribute.Value == nil && attribute.Text == "" && field.Type.Kind == schema.KindBool {
		return b.boolean(true)
	}
	return b.literal(attribute.Text, field)
}

func (b *builder) defaultValue(field schema.Field) uint32 {
	return b.literal(field.Default, field)
}

func (b *builder) literal(text string, field schema.Field) uint32 {
	switch field.Type.Kind {
	case schema.KindInt:
		return b.integer(text)
	case schema.KindFloat:
		return b.real(text)
	case schema.KindBool:
		return b.boolean(text == "true" || text == "")
	default:
		return b.constant(ir.Const{Kind: ir.ConstString, Str: text}, "s"+text)
	}
}

func hasField(fields []schema.Field, name string) bool {
	for _, field := range fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

func propHelp(name string, fields []schema.Field) string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	if suggestion := schema.Suggest(name, names); suggestion != "" {
		return fmt.Sprintf("did you mean %s?", suggestion)
	}
	if len(names) == 0 {
		return "this component declares no props"
	}
	return "props: " + strings.Join(names, ", ")
}

func (b *builder) componentHelp(name string) string {
	names := make([]string, 0, len(b.components)+len(BuiltinNames()))
	for available := range b.components {
		names = append(names, available)
	}
	names = append(names, BuiltinNames()...)
	slices.Sort(names)
	if suggestion := schema.Suggest(name, names); suggestion != "" {
		return fmt.Sprintf("did you mean <%s>?", suggestion)
	}
	if len(names) == 0 {
		return "components live in components/<Name>/template.rill"
	}
	return "components: " + strings.Join(names, ", ")
}
