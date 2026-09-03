package compile

import (
	"fmt"
	"strings"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/schema"
	"github.com/sonquer/rill/internal/syntax"
)

const (
	HrefAttribute = "href"
	RawPrefix     = "!"
	dynamicMarker = "\x00"
)

type Linker struct {
	patterns []string
	files    map[string]bool
}

func NewLinker(routes []Route) *Linker {
	patterns := make([]string, 0, len(routes))
	for _, route := range routes {
		patterns = append(patterns, route.Pattern)
	}
	return &Linker{patterns: patterns, files: map[string]bool{}}
}

func (l *Linker) Serving(paths ...string) *Linker {
	for _, file := range paths {
		l.files[file] = true
	}
	return l
}

func (l *Linker) Check(doc *syntax.Document, file string, bag *diag.Bag) {
	syntax.Walk(doc.Nodes, func(node syntax.Node) {
		element, ok := node.(*syntax.Element)
		if !ok {
			return
		}
		for i := range element.Attributes {
			l.attribute(&element.Attributes[i], file, bag)
		}
	})
}

func (l *Linker) attribute(attribute *syntax.Attribute, file string, bag *diag.Bag) {
	if attribute.Name != HrefAttribute || attribute.Bound || attribute.Conditional {
		return
	}
	if strings.HasPrefix(attribute.Text, RawPrefix) {
		attribute.Text = strings.TrimPrefix(attribute.Text, RawPrefix)
		trimRawPrefix(attribute)
		return
	}
	if l.files[attribute.Text] {
		return
	}
	segments, ok := l.segments(attribute)
	if !ok {
		return
	}
	if l.match(segments) {
		return
	}
	shown := strings.ReplaceAll(strings.Join(segments, "/"), dynamicMarker, "…")
	help := "add the route, or write href=\"!/…\" to leave the link untouched"
	if suggestion := schema.Suggest("/"+shown, l.patterns); suggestion != "" {
		help = fmt.Sprintf("did you mean %s?", suggestion)
	}
	bag.Add(diag.New(diag.C111, file, attribute.Span,
		fmt.Sprintf("no route answers /%s", shown)).WithHelp(help))
}

func trimRawPrefix(attribute *syntax.Attribute) {
	if len(attribute.Parts) == 0 {
		return
	}
	if text, ok := attribute.Parts[0].(*syntax.Text); ok {
		text.Value = strings.TrimPrefix(text.Value, RawPrefix)
	}
}

func (l *Linker) segments(attribute *syntax.Attribute) ([]string, bool) {
	path, ok := linkPath(attribute)
	if !ok {
		return nil, false
	}
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil, true
	}
	return strings.Split(trimmed, "/"), true
}

func linkPath(attribute *syntax.Attribute) (string, bool) {
	var b strings.Builder
	if len(attribute.Parts) == 0 {
		b.WriteString(attribute.Text)
	}
	for _, part := range attribute.Parts {
		switch n := part.(type) {
		case *syntax.Text:
			b.WriteString(n.Value)
		case *syntax.Interpolation:
			b.WriteString(dynamicMarker)
		}
	}
	value := b.String()
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", false
	}
	if cut := strings.IndexAny(value, "?#"); cut >= 0 {
		value = value[:cut]
	}
	return value, true
}

func (l *Linker) match(segments []string) bool {
	for _, pattern := range l.patterns {
		if matchSegments(patternSegments(pattern), segments) {
			return true
		}
	}
	return false
}

func patternSegments(pattern string) []string {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func matchSegments(pattern, actual []string) bool {
	for i, segment := range pattern {
		switch {
		case strings.HasPrefix(segment, "[") && strings.Contains(segment, "..."):
			return len(actual) > i || strings.HasPrefix(segment, "[[")
		case i >= len(actual):
			return false
		case strings.HasPrefix(segment, "["):
			continue
		case segment != actual[i]:
			return false
		}
	}
	return len(actual) == len(pattern)
}
