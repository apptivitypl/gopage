package compile

import (
	"fmt"
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/syntax"
)

const interpolationOpen = "{{"

var booleanAttributes = map[string]bool{
	"allowfullscreen": true, "async": true, "autofocus": true, "autoplay": true,
	"checked": true, "controls": true, "default": true, "defer": true,
	"disabled": true, "formnovalidate": true, "hidden": true, "inert": true,
	"ismap": true, "itemscope": true, "loop": true, "multiple": true,
	"muted": true, "nomodule": true, "novalidate": true, "open": true,
	"playsinline": true, "readonly": true, "required": true, "reversed": true,
	"selected": true,
}

func (b *builder) interpolated(attribute syntax.Attribute) {
	if !strings.Contains(attribute.Text, interpolationOpen) {
		return
	}
	b.report(diag.C326, attribute.Span,
		fmt.Sprintf("%s is passed on literally, interpolation included", attribute.Name),
		fmt.Sprintf("bind the expression instead: :%s=\"…\"", attribute.Name))
}

func (b *builder) boundBoolean(attribute syntax.Attribute) bool {
	if !attribute.Bound || !booleanAttributes[strings.ToLower(attribute.Name)] {
		return false
	}
	b.report(diag.C327, attribute.Span,
		fmt.Sprintf("%s is a boolean attribute, so a bound value would always emit it", attribute.Name),
		fmt.Sprintf("write %s?=\"condition\" to leave it out when the condition is false", attribute.Name))
	return true
}
