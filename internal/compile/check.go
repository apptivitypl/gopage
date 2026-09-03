package compile

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/schema"
	"github.com/sonquer/rill/internal/syntax"
)

type binding struct {
	name  string
	typ   schema.Type
	known bool
}

type checker struct {
	schema *schema.Schema
	file   string
	bag    *diag.Bag
	locals []binding
}

func Check(doc *syntax.Document, file string, model *schema.Schema, bag *diag.Bag) {
	if model == nil {
		return
	}
	if _, ok := model.Props(); !ok {
		return
	}
	c := &checker{schema: model, file: file, bag: bag}
	c.nodes(doc.Nodes)
}

func (c *checker) nodes(nodes []syntax.Node) {
	depth := len(c.locals)
	for _, node := range nodes {
		c.node(node)
	}
	c.locals = c.locals[:depth]
}

func (c *checker) node(node syntax.Node) {
	switch n := node.(type) {
	case *syntax.Interpolation:
		c.expr(n.Expr)
	case *syntax.Let:
		c.expr(n.Value)
		typ, known := c.typeOf(n.Value)
		c.locals = append(c.locals, binding{name: n.Name, typ: typ, known: known})
	case *syntax.If:
		for _, branch := range n.Branches {
			if branch.Cond != nil {
				c.expr(branch.Cond)
			}
			c.nodes(branch.Body)
		}
	case *syntax.For:
		c.forNode(n)
	case *syntax.Element:
		c.element(n)
	case *syntax.Match:
		c.matchNode(n)
	case *syntax.Component:
		for _, attribute := range n.Attributes {
			if attribute.Bound {
				c.expr(attribute.Value)
			}
		}
		c.nodes(n.Children)
		for _, slot := range n.Slots {
			c.nodes(slot.Nodes)
		}
	}
}

func (c *checker) element(node *syntax.Element) {
	for _, attribute := range node.Attributes {
		switch {
		case len(attribute.Classes) > 0:
			for _, entry := range attribute.Classes {
				c.expr(entry.Cond)
			}
		case attribute.Value != nil:
			c.expr(attribute.Value)
		default:
			c.nodes(attribute.Parts)
		}
	}
}

func (c *checker) matchNode(node *syntax.Match) {
	c.expr(node.Subject)
	for _, arm := range node.Arms {
		c.nodes(arm.Body)
	}

	typ, known := c.typeOf(node.Subject)
	if !known {
		return
	}
	enum, ok := c.schema.Enum(typ.Name)
	if !ok {
		c.report(diag.C309, node.Span,
			fmt.Sprintf("match needs a named constant type, and this is a %s", typ.Kind),
			"declare the type and its constants in the Go block, for example type Status string")
		return
	}
	c.arms(node, enum)
}

func (c *checker) arms(node *syntax.Match, enum schema.Enum) {
	seen := map[string]bool{}
	for _, arm := range node.Arms {
		switch {
		case !slices.Contains(enum.Members, arm.Name):
			help := "cases: " + strings.Join(enum.Members, ", ")
			if suggestion := schema.Suggest(arm.Name, enum.Members); suggestion != "" {
				help = fmt.Sprintf("did you mean %s?", suggestion)
			}
			c.report(diag.C309, arm.Span,
				fmt.Sprintf("%s is not a case of %s", arm.Name, enum.Name), help)
		case seen[arm.Name]:
			c.report(diag.C309, arm.Span,
				fmt.Sprintf("%s is handled twice", arm.Name), "remove the duplicate case")
		default:
			seen[arm.Name] = true
		}
	}
	var missing []string
	for _, member := range enum.Members {
		if !seen[member] {
			missing = append(missing, member)
		}
	}
	if len(missing) > 0 {
		c.report(diag.C309, node.Span,
			fmt.Sprintf("match on %s does not handle %s", enum.Name, strings.Join(missing, ", ")),
			"every case must be handled; a match has no fallthrough")
	}
}

func (c *checker) forNode(node *syntax.For) {
	c.expr(node.Seq)
	element, known := c.elementType(node.Seq)

	depth := len(c.locals)
	c.locals = append(c.locals, binding{name: node.Var, typ: element, known: known})
	c.nodes(node.Body)
	c.locals = c.locals[:depth]

	c.nodes(node.Empty)
}

func (c *checker) elementType(seq syntax.Expr) (schema.Type, bool) {
	typ, known := c.typeOf(seq)
	if !known {
		return schema.Type{}, false
	}
	if typ.Kind != schema.KindSlice {
		if path, ok := seq.(*syntax.Path); ok {
			c.report(diag.C305, path.Span,
				fmt.Sprintf("%s is a %s, not a list", strings.Join(path.Segments, "."), typ.Kind),
				"loops read slices; convert the value in Load if you need one")
		}
		return schema.Type{}, false
	}
	return *typ.Elem, true
}

func (c *checker) expr(node syntax.Expr) {
	switch n := node.(type) {
	case *syntax.Path:
		c.path(n)
	case *syntax.Unary:
		c.expr(n.Operand)
	case *syntax.Binary:
		c.expr(n.Left)
		c.expr(n.Right)
	case *syntax.Index:
		c.expr(n.Base)
		c.expr(n.Index)
	case *syntax.FilterCall:
		c.expr(n.Input)
		if n.Argument != nil {
			c.expr(n.Argument)
		}
	}
}

func (c *checker) path(node *syntax.Path) {
	if local, ok := c.lookup(node.Segments[0]); ok {
		c.localPath(node, local)
		return
	}
	if RootPath(node.Segments) {
		return
	}
	if _, ok := c.schema.Resolve(schema.PropsName, node.Segments); ok {
		return
	}
	c.reportUnknown(node, schema.PropsName, node.Segments)
}

func (c *checker) localPath(node *syntax.Path, local binding) {
	rest := node.Segments[1:]
	if len(rest) == 0 || !local.known {
		return
	}
	base := local.typ
	for base.Kind == schema.KindOptional || base.Kind == schema.KindSlice {
		base = *base.Elem
	}
	if base.Kind != schema.KindStruct {
		c.report(diag.C305, node.Span,
			fmt.Sprintf("%s is a %s and has no fields", node.Segments[0], base.Kind), "")
		return
	}
	if _, ok := c.schema.Resolve(base.Name, rest); ok {
		return
	}
	c.reportUnknown(node, base.Name, rest)
}

func (c *checker) reportUnknown(node *syntax.Path, root string, path []string) {
	owner, missing, candidates := c.locate(root, path)
	message := fmt.Sprintf("%s has no field %s", owner, missing)
	help := ""
	if suggestion := schema.Suggest(missing, candidates); suggestion != "" {
		help = fmt.Sprintf("did you mean %s?", suggestion)
	} else if len(candidates) > 0 {
		help = "available fields: " + strings.Join(candidates, ", ")
	}
	c.report(diag.C305, node.Span, message, help)
}

func (c *checker) locate(root string, path []string) (string, string, []string) {
	current, ok := c.schema.Get(root)
	if !ok {
		return root, path[0], nil
	}
	for i, segment := range path {
		field, found := current.Field(segment)
		if !found {
			return current.Name, segment, current.FieldNames()
		}
		if i == len(path)-1 {
			break
		}
		next := field.Type
		for next.Kind == schema.KindOptional || next.Kind == schema.KindSlice {
			next = *next.Elem
		}
		if next.Kind != schema.KindStruct {
			return current.Name, path[i+1], nil
		}
		if current, ok = c.schema.Get(next.Name); !ok {
			return next.Name, path[i+1], nil
		}
	}
	return current.Name, path[len(path)-1], current.FieldNames()
}

func (c *checker) typeOf(node syntax.Expr) (schema.Type, bool) {
	path, ok := node.(*syntax.Path)
	if !ok {
		return schema.Type{}, false
	}
	if local, ok := c.lookup(path.Segments[0]); ok {
		if len(path.Segments) == 1 {
			return local.typ, local.known
		}
		if !local.known {
			return schema.Type{}, false
		}
		base := local.typ
		for base.Kind == schema.KindOptional || base.Kind == schema.KindSlice {
			base = *base.Elem
		}
		return c.schema.Resolve(base.Name, path.Segments[1:])
	}
	return c.schema.Resolve(schema.PropsName, path.Segments)
}

func (c *checker) lookup(name string) (binding, bool) {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].name == name {
			return c.locals[i], true
		}
	}
	return binding{}, false
}

func (c *checker) report(code diag.Code, span diag.Span, message, help string) {
	d := diag.New(code, c.file, span, message)
	if help != "" {
		d = d.WithHelp(help)
	}
	c.bag.Add(d)
}
