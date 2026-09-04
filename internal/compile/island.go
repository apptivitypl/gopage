package compile

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/syntax"
)

const (
	ClientFile      = "client.ts"
	clientAttribute = "client"
	IslandTag       = "gopage-island"
	PropsScriptType = "application/json"
	defaultLang     = "ts"
)

var clientFiles = map[string]string{
	ClientFile:   "ts",
	"client.tsx": "tsx",
	"client.js":  "js",
	"client.jsx": "jsx",
}

var strategies = []string{"load", "idle", "visible", "media"}

func Strategies() []string {
	return slices.Clone(strategies)
}

type Island struct {
	Name   string
	Dir    string
	Client string
	Code   string
	Lang   string
	React  bool
}

func (i Island) Inline() bool {
	return i.Code != ""
}

func exportsMount(code string) bool {
	return strings.Contains(code, "export function mount") || strings.Contains(code, "export const mount")
}

func exportsDefault(code string) bool {
	return strings.Contains(code, "export default")
}

func ReactIsland(code string) bool {
	return exportsDefault(code) && !exportsMount(code)
}

func langOf(lang string) string {
	if lang == "" {
		return defaultLang
	}
	return lang
}

func DiscoverIslands(fsys fs.FS) map[string]Island {
	found := map[string]Island{}
	_ = fs.WalkDir(fsys, ComponentsDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if lang, ok := clientFiles[entry.Name()]; ok {
			dir := path.Dir(file)
			code, _ := fs.ReadFile(fsys, file)
			found[path.Base(dir)] = Island{
				Name: path.Base(dir), Dir: dir, Client: file, Lang: lang, React: ReactIsland(string(code)),
			}
			return nil
		}
		if path.Dir(file) != ComponentsDir || !strings.HasSuffix(entry.Name(), TemplateSuffix) {
			return nil
		}
		source, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil
		}
		var bag diag.Bag
		document := syntax.Parse(file, string(source), &bag)
		script, ok := syntax.ClientScriptOf(document)
		if !ok {
			return nil
		}
		name := strings.TrimSuffix(entry.Name(), TemplateSuffix)
		found[name] = Island{
			Name: name, Dir: ComponentsDir, Client: file, Code: script.Code,
			Lang: langOf(script.Lang), React: ReactIsland(script.Code),
		}
		return nil
	})
	return found
}

func (b *builder) clientScript(node *syntax.ClientScript) {
	if !strings.HasPrefix(b.file, ComponentsDir+"/") {
		b.report(diag.C317, node.Span, "<script client> belongs to a component",
			"move the island into components/, or use a plain <script> tag for page javascript")
		return
	}
	if b.clientSeen {
		b.report(diag.C317, node.Span, "a component takes one <script client>",
			"merge the two blocks, or split the component in two")
		return
	}
	b.clientSeen = true
	switch mount, component := exportsMount(node.Code), exportsDefault(node.Code); {
	case mount && component:
		b.report(diag.C317, node.Span, "<script client> exports both mount and a default component",
			"keep mount for a plain island, or the default export for a react component, not both")
	case !mount && !component:
		b.report(diag.C317, node.Span, "<script client> exports neither mount nor a default component",
			"export function mount(el, props) and return a function that undoes what it set up, "+
				"or export default a react component")
	}
}

func (b *builder) island(node *syntax.Component, component Component, strategy string) {
	if !b.islands[node.Name] {
		b.report(diag.C315, node.Span, fmt.Sprintf("%s has no browser code", node.Name),
			fmt.Sprintf("add <script client> to components/%s%s, write components/%s/%s, or drop client=",
				node.Name, TemplateSuffix, node.Name, ClientFile))
		return
	}
	if !slices.Contains(strategies, strategy) {
		b.report(diag.C315, node.Span, fmt.Sprintf("%s is not an activation strategy", strategy),
			"strategies: "+strings.Join(strategies, ", "))
		return
	}
	b.uses = append(b.uses, ir.IslandUse{Name: node.Name, Strategy: strategy})
	b.static(`<` + IslandTag + ` style="display:contents" name="` + escapeAttribute(node.Name) +
		`" strategy="` + strategy + `"`)
	if strategy == "media" {
		if _, ok := literal(node.Attributes, "media"); !ok {
			b.report(diag.C315, node.Span, `client="media" needs a media query`,
				`write client="media" media="(min-width: 60rem)"`)
			return
		}
	}
	if media, ok := literal(node.Attributes, "media"); ok {
		b.static(` media="` + escapeAttribute(media) + `"`)
	}
	b.static(`><script type="` + PropsScriptType + `">`)
	b.islandProps(node, component)
	b.static(`</script>`)
	b.inline(node, component)
	b.static(`</` + IslandTag + `>`)
}

func (b *builder) islandProps(node *syntax.Component, component Component) {
	arguments := b.arguments(node, component)
	b.static("{")
	written := 0
	for _, argument := range arguments {
		if reservedIslandAttribute(argument.name) {
			continue
		}
		if written > 0 {
			b.static(",")
		}
		b.static(`"` + escapeAttribute(argument.name) + `":`)
		b.emit(ir.Op{Kind: ir.OpJSON, A: argument.expr})
		written++
	}
	b.static("}")
}

func reservedIslandAttribute(name string) bool {
	return name == clientAttribute || name == "media"
}
