package compile

import (
	"fmt"
	"sort"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/i18n"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
	"github.com/apptivitypl/rill/internal/syntax"
)

type messageUse struct {
	index  uint32
	plural bool
	file   string
	span   diag.Span
}

type messageTable struct {
	keys  []string
	index map[string]uint32
	uses  []messageUse
}

func newMessageTable() *messageTable {
	return &messageTable{index: map[string]uint32{}}
}

func (m *messageTable) intern(key string) uint32 {
	if index, ok := m.index[key]; ok {
		return index
	}
	index := uint32(len(m.keys))
	m.keys = append(m.keys, key)
	m.index[key] = index
	return index
}

func (m *messageTable) Keys() []string {
	return m.keys
}

func (b *builder) messageCall(node *syntax.MessageCall) uint32 {
	if b.messages == nil {
		return b.constant(ir.Const{Kind: ir.ConstString, Str: node.Key}, "s"+node.Key)
	}
	index := b.messages.intern(node.Key)
	b.messages.uses = append(b.messages.uses, messageUse{
		index:  index,
		plural: node.Count != nil,
		file:   b.file,
		span:   node.KeySpan,
	})
	count := runtime.NoArgument
	if node.Count != nil {
		count = b.expr(node.Count)
	}
	return b.emitExpr(ir.ExprNode{Kind: ir.ExprMessage, A: index, B: count})
}

func BuildCatalogs(table *messageTable, catalogs map[string]i18n.Catalog, locales []string,
	fallback string, strict bool, bag *diag.Bag) []ir.Catalog {
	if len(table.keys) == 0 {
		return nil
	}
	built := make([]ir.Catalog, 0, len(locales))
	for _, locale := range locales {
		built = append(built, buildCatalog(table, catalogs, locale, fallback, strict, bag))
	}
	return built
}

func buildCatalog(table *messageTable, catalogs map[string]i18n.Catalog, locale, fallback string,
	strict bool, bag *diag.Bag) ir.Catalog {
	catalog := ir.Catalog{Locale: locale, Texts: make([][ir.PluralForms]string, len(table.keys))}
	for index, key := range table.keys {
		message, found := catalogs[locale].Messages[key]
		if !found {
			if strict {
				report(table, uint32(index), key, locale, bag)
			}
			message, found = catalogs[fallback].Messages[key]
		}
		if !found {
			catalog.Texts[index][i18n.FormOther] = key
			continue
		}
		for form, text := range message.Forms {
			catalog.Texts[index][form] = text
		}
	}
	return catalog
}

func report(table *messageTable, index uint32, key, locale string, bag *diag.Bag) {
	for _, use := range table.uses {
		if use.index != index {
			continue
		}
		bag.Add(diag.New(diag.C601, use.file, use.span,
			fmt.Sprintf("%s has no translation in %s", key, locale)).
			WithHelp(fmt.Sprintf("add %s to locales/%s.json, or run rill i18n sync", key, locale)))
		return
	}
}

func CheckPlurals(table *messageTable, catalogs map[string]i18n.Catalog, locales []string, bag *diag.Bag) {
	for _, use := range table.uses {
		key := table.keys[use.index]
		for _, locale := range locales {
			catalog, ok := catalogs[locale]
			if !ok {
				continue
			}
			message, found := catalog.Messages[key]
			if !found || message.Plural() == use.plural {
				continue
			}
			bag.Add(diag.New(diag.C602, use.file, use.span, pluralMessage(key, locale, use.plural)).
				WithHelp(pluralHelp(use.plural)))
		}
	}
}

func pluralMessage(key, locale string, counted bool) string {
	if counted {
		return fmt.Sprintf("%s is called with a count but %s declares one form", key, locale)
	}
	return fmt.Sprintf("%s declares plural forms in %s but is called without a count", key, locale)
}

func pluralHelp(counted bool) string {
	if counted {
		return "give the message one, few and other forms, or drop the count argument"
	}
	return `pass the count: t("reviews.count", count = len(Reviews))`
}

func Orphans(table *messageTable, catalogs map[string]i18n.Catalog, locale string) []string {
	catalog, ok := catalogs[locale]
	if !ok {
		return nil
	}
	var unused []string
	for _, key := range catalog.Keys() {
		if _, used := table.index[key]; !used {
			unused = append(unused, key)
		}
	}
	sort.Strings(unused)
	return unused
}

func (b *builder) messageKeys() []string {
	if b.messages == nil {
		return nil
	}
	return b.messages.Keys()
}
