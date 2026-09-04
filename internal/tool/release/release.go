package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/apptivitypl/gopage/internal/jsonc"
)

const FileName = "versions.jsonc"

type Kind string

const (
	KindGo   Kind = "go"
	KindNPM  Kind = "npm"
	KindVSIX Kind = "vsix"
)

type State string

const (
	Publish    State = "publish"
	Current    State = "up to date"
	Unreleased State = "unreleased changes"
)

type entry struct {
	Kind       Kind     `json:"kind"`
	Version    string   `json:"version"`
	Dir        string   `json:"dir,omitempty"`
	Tag        string   `json:"tag,omitempty"`
	Release    string   `json:"release,omitempty"`
	Needs      []string `json:"needs,omitempty"`
	Paths      []string `json:"paths"`
	Exclude    []string `json:"exclude,omitempty"`
	Unreleased bool     `json:"unreleased,omitempty"`
}

type file struct {
	Packages map[string]entry `json:"packages"`
}

type Package struct {
	Name       string
	Kind       Kind
	Version    string
	Dir        string
	Tag        string
	Release    string
	Needs      []string
	Unreleased bool

	paths   []string
	exclude []string
}

type Manifest struct {
	packages map[string]Package
	order    []string
}

var (
	version = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
	kinds   = map[Kind]bool{KindGo: true, KindNPM: true, KindVSIX: true}
)

func Parse(text string) (*Manifest, error) {
	plain, err := jsonc.ToJSON([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	var f file
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	if len(f.Packages) == 0 {
		return nil, fmt.Errorf("%s: no packages", FileName)
	}
	m := &Manifest{packages: make(map[string]Package, len(f.Packages))}
	for name, e := range f.Packages {
		pkg, err := build(name, e)
		if err != nil {
			return nil, err
		}
		m.packages[name] = pkg
	}
	if err := m.resolve(); err != nil {
		return nil, err
	}
	return m, nil
}

func build(name string, e entry) (Package, error) {
	if !kinds[e.Kind] {
		return Package{}, fmt.Errorf("%s: package %q has an unknown kind %q", FileName, name, e.Kind)
	}
	if !version.MatchString(e.Version) {
		return Package{}, fmt.Errorf("%s: package %q has version %q, which is not a semantic version", FileName, name, e.Version)
	}
	if len(e.Paths) == 0 {
		return Package{}, fmt.Errorf("%s: package %q owns no paths, so nothing can tell whether it changed", FileName, name)
	}
	for _, pattern := range append(append([]string{}, e.Paths...), e.Exclude...) {
		if !doublestar.ValidatePattern(pattern) {
			return Package{}, fmt.Errorf("%s: package %q has an invalid pattern %q", FileName, name, pattern)
		}
	}
	if e.Kind != KindGo && e.Dir == "" {
		return Package{}, fmt.Errorf("%s: package %q needs a dir to build from", FileName, name)
	}
	tag := e.Tag
	if tag == "" {
		tag = "{name}@{version}"
	}
	return Package{
		Name:       name,
		Kind:       e.Kind,
		Version:    e.Version,
		Dir:        e.Dir,
		Tag:        tag,
		Release:    e.Release,
		Needs:      e.Needs,
		Unreleased: e.Unreleased,
		paths:      e.Paths,
		exclude:    e.Exclude,
	}, nil
}

func (m *Manifest) resolve() error {
	names := make([]string, 0, len(m.packages))
	for name := range m.packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, need := range m.packages[name].Needs {
			if _, ok := m.packages[need]; !ok {
				return fmt.Errorf("%s: package %q needs %q, which is not in the manifest", FileName, name, need)
			}
		}
	}
	order, err := sortNeeds(names, m.packages)
	if err != nil {
		return err
	}
	m.order = order
	return nil
}

func sortNeeds(names []string, packages map[string]Package) ([]string, error) {
	const (
		open = 1
		done = 2
	)
	mark := map[string]int{}
	order := make([]string, 0, len(names))
	var visit func(string, []string) error
	visit = func(name string, path []string) error {
		switch mark[name] {
		case done:
			return nil
		case open:
			return fmt.Errorf("%s: %s needs itself through %s", FileName, name, strings.Join(append(path, name), " -> "))
		}
		mark[name] = open
		needs := append([]string{}, packages[name].Needs...)
		sort.Strings(needs)
		for _, need := range needs {
			if err := visit(need, append(path, name)); err != nil {
				return err
			}
		}
		mark[name] = done
		order = append(order, name)
		return nil
	}
	for _, name := range names {
		if err := visit(name, nil); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func (m *Manifest) Packages() []Package {
	list := make([]Package, 0, len(m.order))
	for _, name := range m.order {
		list = append(list, m.packages[name])
	}
	return list
}

func (m *Manifest) Package(name string) (Package, bool) {
	pkg, ok := m.packages[name]
	return pkg, ok
}

func (p Package) TagName() string {
	tag := strings.ReplaceAll(p.Tag, "{version}", p.Version)
	return strings.ReplaceAll(tag, "{name}", p.Name)
}

func (p Package) Owns(path string) bool {
	path = strings.TrimPrefix(path, "./")
	for _, pattern := range p.exclude {
		if matches(pattern, path) {
			return false
		}
	}
	for _, pattern := range p.paths {
		if matches(pattern, path) {
			return true
		}
	}
	return false
}

func matches(pattern, path string) bool {
	ok, err := doublestar.Match(pattern, path)
	return err == nil && ok
}

type Lookup func(Package) (bool, error)

type Changes func(Package) (bool, error)

type Status struct {
	Package Package `json:"-"`
	Name    string  `json:"name"`
	Kind    Kind    `json:"kind"`
	Version string  `json:"version"`
	Tag     string  `json:"tag"`
	State   State   `json:"state"`
}

func Plan(m *Manifest, published Lookup, changed Changes) ([]Status, error) {
	statuses := make([]Status, 0, len(m.order))
	for _, pkg := range m.Packages() {
		state, err := stateOf(pkg, published, changed)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, Status{
			Package: pkg,
			Name:    pkg.Name,
			Kind:    pkg.Kind,
			Version: pkg.Version,
			Tag:     pkg.TagName(),
			State:   state,
		})
	}
	return statuses, nil
}

func stateOf(pkg Package, published Lookup, changed Changes) (State, error) {
	out, err := published(pkg)
	if err != nil {
		return "", err
	}
	if !out {
		return Publish, nil
	}
	if pkg.Unreleased {
		return Current, nil
	}
	touched, err := changed(pkg)
	if err != nil {
		return "", err
	}
	if touched {
		return Unreleased, nil
	}
	return Current, nil
}

func Pending(statuses []Status) []Status {
	pending := make([]Status, 0, len(statuses))
	for _, status := range statuses {
		if status.State == Publish {
			pending = append(pending, status)
		}
	}
	return pending
}

func Problems(statuses []Status) []string {
	var problems []string
	for _, status := range statuses {
		if status.State != Unreleased {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s: %s changed since %s went out; raise its version in %s or mark it \"unreleased\": true",
			FileName, status.Name, status.Tag, FileName))
	}
	return problems
}

func Render(statuses []Status) string {
	width := 0
	for _, status := range statuses {
		if len(status.Name) > width {
			width = len(status.Name)
		}
	}
	var b strings.Builder
	for _, status := range statuses {
		fmt.Fprintf(&b, "%-*s  %-8s  %-10s  %s\n", width, status.Name, status.Kind, status.Version, status.State)
	}
	return b.String()
}
