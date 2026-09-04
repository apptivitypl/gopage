package bundle

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/evanw/esbuild/pkg/api"

	"github.com/apptivitypl/gopage/internal/paths"
)

const (
	PropsPrefix   = "gopage:props/"
	RuntimePrefix = "gopage.client."
	IslandPrefix  = "island."
	entryName     = "gopage-entry.ts"
	runtimeName   = "gopage-runtime.ts"
	outDir        = "gopage-bundle"
	workDir       = paths.CacheDir
	islandDir     = "islands"
)

const (
	EngineReact  = "react"
	EnginePreact = "preact"
	ReactPrefix  = "gopage:react"
	mountSuffix  = ".mount.ts"
)

type Result struct {
	Name  string
	Bytes []byte
}

type Output struct {
	Files    []Result
	Metafile string
}

type Island struct {
	Source string
	Code   string
	Lang   string
	React  bool
}

func (i Island) Inline() bool {
	return i.Code != ""
}

type Options struct {
	Dir        string
	Islands    map[string]Island
	Navigation bool
	Minify     bool
	Target     api.Target
	Engine     string
}

//go:embed runtime.ts
var runtimeSource string

func RuntimeSource() string {
	return runtimeSource
}

func Build(opts Options) (Output, error) {
	if len(opts.Islands) == 0 && !opts.Navigation {
		return Output{}, nil
	}
	entry, err := writeEntry(opts)
	if err != nil {
		return Output{}, err
	}
	defer func() {
		_ = os.Remove(entry)
		_ = os.Remove(filepath.Join(opts.Dir, workDir, runtimeName))
	}()

	engine := engineOf(opts.Engine)
	result := api.Build(api.BuildOptions{
		Plugins:           []api.Plugin{propsPlugin(), reactPlugin(opts.Dir)},
		EntryPoints:       []string{entry},
		Bundle:            true,
		Splitting:         true,
		Format:            api.FormatESModule,
		Target:            target(opts.Target),
		MinifyWhitespace:  opts.Minify,
		MinifyIdentifiers: opts.Minify,
		MinifySyntax:      opts.Minify,
		Write:             false,
		Metafile:          true,
		Outdir:            outDir,
		EntryNames:        "gopage.client.[hash]",
		PublicPath:        "/assets/",
		ChunkNames:        "island.[hash]",
		JSX:               api.JSXAutomatic,
		JSXImportSource:   engine,
		Alias:             aliases(engine),
		LogLevel:          api.LogLevelSilent,
		AbsWorkingDir:     opts.Dir,
	})
	if name, ok := missingPackage(result.Errors); ok {
		return Output{}, fmt.Errorf("bundling islands: %s is not installed; run pnpm add %s (or npm install %s) in %s",
			name, packagesFor(engine), packagesFor(engine), opts.Dir)
	}
	if len(result.Errors) > 0 {
		return Output{}, fmt.Errorf("bundling islands: %s", messages(result.Errors))
	}
	files := make([]Result, 0, len(result.OutputFiles))
	for _, file := range result.OutputFiles {
		files = append(files, Result{Name: filepath.Base(file.Path), Bytes: file.Contents})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return Output{Files: files, Metafile: result.Metafile}, nil
}

func packagesFor(engine string) string {
	if engine == EnginePreact {
		return "preact"
	}
	return "react react-dom"
}

func engineOf(engine string) string {
	if engine == "" {
		return EngineReact
	}
	return engine
}

func aliases(engine string) map[string]string {
	if engine != EnginePreact {
		return nil
	}
	return map[string]string{
		"react":                 "preact/compat",
		"react-dom":             "preact/compat",
		"react-dom/client":      "preact/compat",
		"react/jsx-runtime":     "preact/jsx-runtime",
		"react/jsx-dev-runtime": "preact/jsx-dev-runtime",
	}
}

const reactNamespace = "gopage-react"

const reactAdapter = `import { createElement } from "react";
import { createRoot } from "react-dom/client";

export function react(Component) {
	return (element, props) => {
		const root = createRoot(element);
		root.render(createElement(Component, props));
		return () => root.unmount();
	};
}
`

func reactPlugin(dir string) api.Plugin {
	return api.Plugin{
		Name: reactNamespace,
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: "^" + ReactPrefix + "$"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				return api.OnResolveResult{Path: args.Path, Namespace: reactNamespace}, nil
			})
			build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: reactNamespace}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
				contents := reactAdapter
				return api.OnLoadResult{Contents: &contents, Loader: api.LoaderJS, ResolveDir: dir}, nil
			})
		},
	}
}

const propsNamespace = "gopage-props"

func propsPlugin() api.Plugin {
	return api.Plugin{
		Name: "gopage-props",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: "^gopage:props/"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				name := strings.TrimPrefix(args.Path, PropsPrefix)
				if name == "" || strings.ContainsAny(name, `/\.`) {
					return api.OnResolveResult{}, fmt.Errorf("%s is not a component name", args.Path)
				}
				return api.OnResolveResult{Path: args.Path, Namespace: propsNamespace}, nil
			})
			build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: propsNamespace}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
				contents := "export {};\n"
				return api.OnLoadResult{Contents: &contents, Loader: api.LoaderTS}, nil
			})
		},
	}
}

func target(requested api.Target) api.Target {
	if requested == api.DefaultTarget {
		return api.ES2020
	}
	return requested
}

func writeEntry(opts Options) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "import { navigation, register, start } from %q;\n", "./"+runtimeName)
	for _, name := range names(opts.Islands) {
		fmt.Fprintf(&b, "register(%q, () => import(%q));\n", name, entryOf(opts.Islands[name], name))
	}
	b.WriteString("start();\n")
	if opts.Navigation {
		b.WriteString("navigation();\n")
	}

	work := filepath.Join(opts.Dir, workDir)
	if err := os.MkdirAll(filepath.Join(work, islandDir), 0o755); err != nil {
		return "", err
	}
	for name, island := range opts.Islands {
		if err := writeIsland(work, name, island); err != nil {
			return "", err
		}
	}
	sources := map[string]string{runtimeName: runtimeSource, entryName: b.String()}
	for name, body := range sources {
		if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o644); err != nil {
			return "", err
		}
	}
	return filepath.Join(work, entryName), nil
}

func writeIsland(work, name string, island Island) error {
	if island.Inline() {
		target := filepath.Join(work, islandDir, name+"."+langOf(island.Lang))
		if err := os.WriteFile(target, []byte(island.Code), 0o644); err != nil {
			return err
		}
	}
	if !island.React {
		return nil
	}
	wrapper := fmt.Sprintf("import Component from %q;\nimport { react } from %q;\n\nexport const mount = react(Component);\n",
		fromIslands(island, name), ReactPrefix)
	return os.WriteFile(filepath.Join(work, islandDir, name+mountSuffix), []byte(wrapper), 0o644)
}

func entryOf(island Island, name string) string {
	if island.React {
		return "./" + islandDir + "/" + name + mountSuffix
	}
	return fromCache(island, name)
}

func fromCache(island Island, name string) string {
	if island.Inline() {
		return "./" + islandDir + "/" + name + "." + langOf(island.Lang)
	}
	return upward(workDir) + island.Source
}

func fromIslands(island Island, name string) string {
	if island.Inline() {
		return "./" + name + "." + langOf(island.Lang)
	}
	return upward(workDir+"/"+islandDir) + island.Source
}

func upward(dir string) string {
	return strings.Repeat("../", strings.Count(dir, "/")+1)
}

func langOf(lang string) string {
	if lang == "" {
		return "ts"
	}
	return lang
}

func names(islands map[string]Island) []string {
	out := make([]string, 0, len(islands))
	for name := range islands {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func messages(errors []api.Message) string {
	parts := make([]string, 0, len(errors))
	for _, item := range errors {
		parts = append(parts, describe(item))
	}
	return strings.Join(parts, "; ")
}

func missingPackage(errors []api.Message) (string, bool) {
	for _, item := range errors {
		rest, ok := strings.CutPrefix(item.Text, `Could not resolve "`)
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, `"`)
		if name == "react" || name == "react-dom" || name == "react-dom/client" ||
			strings.HasPrefix(name, "react/") || strings.HasPrefix(name, "preact") {
			return name, true
		}
	}
	return "", false
}

func describe(item api.Message) string {
	if item.Location == nil {
		return item.Text
	}
	return fmt.Sprintf("%s:%d: %s", item.Location.File, item.Location.Line, item.Text)
}

type metaOutput struct {
	EntryPoint string `json:"entryPoint"`
	Imports    []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"imports"`
	Inputs map[string]struct{} `json:"inputs"`
}

type metafile struct {
	Outputs map[string]metaOutput `json:"outputs"`
}

func EagerChunks(out Output) map[string]bool {
	eager := map[string]bool{}
	var meta metafile
	if err := json.Unmarshal([]byte(out.Metafile), &meta); err != nil {
		return eager
	}
	for name, output := range meta.Outputs {
		if output.EntryPoint == "" || !strings.HasPrefix(filepath.Base(name), RuntimePrefix) {
			continue
		}
		for _, imported := range output.Imports {
			if imported.Kind == "import-statement" {
				eager[filepath.Base(imported.Path)] = true
			}
		}
	}
	return eager
}

func IslandChunks(out Output, islands map[string]Island) map[string][]string {
	var meta metafile
	if err := json.Unmarshal([]byte(out.Metafile), &meta); err != nil {
		return nil
	}
	byInput := map[string]string{}
	for name, output := range meta.Outputs {
		for input := range output.Inputs {
			byInput[input] = name
		}
	}
	result := make(map[string][]string, len(islands))
	for name, island := range islands {
		chunk, ok := byInput[moduleOf(island, name)]
		if !ok {
			continue
		}
		result[name] = closure(meta, chunk)
	}
	return result
}

func moduleOf(island Island, name string) string {
	switch {
	case island.React:
		return workDir + "/" + islandDir + "/" + name + mountSuffix
	case island.Inline():
		return workDir + "/" + islandDir + "/" + name + "." + langOf(island.Lang)
	default:
		return island.Source
	}
}

func closure(meta metafile, chunk string) []string {
	seen := map[string]bool{}
	var order []string
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		order = append(order, filepath.Base(name))
		for _, imported := range meta.Outputs[name].Imports {
			if imported.Kind == "import-statement" {
				walk(imported.Path)
			}
		}
	}
	walk(chunk)
	sort.Strings(order)
	return order
}
