package examplecheck

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/apptivitypl/gopage/internal/scaffold"
)

const Root = "examples"

type Example struct {
	Name     string
	Template string
	Locales  []string
}

func Examples() []Example {
	return []Example{
		{Name: "hello-world", Template: "hello-world", Locales: []string{"en"}},
		{Name: "blog", Template: "blog", Locales: []string{"en"}},
	}
}

func (e Example) Dir() string {
	return path.Join(Root, e.Name)
}

func (e Example) Module() string {
	return "github.com/apptivitypl/gopage/" + e.Dir()
}

func (e Example) Config(version string) scaffold.Config {
	return scaffold.Config{
		Module:        e.Module(),
		Name:          e.Name,
		Template:      e.Template,
		GopageVersion: version,
		Locales:       e.Locales,
		Nav:           scaffold.NavPartial,
		CSS:           scaffold.CSSTailwind,
		Theme:         scaffold.ThemeToggle,
		React:         scaffold.ReactOn,
	}
}

func Skipped() []string {
	return []string{"internal", "dist", ".gopage", ".wrangler", "node_modules"}
}

func Extras() []string {
	return []string{
		"README.md", "go.sum", "wrangler.jsonc", "pnpm-lock.yaml",
		".devcontainer", ".codesandbox",
	}
}

type Kind string

const (
	Missing Kind = "missing"
	Extra   Kind = "extra"
	Changed Kind = "changed"
)

type Difference struct {
	Example string
	Path    string
	Kind    Kind
}

func (d Difference) Message() string {
	switch d.Kind {
	case Missing:
		return fmt.Sprintf("%s: %s is not committed", d.Example, d.Path)
	case Extra:
		return fmt.Sprintf("%s: %s is committed but the template does not write it", d.Example, d.Path)
	default:
		return fmt.Sprintf("%s: %s differs from what the template writes", d.Example, d.Path)
	}
}

func Fingerprint(fsys fs.FS) (map[string][sha256.Size]byte, error) {
	sums := map[string][sha256.Size]byte{}
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name != "." && skipped(name) {
				return fs.SkipDir
			}
			return nil
		}
		if skipped(name) || strings.HasSuffix(name, ".log") {
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if name == "go.mod" {
			data = direct(data)
		}
		sums[name] = sha256.Sum256(data)
		return nil
	})
	return sums, err
}

func direct(module []byte) []byte {
	var kept []string
	var block []string
	inside := false
	for _, line := range strings.Split(string(module), "\n") {
		switch {
		case strings.HasPrefix(line, "require ("):
			inside, block = true, nil
		case inside && strings.HasPrefix(line, ")"):
			inside = false
			if len(block) > 0 {
				kept = append(kept, "require (")
				kept = append(kept, block...)
				kept = append(kept, ")")
			}
		case inside:
			if !strings.HasSuffix(strings.TrimSpace(line), "// indirect") {
				block = append(block, line)
			}
		default:
			if !strings.HasSuffix(strings.TrimSpace(line), "// indirect") {
				kept = append(kept, line)
			}
		}
	}
	return []byte(strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n")
}

func skipped(name string) bool {
	head, _, _ := strings.Cut(name, "/")
	for _, skip := range Skipped() {
		if head == skip {
			return true
		}
	}
	for _, extra := range Extras() {
		if head == extra || name == extra {
			return true
		}
	}
	return false
}

func Compare(name string, committed, generated fs.FS) ([]Difference, error) {
	here, err := Fingerprint(committed)
	if err != nil {
		return nil, err
	}
	wanted, err := Fingerprint(generated)
	if err != nil {
		return nil, err
	}
	var differences []Difference
	for path, sum := range wanted {
		held, ok := here[path]
		switch {
		case !ok:
			differences = append(differences, Difference{Example: name, Path: path, Kind: Missing})
		case held != sum:
			differences = append(differences, Difference{Example: name, Path: path, Kind: Changed})
		}
	}
	for path := range here {
		if _, ok := wanted[path]; !ok {
			differences = append(differences, Difference{Example: name, Path: path, Kind: Extra})
		}
	}
	sort.Slice(differences, func(i, j int) bool {
		if differences[i].Path == differences[j].Path {
			return differences[i].Kind < differences[j].Kind
		}
		return differences[i].Path < differences[j].Path
	})
	return differences, nil
}

func Render(differences []Difference) string {
	if len(differences) == 0 {
		return "example: ok\n"
	}
	var b strings.Builder
	for _, difference := range differences {
		fmt.Fprintf(&b, "example: %s\n", difference.Message())
	}
	fmt.Fprintf(&b, "run `go run ./cmd/gopagetool example --update` to write what the template says\n")
	return b.String()
}
