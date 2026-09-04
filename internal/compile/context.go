package compile

import (
	"fmt"
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/syntax"
)

const (
	ScriptElement   = "script"
	StyleElement    = "style"
	StyleAttribute  = "style"
	SrcdocAttribute = "srcdoc"
	handlerPrefix   = "on"
)

func CheckContexts(doc *syntax.Document, file string, bag *diag.Bag) {
	checkNodes(doc.Nodes, "", file, bag)
}

func checkNodes(nodes []syntax.Node, raw, file string, bag *diag.Bag) string {
	for _, node := range nodes {
		raw = checkNode(node, raw, file, bag)
	}
	return raw
}

func checkNode(node syntax.Node, raw, file string, bag *diag.Bag) string {
	switch n := node.(type) {
	case *syntax.Element:
		checkAttributes(n.Attributes, file, bag)
		if n.SelfClosing {
			return raw
		}
		switch strings.ToLower(n.Name) {
		case ScriptElement:
			return ScriptElement
		case StyleElement:
			return StyleElement
		}
	case *syntax.Text:
		if raw != "" && strings.EqualFold(strings.TrimSpace(n.Value), "</"+raw+">") {
			return ""
		}
	case *syntax.Interpolation:
		reportRaw(raw, n.Span, file, bag)
	case *syntax.Component:
		checkAttributes(n.Attributes, file, bag)
		checkNodes(n.Children, "", file, bag)
		for _, slot := range n.Slots {
			checkNodes(slot.Nodes, "", file, bag)
		}
	case *syntax.If:
		for _, branch := range n.Branches {
			checkNodes(branch.Body, raw, file, bag)
		}
	case *syntax.For:
		checkNodes(n.Body, raw, file, bag)
		checkNodes(n.Empty, raw, file, bag)
	case *syntax.Match:
		for _, arm := range n.Arms {
			checkNodes(arm.Body, raw, file, bag)
		}
	case *syntax.Fragment:
		checkNodes(n.Placeholder, raw, file, bag)
		return checkNodes(n.Body, raw, file, bag)
	}
	return raw
}

func reportRaw(raw string, span diag.Span, file string, bag *diag.Bag) {
	switch raw {
	case ScriptElement:
		bag.Add(diag.New(diag.C321, file, span,
			"a value is interpolated into a <script> body").
			WithHelp("the escaper writes html entities, which javascript does not read as text; " +
				"pass the value to an island instead, or put it in a data- attribute and read it from there"))
	case StyleElement:
		bag.Add(diag.New(diag.C322, file, span,
			"a value is interpolated into a <style> body").
			WithHelp("the escaper writes html entities, which css does not read as text; " +
				"set a custom property on an element and use var() in the stylesheet"))
	}
}

func checkAttributes(attributes []syntax.Attribute, file string, bag *diag.Bag) {
	for _, attribute := range attributes {
		if !dynamicAttribute(attribute) {
			continue
		}
		name := strings.ToLower(attribute.Name)
		switch {
		case strings.HasPrefix(name, handlerPrefix) && len(name) > len(handlerPrefix):
			bag.Add(diag.New(diag.C323, file, attribute.Span,
				fmt.Sprintf("a value is interpolated into %s, an event handler", attribute.Name)).
				WithHelp("the browser decodes html entities before javascript reads the attribute, " +
					"so escaping cannot hold here; move the behaviour into an island"))
		case name == StyleAttribute:
			bag.Add(diag.New(diag.C322, file, attribute.Span,
				"a value is interpolated into a style attribute").
				WithHelp("the escaper writes html entities, which css does not read as text; " +
					"bind a class instead, or set a custom property the stylesheet reads with var()"))
		case name == SrcdocAttribute:
			bag.Add(diag.New(diag.C324, file, attribute.Span,
				"a value is interpolated into srcdoc").
				WithHelp("the browser decodes the attribute and parses it as a document, " +
					"so escaped markup becomes markup again; serve the frame a url with src"))
		}
	}
}

func dynamicAttribute(attribute syntax.Attribute) bool {
	if attribute.Bound {
		return true
	}
	for _, part := range attribute.Parts {
		if _, ok := part.(*syntax.Interpolation); ok {
			return true
		}
	}
	return false
}
