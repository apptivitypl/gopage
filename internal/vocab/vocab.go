package vocab

import (
	"context"
	"strings"

	"github.com/apptivitypl/gopage/internal/config"
)

type Table struct {
	settings  config.Config
	public    map[string]map[string]string
	canonical map[string]map[string]string
}

type key struct{}

func With(ctx context.Context, table Table) context.Context {
	return context.WithValue(ctx, key{}, table)
}

func From(ctx context.Context) Table {
	table, _ := ctx.Value(key{}).(Table)
	return table
}

func New(settings config.Config) Table {
	if len(settings.Routing.Aliases) == 0 {
		return Table{settings: settings}
	}
	table := Table{
		settings:  settings,
		public:    make(map[string]map[string]string, len(settings.Routing.Aliases)),
		canonical: make(map[string]map[string]string, len(settings.Routing.Aliases)),
	}
	for locale, aliases := range settings.Routing.Aliases {
		forward := make(map[string]string, len(aliases))
		backward := make(map[string]string, len(aliases))
		for from, to := range aliases {
			forward[from] = to
			backward[to] = from
		}
		table.public[locale] = forward
		table.canonical[locale] = backward
	}
	return table
}

func (t Table) Any() bool {
	return len(t.public) > 0
}

func (t Table) Localises() bool {
	if t.Any() {
		return true
	}
	return t.settings.I18n.Mode == config.ModePath && len(t.settings.I18n.Locales) > 1
}

func (t Table) Speaks(locale string) bool {
	return len(t.public[locale]) > 0
}

func (t Table) Localise(locale, path string) string {
	spoken := t.Public(locale, path)
	settings := t.settings
	if settings.I18n.Mode != config.ModePath {
		return spoken
	}
	if locale == settings.I18n.DefaultLocale && !settings.I18n.PrefixDefault {
		return spoken
	}
	if spoken == "/" {
		return "/" + locale
	}
	return "/" + locale + spoken
}

func (t Table) Public(locale, path string) string {
	return translate(t.public[locale], path)
}

func (t Table) Canonical(locale, path string) string {
	return translate(t.canonical[locale], path)
}

func translate(words map[string]string, path string) string {
	if len(words) == 0 || len(path) < 2 {
		return path
	}
	var out strings.Builder
	start, changed := 0, false
	for start < len(path) {
		end := strings.IndexByte(path[start+1:], '/')
		if end < 0 {
			end = len(path)
		} else {
			end += start + 1
		}
		segment := path[start+1 : end]
		if word, ok := words[segment]; ok {
			if !changed {
				out.Grow(len(path) + 8)
				out.WriteString(path[:start])
				changed = true
			}
			out.WriteByte('/')
			out.WriteString(word)
		} else if changed {
			out.WriteByte('/')
			out.WriteString(segment)
		}
		start = end
	}
	if !changed {
		return path
	}
	return out.String()
}
