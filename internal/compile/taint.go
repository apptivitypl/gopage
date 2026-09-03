package compile

import (
	"fmt"
	"strings"

	"github.com/apptivitypl/rill/internal/action"
	"github.com/apptivitypl/rill/internal/form"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/schema"
	"github.com/apptivitypl/rill/internal/syntax"
)

func CheckFragments(doc *syntax.Document, file string, model *schema.Schema, bag *diag.Bag) {
	syntax.Walk(doc.Nodes, func(node syntax.Node) {
		fragment, ok := node.(*syntax.Fragment)
		if !ok || fragment.Cache == "" {
			return
		}
		guardPerVisitor(fragment, file, bag)
		if model != nil {
			guardFragment(fragment, file, model, bag)
		}
	})
}

func guardPerVisitor(fragment *syntax.Fragment, file string, bag *diag.Bag) {
	syntax.Walk(fragment.Body, func(node syntax.Node) {
		component, ok := node.(*syntax.Component)
		if !ok || component.Name != FormComponent {
			return
		}
		bag.Add(diag.New(diag.C503, file, component.Span,
			fmt.Sprintf("<%s> carries a csrf token and this fragment is cached", FormComponent)).
			WithHelp(fmt.Sprintf("the token belongs to one visitor; drop cache= from {%% fragment %q %%}, "+
				"or keep the form outside the cached part", fragment.Name)))
	})
	bound := boundNames(fragment)
	syntax.WalkExprs(fragment.Body, func(expr syntax.Expr) {
		path, ok := expr.(*syntax.Path)
		if !ok || bound[path.Segments[0]] || !perVisitor(path.Segments) {
			return
		}
		bag.Add(diag.New(diag.C503, file, path.Span,
			fmt.Sprintf("%s belongs to one visitor and this fragment is cached",
				strings.Join(path.Segments, "."))).
			WithHelp(fmt.Sprintf("drop cache= from {%% fragment %q %%}, or move the value outside it",
				fragment.Name)))
	})
}

func perVisitor(segments []string) bool {
	switch len(segments) {
	case 1:
		return segments[0] == action.FlashRoot
	case 2:
		return segments[0] == form.Root && segments[1] == form.TokenField
	}
	return false
}

func boundNames(fragment *syntax.Fragment) map[string]bool {
	bound := map[string]bool{}
	syntax.Walk(fragment.Body, func(node syntax.Node) {
		switch n := node.(type) {
		case *syntax.Let:
			bound[n.Name] = true
		case *syntax.For:
			bound[n.Var] = true
		}
	})
	return bound
}

func guardFragment(fragment *syntax.Fragment, file string, model *schema.Schema, bag *diag.Bag) {
	bound := boundNames(fragment)
	syntax.WalkExprs(fragment.Body, func(expr syntax.Expr) {
		path, ok := expr.(*syntax.Path)
		if !ok || bound[path.Segments[0]] {
			return
		}
		if !model.Private(schema.PropsName, path.Segments) {
			return
		}
		bag.Add(diag.New(diag.C503, file, path.Span,
			fmt.Sprintf("%s is private and this fragment is cached", strings.Join(path.Segments, "."))).
			WithHelp(fmt.Sprintf("drop cache= from {%% fragment %q %%}, or move the private value outside it",
				fragment.Name)))
	})
}

func CheckIslands(doc *syntax.Document, file string, model *schema.Schema,
	components map[string]Component, islands map[string]bool, bag *diag.Bag) {
	tainted := map[string]bool{}
	syntax.Walk(doc.Nodes, func(node syntax.Node) {
		switch n := node.(type) {
		case *syntax.Let:
			if model != nil && privateExpr(n.Value, model, tainted) {
				tainted[n.Name] = true
			}
		case *syntax.Component:
			if !islands[n.Name] {
				return
			}
			guardIslandFields(n, file, components[n.Name], bag)
			if model != nil {
				guardIslandProps(n, file, model, tainted, bag)
			}
		}
	})
}

func guardIslandFields(node *syntax.Component, file string, component Component, bag *diag.Bag) {
	for _, field := range component.Fields() {
		if !field.Private {
			continue
		}
		bag.Add(diag.New(diag.C320, file, node.Span,
			fmt.Sprintf("%s.%s is private and %s is an island", schema.PropsName, field.Name, node.Name)).
			WithHelp("island props are serialised into the document for the browser to read; " +
				"drop the private tag, or move the value out of the island"))
	}
}

func guardIslandProps(node *syntax.Component, file string, model *schema.Schema,
	tainted map[string]bool, bag *diag.Bag) {
	for _, attribute := range node.Attributes {
		walkPaths(attribute, func(path *syntax.Path) {
			if !privatePath(path, model, tainted) {
				return
			}
			bag.Add(diag.New(diag.C320, file, path.Span,
				fmt.Sprintf("%s reaches the browser: %s is an island",
					strings.Join(path.Segments, "."), node.Name)).
				WithHelp(fmt.Sprintf("everything passed to <%s> is written into the document as json; "+
					"pass what the browser needs, not the private value", node.Name)))
		})
	}
}

func walkPaths(attribute syntax.Attribute, visit func(*syntax.Path)) {
	syntax.WalkExprs([]syntax.Node{&syntax.Element{Attributes: []syntax.Attribute{attribute}}},
		func(expr syntax.Expr) {
			if path, ok := expr.(*syntax.Path); ok {
				visit(path)
			}
		})
}

func privateExpr(expr syntax.Expr, model *schema.Schema, tainted map[string]bool) bool {
	found := false
	walkPaths(syntax.Attribute{Value: expr}, func(path *syntax.Path) {
		if privatePath(path, model, tainted) {
			found = true
		}
	})
	return found
}

func privatePath(path *syntax.Path, model *schema.Schema, tainted map[string]bool) bool {
	if len(path.Segments) == 0 {
		return false
	}
	if tainted[path.Segments[0]] {
		return true
	}
	return model.Private(schema.PropsName, path.Segments)
}
