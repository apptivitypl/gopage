package compile

import (
	"sort"
	"strings"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/syntax"
)

const ClassAttribute = "class"

type inventory struct {
	seen map[string]struct{}
}

func newInventory() *inventory {
	return &inventory{seen: map[string]struct{}{}}
}

func (i *inventory) add(text string) {
	for name := range strings.FieldsSeq(text) {
		i.seen[name] = struct{}{}
	}
}

func (i *inventory) Names() []string {
	names := make([]string, 0, len(i.seen))
	for name := range i.seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (b *builder) classes(attribute syntax.Attribute) {
	if b.inventory == nil {
		return
	}
	switch {
	case len(attribute.Classes) > 0:
		for _, entry := range attribute.Classes {
			b.inventory.add(entry.Name)
		}
	case attribute.Bound:
		b.warnDynamicClass(attribute)
	case len(attribute.Parts) > 0:
		b.warnDynamicClass(attribute)
	default:
		b.inventory.add(attribute.Text)
	}
}

func (b *builder) warnDynamicClass(attribute syntax.Attribute) {
	b.bag.Add(diag.Warn(diag.W703, b.file, attribute.Span,
		"this class list is built at request time").
		WithHelp(`the css engine cannot see it; write :class="{ 'name': condition }" or add the name to safelist`))
}

func Inventory(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, "\n") + "\n"
}

func (b *builder) recordClasses(names string) {
	if b.inventory != nil {
		b.inventory.add(names)
	}
}
