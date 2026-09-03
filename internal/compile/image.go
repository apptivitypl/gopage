package compile

import (
	"strconv"
	"strings"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/syntax"
)

const (
	srcAttribute    = "src"
	altAttribute    = "alt"
	widthAttribute  = "width"
	heightAttribute = "height"
	eagerAttribute  = "eager"
)

func (b *builder) imageComponent(node *syntax.Component) {
	width, okWidth := dimension(node.Attributes, widthAttribute)
	height, okHeight := dimension(node.Attributes, heightAttribute)
	if !okWidth || !okHeight {
		b.report(diag.C316, node.Span, "an image needs a literal width and height",
			`write <Image src="/hero.avif" width="1200" height="800" alt="..." /> so the box is reserved before the file arrives`)
		return
	}
	if _, ok := literal(node.Attributes, altAttribute); !ok && !hasAttribute(node.Attributes, altAttribute) {
		b.report(diag.C316, node.Span, "an image needs an alt attribute",
			`write alt="what the picture shows", or alt="" when it is decoration`)
		return
	}

	b.static("<img")
	b.imageSource(node)
	b.static(` width="` + strconv.Itoa(width) + `" height="` + strconv.Itoa(height) + `"`)
	b.static(` loading="` + b.loading(node) + `" decoding="async"`)
	for _, attribute := range node.Attributes {
		if reservedImageAttribute(attribute.Name) {
			continue
		}
		b.attribute(attribute)
	}
	b.static(">")
}

func (b *builder) imageSource(node *syntax.Component) {
	for _, attribute := range node.Attributes {
		switch {
		case strings.EqualFold(attribute.Name, srcAttribute):
			b.attribute(attribute)
		case strings.EqualFold(attribute.Name, altAttribute) && !attribute.Bound && len(attribute.Parts) == 0:
			b.static(` alt="` + escapeAttribute(attribute.Text) + `"`)
		case strings.EqualFold(attribute.Name, altAttribute):
			b.attribute(attribute)
		}
	}
}

func (b *builder) loading(node *syntax.Component) string {
	if hasAttribute(node.Attributes, eagerAttribute) {
		return "eager"
	}
	return "lazy"
}

func dimension(attributes []syntax.Attribute, name string) (int, bool) {
	text, ok := literal(attributes, name)
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(text)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func reservedImageAttribute(name string) bool {
	switch strings.ToLower(name) {
	case srcAttribute, altAttribute, widthAttribute, heightAttribute, eagerAttribute:
		return true
	default:
		return false
	}
}
