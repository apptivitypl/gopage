package cache

import (
	"strconv"
	"strings"
)

type Key struct {
	Host    string
	Locale  string
	Path    string
	Query   string
	Variant string
	Level   int
}

func (k Key) String() string {
	var b strings.Builder
	b.Grow(len(k.Host) + len(k.Locale) + len(k.Path) + len(k.Query) + len(k.Variant) + 26)
	part(&b, k.Host)
	part(&b, k.Locale)
	part(&b, k.Path)
	part(&b, k.Query)
	part(&b, k.Variant)
	part(&b, strconv.Itoa(k.Level))
	return b.String()
}

func part(b *strings.Builder, value string) {
	var scratch [20]byte
	b.Write(strconv.AppendInt(scratch[:0], int64(len(value)), 10))
	b.WriteByte(':')
	b.WriteString(value)
}
