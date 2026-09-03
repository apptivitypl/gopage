package i18n

import (
	"encoding/json"
	"sort"
	"strings"
)

type Report struct {
	Locale     string
	Keys       int
	Translated int
	Missing    []string
	Orphans    []string
}

func (r Report) Complete() bool {
	return len(r.Missing) == 0
}

func (r Report) Percent() float64 {
	if r.Keys == 0 {
		return 100
	}
	return float64(r.Translated) * 100 / float64(r.Keys)
}

func Audit(keys []string, catalogs map[string]Catalog, locales []string) []Report {
	used := make(map[string]bool, len(keys))
	for _, key := range keys {
		used[key] = true
	}
	reports := make([]Report, 0, len(locales))
	for _, locale := range locales {
		reports = append(reports, audit(keys, used, catalogs[locale], locale))
	}
	return reports
}

func audit(keys []string, used map[string]bool, catalog Catalog, locale string) Report {
	report := Report{Locale: locale, Keys: len(keys)}
	for _, key := range keys {
		if _, ok := catalog.Messages[key]; ok {
			report.Translated++
			continue
		}
		report.Missing = append(report.Missing, key)
	}
	for _, key := range catalog.Keys() {
		if !used[key] {
			report.Orphans = append(report.Orphans, key)
		}
	}
	sort.Strings(report.Missing)
	return report
}

func Snippet(missing []string, source Catalog) string {
	if len(missing) == 0 {
		return ""
	}
	sorted := append([]string(nil), missing...)
	sort.Strings(sorted)

	tree := map[string]any{}
	for _, key := range sorted {
		message, ok := source.Messages[key]
		if !ok {
			insert(tree, key, key)
			continue
		}
		if !message.Plural() {
			text, _ := message.Text(FormOther)
			insert(tree, key, text)
			continue
		}
		forms := map[string]any{}
		for _, form := range []Form{FormZero, FormOne, FormTwo, FormFew, FormMany, FormOther} {
			if text, ok := message.Forms[form]; ok {
				forms[form.String()] = text
			}
		}
		insert(tree, key, forms)
	}
	encoded, _ := json.MarshalIndent(tree, "", "  ")
	return string(encoded) + "\n"
}

func insert(tree map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	for _, part := range parts[:len(parts)-1] {
		nested, ok := tree[part].(map[string]any)
		if !ok {
			nested = map[string]any{}
			tree[part] = nested
		}
		tree = nested
	}
	tree[parts[len(parts)-1]] = value
}
