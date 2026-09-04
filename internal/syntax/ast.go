package syntax

import "github.com/apptivitypl/gopage/internal/diag"

type Node interface {
	NodeSpan() diag.Span
}

type Document struct {
	Frontmatter *Frontmatter
	Nodes       []Node
}

type Frontmatter struct {
	Span diag.Span
	Code string
}

type Text struct {
	Span  diag.Span
	Value string
}

func (t *Text) NodeSpan() diag.Span { return t.Span }

type Interpolation struct {
	Span diag.Span
	Expr Expr
}

func (i *Interpolation) NodeSpan() diag.Span { return i.Span }

type ClientScript struct {
	Span diag.Span
	Code string
	Lang string
}

func (c *ClientScript) NodeSpan() diag.Span { return c.Span }

type Outlet struct {
	Span diag.Span
}

func (o *Outlet) NodeSpan() diag.Span { return o.Span }

type Branch struct {
	Cond Expr
	Body []Node
}

type If struct {
	Span     diag.Span
	Branches []Branch
}

func (i *If) NodeSpan() diag.Span { return i.Span }

type For struct {
	Span  diag.Span
	Var   string
	Seq   Expr
	Body  []Node
	Empty []Node
}

func (f *For) NodeSpan() diag.Span { return f.Span }

type Let struct {
	Span  diag.Span
	Name  string
	Value Expr
}

func (l *Let) NodeSpan() diag.Span { return l.Span }

type Arm struct {
	Span diag.Span
	Name string
	Body []Node
}

type Match struct {
	Span    diag.Span
	Subject Expr
	Arms    []Arm
}

func (m *Match) NodeSpan() diag.Span { return m.Span }

type ClassEntry struct {
	Name string
	Cond Expr
}

type Attribute struct {
	Span        diag.Span
	Name        string
	Bound       bool
	Conditional bool
	Value       Expr
	Text        string
	Parts       []Node
	Classes     []ClassEntry
}

type MessageCall struct {
	Span    diag.Span
	Key     string
	KeySpan diag.Span
	Count   Expr
}

func (m *MessageCall) ExprSpan() diag.Span { return m.Span }

type FilterCall struct {
	Span     diag.Span
	Name     string
	NameSpan diag.Span
	Input    Expr
	Argument Expr
}

func (f *FilterCall) ExprSpan() diag.Span { return f.Span }

type Fragment struct {
	Span        diag.Span
	Name        string
	NameSpan    diag.Span
	Cache       string
	Stale       string
	Defer       bool
	Strategy    string
	Body        []Node
	Placeholder []Node
	HoldSpan    diag.Span
}

func (f *Fragment) NodeSpan() diag.Span { return f.Span }

type Element struct {
	Span        diag.Span
	Name        string
	Attributes  []Attribute
	SelfClosing bool
}

func (e *Element) NodeSpan() diag.Span { return e.Span }

type Slot struct {
	Name  string
	Nodes []Node
}

type Component struct {
	Span       diag.Span
	Name       string
	Attributes []Attribute
	Children   []Node
	Slots      []Slot
}

func (c *Component) NodeSpan() diag.Span { return c.Span }

func (c *Component) Slot(name string) ([]Node, bool) {
	for _, slot := range c.Slots {
		if slot.Name == name {
			return slot.Nodes, true
		}
	}
	return nil, false
}

type Children struct {
	Span diag.Span
}

func (c *Children) NodeSpan() diag.Span { return c.Span }

type MetaBlock struct {
	Span diag.Span
}

func (m *MetaBlock) NodeSpan() diag.Span { return m.Span }

type AssetsBlock struct {
	Span diag.Span
}

func (a *AssetsBlock) NodeSpan() diag.Span { return a.Span }

type SlotOutlet struct {
	Span diag.Span
	Name string
}

func (s *SlotOutlet) NodeSpan() diag.Span { return s.Span }

func (d *Document) HasOutlet() bool {
	return hasOutlet(d.Nodes)
}

func hasOutlet(nodes []Node) bool {
	for _, node := range nodes {
		switch n := node.(type) {
		case *Outlet:
			return true
		case *If:
			for _, branch := range n.Branches {
				if hasOutlet(branch.Body) {
					return true
				}
			}
		case *For:
			if hasOutlet(n.Body) || hasOutlet(n.Empty) {
				return true
			}
		case *Match:
			for _, arm := range n.Arms {
				if hasOutlet(arm.Body) {
					return true
				}
			}
		case *Component:
			if hasOutlet(n.Children) {
				return true
			}
			for _, slot := range n.Slots {
				if hasOutlet(slot.Nodes) {
					return true
				}
			}
		}
	}
	return false
}

func Walk(nodes []Node, visit func(Node)) {
	for _, node := range nodes {
		visit(node)
		switch n := node.(type) {
		case *If:
			for _, branch := range n.Branches {
				Walk(branch.Body, visit)
			}
		case *For:
			Walk(n.Body, visit)
			Walk(n.Empty, visit)
		case *Match:
			for _, arm := range n.Arms {
				Walk(arm.Body, visit)
			}
		case *Component:
			Walk(n.Children, visit)
			for _, slot := range n.Slots {
				Walk(slot.Nodes, visit)
			}
		case *Fragment:
			Walk(n.Body, visit)
		}
	}
}

func WalkExprs(nodes []Node, visit func(Expr)) {
	Walk(nodes, func(node Node) {
		switch n := node.(type) {
		case *Interpolation:
			walkExpr(n.Expr, visit)
		case *If:
			for _, branch := range n.Branches {
				walkExpr(branch.Cond, visit)
			}
		case *For:
			walkExpr(n.Seq, visit)
		case *Let:
			walkExpr(n.Value, visit)
		case *Match:
			walkExpr(n.Subject, visit)
		case *Element:
			walkAttributes(n.Attributes, visit)
		case *Component:
			walkAttributes(n.Attributes, visit)
		}
	})
}

func walkAttributes(attributes []Attribute, visit func(Expr)) {
	for _, attribute := range attributes {
		walkExpr(attribute.Value, visit)
		for _, entry := range attribute.Classes {
			walkExpr(entry.Cond, visit)
		}
		WalkExprs(attribute.Parts, visit)
	}
}

func walkExpr(expr Expr, visit func(Expr)) {
	if expr == nil {
		return
	}
	visit(expr)
	switch n := expr.(type) {
	case *Unary:
		walkExpr(n.Operand, visit)
	case *Binary:
		walkExpr(n.Left, visit)
		walkExpr(n.Right, visit)
	case *Index:
		walkExpr(n.Base, visit)
		walkExpr(n.Index, visit)
	case *FilterCall:
		walkExpr(n.Input, visit)
		walkExpr(n.Argument, visit)
	}
}
