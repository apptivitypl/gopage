package compile

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/syntax"
)

const (
	srcAttribute    = "src"
	altAttribute    = "alt"
	widthAttribute  = "width"
	heightAttribute = "height"
	eagerAttribute  = "eager"
	plainAttribute  = "plain"
	sizesAttribute  = "sizes"
	ImageEndpoint   = "/_gopage/image"
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
	b.imageSource(node, width)
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

func (b *builder) imageSource(node *syntax.Component, width int) {
	for _, attribute := range node.Attributes {
		switch {
		case strings.EqualFold(attribute.Name, srcAttribute):
			b.imageAddress(node, attribute, width)
		case strings.EqualFold(attribute.Name, altAttribute) && !attribute.Bound && len(attribute.Parts) == 0:
			b.static(` alt="` + escapeAttribute(attribute.Text) + `"`)
		case strings.EqualFold(attribute.Name, altAttribute):
			b.attribute(attribute)
		}
	}
}

func (b *builder) imageAddress(node *syntax.Component, attribute syntax.Attribute, width int) {
	if !b.optimises(node, attribute) {
		b.attribute(attribute)
		return
	}
	b.static(` src="`)
	b.imageURL(attribute, width)
	b.static(`" srcset="`)
	for index, size := range b.imageWidths(width) {
		if index > 0 {
			b.static(", ")
		}
		b.imageURL(attribute, size)
		b.static(" " + strconv.Itoa(size) + "w")
	}
	b.static(`"`)
}

func (b *builder) imageURL(attribute syntax.Attribute, width int) {
	b.static(ImageEndpoint + "?src=")
	if attribute.Bound {
		b.emit(ir.Op{Kind: ir.OpQuery, A: b.expr(attribute.Value)})
	} else {
		b.static(url.QueryEscape(attribute.Text))
	}
	b.static("&amp;w=" + strconv.Itoa(width))
}

func (b *builder) optimises(node *syntax.Component, attribute syntax.Attribute) bool {
	if !b.images.Enabled() || hasAttribute(node.Attributes, plainAttribute) {
		return false
	}
	if attribute.Bound {
		return true
	}
	return len(attribute.Parts) == 0 && strings.HasPrefix(attribute.Text, "/")
}

func (b *builder) imageWidths(width int) []int {
	var sizes []int
	for _, size := range b.images.Sizes() {
		if size < width {
			sizes = append(sizes, size)
		}
	}
	return append(sizes, width)
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
	case srcAttribute, altAttribute, widthAttribute, heightAttribute, eagerAttribute, plainAttribute:
		return true
	default:
		return false
	}
}
