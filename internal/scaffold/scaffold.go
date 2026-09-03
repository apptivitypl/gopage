package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

//go:embed all:templates
var templates embed.FS

const root = "templates"

const (
	tmplSuffix      = ".tmpl"
	DefaultTemplate = "hello-world"
)

var renames = map[string]string{
	"gomod":     "go.mod",
	"gitignore": ".gitignore",
}

type Config struct {
	Dir           string
	Module        string
	Name          string
	Template      string
	RillPath      string
	RillVersion   string
	CompatDate    string
	Locales       []string
	DefaultLocale string
	Nav           string
	CSS           string
	Theme         string
	React         string
}

const DefaultRillVersion = "v0.0.0"

const (
	ReactOn     = "react"
	ReactPreact = "preact"
	ReactOff    = "off"
	ThemeSystem = "system"
	ThemeToggle = "toggle"
	ThemeLight  = "light"
	ThemeDark   = "dark"
	NavOff      = "off"
	NavPartial  = "partial"
	CSSPlain    = "plain"
	CSSTailwind = "tailwind"
)

func (c Config) withDefaults() Config {
	if c.RillVersion == "" {
		c.RillVersion = DefaultRillVersion
	}
	if c.Template == "" {
		c.Template = DefaultTemplate
	}
	if c.Name == "" {
		c.Name = DefaultName(c.Dir)
	}
	if c.DefaultLocale == "" {
		c.DefaultLocale = "en"
	}
	if len(c.Locales) == 0 {
		c.Locales = []string{c.DefaultLocale}
	}
	if c.Nav == "" {
		c.Nav = NavPartial
	}
	if c.CSS == "" {
		c.CSS = CSSTailwind
	}
	if c.Theme == "" {
		c.Theme = ThemeToggle
	}
	if c.React == "" {
		c.React = ReactOn
	}
	return c
}

func Reacts() []string {
	return []string{ReactOn, ReactPreact, ReactOff}
}

func (c Config) UsesReact() bool {
	return c.React != ReactOff
}

type data struct {
	Module        string
	Name          string
	RillPath      string
	RillVersion   string
	CompatDate    string
	Locales       string
	DefaultLocale string
	Nav           string
	CSS           string
	Theme         string
	Toggle        bool
	Forced        string
	React         bool
	Engine        string
}

func Themes() []string {
	return []string{ThemeToggle, ThemeSystem, ThemeLight, ThemeDark}
}

func Names() []string {
	entries, err := templates.ReadDir(root)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func Has(name string) bool {
	for _, candidate := range Names() {
		if candidate == name {
			return true
		}
	}
	return false
}

func quoteList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func SplitLocales(text string) []string {
	var locales []string
	for part := range strings.SplitSeq(text, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			locales = append(locales, trimmed)
		}
	}
	return locales
}

func DefaultName(dir string) string {
	return filepath.Base(strings.TrimSuffix(filepath.Clean(dir), string(filepath.Separator)))
}

func Create(cfg Config) error {
	cfg = cfg.withDefaults()
	if !Has(cfg.Template) {
		return fmt.Errorf("unknown template %q, available: %s", cfg.Template, strings.Join(Names(), ", "))
	}
	if cfg.Module == "" {
		return fmt.Errorf("a module path is required, for example example.com/%s", cfg.Name)
	}
	if err := ensureEmpty(cfg.Dir); err != nil {
		return err
	}
	values := data{
		Module:        cfg.Module,
		Name:          cfg.Name,
		RillPath:      cfg.RillPath,
		RillVersion:   cfg.RillVersion,
		CompatDate:    cfg.CompatDate,
		Locales:       quoteList(cfg.Locales),
		DefaultLocale: cfg.DefaultLocale,
		Nav:           cfg.Nav,
		CSS:           cfg.CSS,
		Theme:         cfg.Theme,
		Toggle:        cfg.Theme == ThemeToggle,
		Forced:        forcedTheme(cfg.Theme),
		React:         cfg.UsesReact(),
		Engine:        cfg.React,
	}
	source := filepath.ToSlash(filepath.Join(root, cfg.Template))
	return fs.WalkDir(templates, source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative := strings.TrimPrefix(path, source+"/")
		if skipped(relative, values) {
			return nil
		}
		return writeOne(cfg.Dir, relative, path, values)
	})
}

var optional = map[string]func(data) bool{
	"components/ThemeToggle.rill": func(values data) bool { return values.Toggle },
	"package.json.tmpl":           func(values data) bool { return values.React },
}

func skipped(relative string, values data) bool {
	wanted, ok := optional[relative]
	return ok && !wanted(values)
}

func forcedTheme(theme string) string {
	if theme == ThemeLight || theme == ThemeDark {
		return theme
	}
	return ""
}

func writeOne(dir, relative, path string, values data) error {
	content, err := templates.ReadFile(path)
	if err != nil {
		return err
	}
	target, isTemplate := targetName(relative)
	if isTemplate {
		if content, err = render(path, content, values); err != nil {
			return err
		}
	}
	full := filepath.Join(dir, filepath.FromSlash(target))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, content, 0o644)
}

func targetName(relative string) (string, bool) {
	name, isTemplate := strings.CutSuffix(relative, tmplSuffix)
	if renamed, ok := renames[name]; ok {
		return renamed, isTemplate
	}
	return name, isTemplate
}

const (
	rillOpen  = "<<"
	rillClose = ">>"
)

func render(name string, content []byte, values data) ([]byte, error) {
	engine := template.New(name)
	if strings.HasSuffix(name, ".rill"+tmplSuffix) {
		engine = engine.Delims(rillOpen, rillClose)
	}
	parsed, err := engine.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	var out bytes.Buffer
	if err := parsed.Execute(&out, values); err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	return out.Bytes(), nil
}

var ErrNotEmpty = errors.New("the directory is not empty")

func ensureEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %s", ErrNotEmpty, dir)
	}
	return nil
}
