package cache

import "strings"

type Key struct {
	Host    string
	Locale  string
	Path    string
	Query   string
	Variant string
}

func (k Key) String() string {
	var b strings.Builder
	b.Grow(len(k.Host) + len(k.Locale) + len(k.Path) + len(k.Query) + len(k.Variant) + 4)
	b.WriteString(k.Host)
	b.WriteByte('|')
	b.WriteString(k.Locale)
	b.WriteByte('|')
	b.WriteString(k.Path)
	b.WriteByte('|')
	b.WriteString(k.Query)
	b.WriteByte('|')
	b.WriteString(k.Variant)
	return b.String()
}
