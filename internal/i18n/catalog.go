package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/apptivitypl/rill/internal/paths"
)

const (
	Dir       = paths.LocalesDir
	Extension = ".json"
	Legacy    = ".toml"
)

type Message struct {
	Key   string
	Forms map[Form]string
}

func (m Message) Plural() bool {
	if len(m.Forms) != 1 {
		return true
	}
	_, only := m.Forms[FormOther]
	return !only
}

func (m Message) Text(form Form) (string, bool) {
	if text, ok := m.Forms[form]; ok {
		return text, true
	}
	text, ok := m.Forms[FormOther]
	return text, ok
}

type Catalog struct {
	Locale   string
	Messages map[string]Message
}

func (c Catalog) Keys() []string {
	keys := make([]string, 0, len(c.Messages))
	for key := range c.Messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func Load(fsys fs.FS) (map[string]Catalog, error) {
	entries, err := fs.ReadDir(fsys, Dir)
	if err != nil {
		return map[string]Catalog{}, nil
	}
	catalogs := map[string]Catalog{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), Extension) {
			continue
		}
		locale := strings.TrimSuffix(entry.Name(), Extension)
		data, err := fs.ReadFile(fsys, path.Join(Dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		catalog, err := Parse(locale, string(data))
		if err != nil {
			return nil, err
		}
		catalogs[locale] = catalog
	}
	return catalogs, nil
}

func Legacies(fsys fs.FS) []string {
	entries, err := fs.ReadDir(fsys, Dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), Legacy) {
			continue
		}
		names = append(names, path.Join(Dir, entry.Name()))
	}
	sort.Strings(names)
	return names
}

func Parse(locale, text string) (Catalog, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return Catalog{}, fmt.Errorf("%s%s: %w", locale, Extension, err)
	}
	catalog := Catalog{Locale: locale, Messages: map[string]Message{}}
	if err := collect(catalog, "", raw); err != nil {
		return Catalog{}, fmt.Errorf("%s%s: %w", locale, Extension, err)
	}
	return catalog, nil
}

func collect(catalog Catalog, prefix string, raw map[string]any) error {
	if forms, ok := plural(raw); ok {
		catalog.Messages[prefix] = Message{Key: prefix, Forms: forms}
		return nil
	}
	for name, value := range raw {
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		switch typed := value.(type) {
		case string:
			catalog.Messages[key] = Message{Key: key, Forms: map[Form]string{FormOther: typed}}
		case map[string]any:
			if err := collect(catalog, key, typed); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s holds a %T, want a string or a table of plural forms", key, value)
		}
	}
	return nil
}

func plural(raw map[string]any) (map[Form]string, bool) {
	forms := map[Form]string{}
	for name, value := range raw {
		form, known := FormOf(name)
		if !known {
			return nil, false
		}
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		forms[form] = text
	}
	return forms, len(forms) > 0
}
