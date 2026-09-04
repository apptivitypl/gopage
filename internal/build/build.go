package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/bundle"
	"github.com/apptivitypl/gopage/internal/codegen"
	"github.com/apptivitypl/gopage/internal/compile"
	"github.com/apptivitypl/gopage/internal/compress"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/css"
	"github.com/apptivitypl/gopage/internal/demo"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/gotoolchain"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/paths"
	"github.com/apptivitypl/gopage/internal/server"
)

type Target string

const (
	TargetNative  Target = "native"
	TargetWorkers Target = "workers"
	TargetDemo    Target = "demo"
)

func Targets() []string {
	return []string{string(TargetNative), string(TargetWorkers), string(TargetDemo)}
}

const AssetsGen = "github.com/syumai/workers/cmd/workers-assets-gen@v0.33.0"

type Command struct {
	Dir  string
	Env  []string
	Name string
	Args []string
}

type Runner interface {
	Run(Command) error
}

type Options struct {
	Dir        string
	Styles     css.Processor
	Target     Target
	Name       string
	Module     string
	CompatDate string
	Runner     Runner
	Go         gotoolchain.Resolved
}

type Report struct {
	Dir          string
	Routes       []compile.Route
	Islands      []string
	StaticPages  []string
	ManifestSize int
	Generated    int
	Diagnostics  []diag.Diagnostic
}

type Error struct {
	Diagnostics []diag.Diagnostic
	Sources     map[string]string
}

func (e *Error) Error() string {
	return fmt.Sprintf("build stopped: %d errors", len(e.Diagnostics))
}

func (e *Error) Render() string {
	var b strings.Builder
	for _, d := range e.Diagnostics {
		b.WriteString(diag.Render(d, e.Sources[d.File]))
		b.WriteString("\n")
	}
	return b.String()
}

func Run(opts Options) (Report, error) {
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Report{}, err
	}
	opts.Dir = dir
	opts.Go.Env = slices.Concat(opts.Go.Env, OutsideWorkspace(opts.Dir, opts.Go.Command()))
	if opts.Target == "" {
		opts.Target = TargetNative
	}
	if opts.CompatDate == "" {
		opts.CompatDate = "2025-01-01"
	}
	if opts.Name == "" {
		opts.Name = filepath.Base(opts.Dir)
	}

	fsys := os.DirFS(opts.Dir)
	settings, err := config.Load(fsys)
	if err != nil {
		return Report{}, err
	}
	styles := stylesheet(opts, settings)
	bundled, err := bundleIslands(opts.Dir, fsys, settings.Nav.Differential(), settings.Client.React)
	if err != nil {
		return Report{}, err
	}
	var bag diag.Bag
	result, err := compile.CompileWith(fsys, &bag, compile.Options{
		Extra:     assetsOf(bundled),
		Transform: styles.transform,
		Inline:    inlineStyle(settings.CSS.Inline()),
	})
	if err != nil {
		return Report{}, err
	}
	if bag.HasErrors() {
		return Report{}, &Error{Diagnostics: bag.Sorted(), Sources: sourcesOf(opts.Dir, bag.Sorted())}
	}

	packages, err := writeGenerated(opts.Dir, opts.Module, result)
	if err != nil {
		return Report{}, err
	}

	if err := writeConfig(opts.Dir, settings); err != nil {
		return Report{}, err
	}

	manifest := ir.Encode(result.Manifest)
	if err := writeFile(filepath.Join(opts.Dir, filepath.FromSlash(paths.Manifest)), manifest); err != nil {
		return Report{}, err
	}

	if err := writeAssets(opts.Dir, fsys, result.Assets, styles); err != nil {
		return Report{}, err
	}
	if err := writeBundles(opts.Dir, bundled, sidecarOf(fsys, bundled, result)); err != nil {
		return Report{}, err
	}
	if err := writePublic(opts.Dir, fsys); err != nil {
		return Report{}, err
	}
	if err := writeIslandTypes(opts.Dir, result); err != nil {
		return Report{}, err
	}
	if err := writeFile(filepath.Join(opts.Dir, filepath.FromSlash(paths.Inventory)),
		[]byte(compile.Inventory(result.Classes))); err != nil {
		return Report{}, err
	}

	pages, err := writeStaticPages(opts.Dir, result.Manifest, settings)
	if err != nil {
		return Report{}, err
	}
	if err := writeRedirects(opts.Dir, settings); err != nil {
		return Report{}, err
	}
	if err := writeHeaders(opts.Dir); err != nil {
		return Report{}, err
	}

	report := Report{
		Dir:          opts.Dir,
		Routes:       result.Routes,
		Islands:      islandNames(result),
		StaticPages:  pages,
		ManifestSize: len(manifest),
		Generated:    len(packages),
		Diagnostics:  bag.Sorted(),
	}
	switch opts.Target {
	case TargetWorkers:
		return report, buildWorker(opts, workerPatterns(result))
	case TargetDemo:
		return report, buildDemo(opts, workerPatterns(result))
	default:
		return report, buildNative(opts)
	}
}

func moduleOf(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path), nil
		}
	}
	return "", fmt.Errorf("go.mod in %s has no module directive", dir)
}

func sourcesOf(dir string, diagnostics []diag.Diagnostic) map[string]string {
	sources := map[string]string{}
	for _, d := range diagnostics {
		if _, done := sources[d.File]; done {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(d.File)))
		if err == nil {
			sources[d.File] = string(data)
		}
	}
	return sources
}

func writeConfig(dir string, settings config.Config) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, filepath.FromSlash(paths.GenConfig)), append(data, '\n'))
}

func bundleIslands(dir string, fsys fs.FS, navigation bool, engine string) (bundle.Output, error) {
	islands := compile.DiscoverIslands(fsys)
	if len(islands) == 0 && !navigation {
		return bundle.Output{}, nil
	}
	entries := make(map[string]bundle.Island, len(islands))
	for name, island := range islands {
		entries[name] = bundle.Island{Source: island.Client, Code: island.Code, Lang: island.Lang, React: island.React}
	}
	return bundle.Build(bundle.Options{
		Dir:        dir,
		Islands:    entries,
		Navigation: navigation,
		Minify:     true,
		Engine:     engine,
	})
}

func writeIslandTypes(dir string, result compile.Result) error {
	var declared []string
	for name := range result.Islands {
		component, ok := result.Components[name]
		if !ok {
			continue
		}
		source := codegen.TypeScript(component.Schema)
		if source == "" {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(paths.PropsDir), name+".ts")
		if err := writeFile(target, []byte(source)); err != nil {
			return err
		}
		declared = append(declared, name)
	}
	sort.Strings(declared)
	return writeFile(filepath.Join(dir, filepath.FromSlash(paths.PropsTypes)), declarations(declared))
}

func declarations(names []string) []byte {
	var b strings.Builder
	b.WriteString("// Code generated by gopage. DO NOT EDIT.\n\n")
	for _, name := range names {
		fmt.Fprintf(&b, "declare module %q {\n\texport * from \"./props/%s\";\n}\n\n", bundle.PropsPrefix+name, name)
	}
	return []byte(b.String())
}

func assetsOf(bundled bundle.Output) []assets.Asset {
	eager := bundle.EagerChunks(bundled)
	list := make([]assets.Asset, 0, len(bundled.Files))
	for _, item := range bundled.Files {
		asset := assets.Describe(item.Name, item.Bytes)
		switch {
		case strings.HasPrefix(item.Name, bundle.RuntimePrefix):
		case eager[item.Name]:
			asset.Kind = assets.KindModule
		default:
			asset.Kind = assets.KindOther
		}
		list = append(list, asset)
	}
	return list
}

func writePublic(dir string, fsys fs.FS) error {
	if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(paths.GenPublic))); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, filepath.FromSlash(paths.GenPublic), ".keep"), nil); err != nil {
		return err
	}
	list, err := assets.Public(fsys)
	if err != nil {
		return err
	}
	for _, asset := range list {
		content, err := fs.ReadFile(fsys, asset.Source)
		if err != nil {
			return err
		}
		relative := strings.TrimPrefix(asset.Path, "/")
		targets := []string{
			filepath.Join(dir, filepath.FromSlash(paths.GenPublic), filepath.FromSlash(relative)),
			filepath.Join(dir, filepath.FromSlash(paths.AssetsDir), filepath.FromSlash(relative)),
		}
		for _, target := range targets {
			if err := writeFile(target, content); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeBundles(dir string, bundled bundle.Output, sidecar assets.Sidecar) error {
	if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(paths.GenBundles))); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, filepath.FromSlash(paths.GenBundles), ".keep"), nil); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, filepath.FromSlash(paths.GenBundles), assets.PreloadFile), sidecar.Bytes()); err != nil {
		return err
	}
	for _, item := range bundled.Files {
		asset := assets.Describe(item.Name, item.Bytes)
		target := filepath.Join(dir, filepath.FromSlash(paths.AssetsDir), filepath.FromSlash(strings.TrimPrefix(asset.Path, "/")))
		if err := writeFile(target, item.Bytes); err != nil {
			return err
		}
		site := filepath.Join(dir, filepath.FromSlash(paths.GenBundles), filepath.FromSlash(item.Name))
		if err := writeServed(site, asset.Type, item.Bytes); err != nil {
			return err
		}
	}
	return nil
}

func writeServed(path, contentType string, content []byte) error {
	if err := writeFile(path, content); err != nil {
		return err
	}
	if !compress.Compressible(contentType, len(content)) {
		return nil
	}
	variants := map[string]func(io.Writer, []byte) error{
		assets.BrotliSuffix: compress.Brotli,
		assets.GzipSuffix:   compress.Gzip,
	}
	for suffix, compress := range variants {
		file, err := os.Create(path + suffix)
		if err != nil {
			return err
		}
		if err := errors.Join(compress(file, content), file.Close()); err != nil {
			return err
		}
	}
	return nil
}

func minified(asset assets.Asset, content []byte) ([]byte, error) {
	if asset.Kind != assets.KindStyle {
		return content, nil
	}
	return bundle.MinifyCSS(content)
}

func writeAssets(dir string, fsys fs.FS, list []assets.Asset, styles styles) error {
	for _, stale := range []string{paths.GenStyles, paths.AssetsDir + "/assets"} {
		if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(stale))); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(dir, filepath.FromSlash(paths.GenStyles), ".keep"), nil); err != nil {
		return err
	}
	for _, asset := range list {
		if !strings.HasPrefix(asset.Source, assets.Dir+"/") {
			continue
		}
		content, err := fs.ReadFile(fsys, asset.Source)
		if err != nil {
			return err
		}
		if content, err = styles.transform(asset, content); err != nil {
			return err
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(asset.Source, assets.Dir), "/")
		if err := writeServed(filepath.Join(dir, filepath.FromSlash(paths.GenStyles), filepath.FromSlash(relative)),
			asset.Type, content); err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(paths.AssetsDir), filepath.FromSlash(strings.TrimPrefix(asset.Path, "/")))
		if err := writeFile(target, content); err != nil {
			return err
		}
	}
	return nil
}

func writeHeaders(dir string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s*\n  Cache-Control: %s\n\n", assets.Prefix, assets.CacheControl)
	b.WriteString("/*\n")
	for _, header := range server.SecurityHeaders() {
		fmt.Fprintf(&b, "  %s: %s\n", header.Name, header.Value)
	}
	return writeFile(filepath.Join(dir, filepath.FromSlash(paths.Headers)), []byte(b.String()))
}

func writeRedirects(dir string, settings config.Config) error {
	var b strings.Builder
	for _, redirect := range settings.Redirects {
		fmt.Fprintf(&b, "%s %s %d\n", redirect.From, redirect.To, redirect.Status)
	}
	if settings.I18n.Mode == config.ModePath && !settings.I18n.PrefixDefault {
		fmt.Fprintf(&b, "/%s/* /:splat 301\n", settings.I18n.DefaultLocale)
	}
	if b.Len() == 0 {
		return nil
	}
	return writeFile(filepath.Join(dir, filepath.FromSlash(paths.Redirects)), []byte(b.String()))
}

func writeStaticPages(dir string, manifest *ir.Manifest, settings config.Config) ([]string, error) {
	app := server.New(server.Options{Manifest: manifest, Config: settings})
	var written []string
	for _, route := range manifest.Routes {
		if route.Class != ir.ClassStatic {
			continue
		}
		body, err := app.RenderStatic(route)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", route.Pattern, err)
		}
		target := filepath.Join(dir, filepath.FromSlash(paths.AssetsDir), assetPath(route.Pattern))
		if err := writeFile(target, body); err != nil {
			return nil, err
		}
		written = append(written, assetPath(route.Pattern))
	}
	return written, nil
}

func assetPath(pattern string) string {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return "index.html"
	}
	return filepath.Join(filepath.FromSlash(trimmed), "index.html")
}

func Tidy(dir string, tool gotoolchain.Resolved, runner Runner) error {
	env := slices.Concat(tool.Env, OutsideWorkspace(dir, tool.Command()))
	return runner.Run(Command{Dir: dir, Env: env, Name: tool.Command(), Args: []string{"mod", "tidy"}})
}

var packageManagers = []struct {
	name string
	args []string
}{
	{"pnpm", []string{"install"}},
	{"npm", []string{"install", "--no-fund", "--no-audit"}},
	{"bun", []string{"install"}},
}

func PackageManager(look func(string) (string, error)) string {
	for _, manager := range packageManagers {
		if _, err := look(manager.name); err == nil {
			return manager.name
		}
	}
	return ""
}

func Install(dir, manager string, runner Runner) error {
	for _, known := range packageManagers {
		if known.name == manager {
			return runner.Run(Command{Dir: dir, Name: manager, Args: known.args})
		}
	}
	return fmt.Errorf("%s is not a package manager gopage knows how to run", manager)
}

func buildNative(opts Options) error {
	return opts.Runner.Run(Command{
		Dir:  opts.Dir,
		Env:  opts.Go.Env,
		Name: opts.Go.Command(),
		Args: []string{"build", "-o", paths.Server(), paths.ServerMain},
	})
}

func workerPatterns(result compile.Result) []string {
	seen := map[string]bool{compile.APIPrefix + "/*": true}
	patterns := []string{compile.APIPrefix + "/*"}
	for _, route := range result.Manifest.Routes {
		if route.Class != ir.ClassDynamic {
			continue
		}
		pattern := workerGlob(route.Pattern)
		if !seen[pattern] {
			seen[pattern] = true
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	return patterns
}

func workerGlob(pattern string) string {
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, "[") {
			return strings.Join(append(segments[:i], "*"), "/")
		}
	}
	return pattern
}

func buildWorker(opts Options, patterns []string) error {
	steps := []Command{
		{
			Dir:  opts.Dir,
			Env:  opts.Go.Env,
			Name: opts.Go.Command(),
			Args: []string{"run", AssetsGen, "-mode=go", "-o", paths.WorkerDir},
		},
		{
			Dir:  opts.Dir,
			Env:  append([]string{"GOOS=js", "GOARCH=wasm"}, opts.Go.Env...),
			Name: opts.Go.Command(),
			Args: []string{"build", "-o", paths.WorkerBinary, "-ldflags=-s -w", paths.WorkerMain},
		},
	}
	for _, step := range steps {
		if err := opts.Runner.Run(step); err != nil {
			return err
		}
	}
	return writeWrangler(opts, patterns)
}

func buildDemo(opts Options, patterns []string) error {
	steps := []Command{
		{
			Dir:  opts.Dir,
			Env:  opts.Go.Env,
			Name: opts.Go.Command(),
			Args: []string{"run", AssetsGen, "-mode=go", "-runtime=browser", "-o", paths.DemoDir},
		},
		{
			Dir:  opts.Dir,
			Env:  append([]string{"GOOS=js", "GOARCH=wasm"}, opts.Go.Env...),
			Name: opts.Go.Command(),
			Args: []string{"build", "-o", paths.DemoBinary, "-ldflags=-s -w", paths.WorkerMain},
		},
	}
	for _, step := range steps {
		if err := opts.Runner.Run(step); err != nil {
			return err
		}
	}
	target := filepath.Join(opts.Dir, filepath.FromSlash(paths.DemoDir))
	if err := demo.Write(target, demo.Meta{Name: opts.Name, WorkerFirst: patterns}); err != nil {
		return err
	}
	return copyTree(
		filepath.Join(opts.Dir, filepath.FromSlash(paths.AssetsDir)),
		filepath.Join(opts.Dir, filepath.FromSlash(paths.DemoAssets)),
	)
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(to, relative), data)
	})
}

type wranglerAssets struct {
	Directory        string `json:"directory"`
	Binding          string `json:"binding"`
	NotFoundHandling string `json:"not_found_handling"`
}

type wranglerObservability struct {
	Enabled bool `json:"enabled"`
}

type wrangler struct {
	Name              string                `json:"name"`
	Main              string                `json:"main"`
	CompatibilityDate string                `json:"compatibility_date"`
	Observability     wranglerObservability `json:"observability"`
	Assets            wranglerAssets        `json:"assets"`
	RunWorkerFirst    []string              `json:"run_worker_first"`
}

func writeWrangler(opts Options, patterns []string) error {
	config := wrangler{
		Name:              opts.Name,
		Main:              "./" + paths.WorkerDir + "/worker.mjs",
		CompatibilityDate: opts.CompatDate,
		Observability:     wranglerObservability{Enabled: true},
		Assets: wranglerAssets{
			Directory:        "./" + paths.AssetsDir,
			Binding:          "ASSETS",
			NotFoundHandling: "none",
		},
		RunWorkerFirst: patterns,
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(opts.Dir, paths.Wrangler), append(data, '\n'))
}

func writeFile(path string, data []byte) error {
	if held, err := os.ReadFile(path); err == nil && bytes.Equal(held, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func islandNames(result compile.Result) []string {
	names := make([]string, 0, len(result.Islands))
	for name := range result.Islands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func inlineStyle(limit int) assets.Inliner {
	return func(_ assets.Asset, data []byte) bool {
		return limit > 0 && len(data) < limit
	}
}

func sidecarOf(fsys fs.FS, bundled bundle.Output, result compile.Result) assets.Sidecar {
	sidecar := assets.Sidecar{Islands: map[string][]string{}}
	linked := make([]assets.Asset, 0, len(result.Assets))
	for _, asset := range result.Assets {
		if asset.Kind == assets.KindOther {
			continue
		}
		linked = append(linked, asset)
	}
	if public, err := assets.Public(fsys); err == nil {
		for _, asset := range public {
			if asset.Kind == assets.KindFont {
				linked = append(linked, asset)
			}
		}
	}
	if link := assets.Link(linked); link != "" {
		sidecar.Links = strings.Split(link, ", ")
	}
	for name := range bundle.EagerChunks(bundled) {
		sidecar.Modules = append(sidecar.Modules, name)
	}
	sort.Strings(sidecar.Modules)
	entries := make(map[string]bundle.Island, len(result.Islands))
	for name, island := range result.Islands {
		entries[name] = bundle.Island{Source: island.Client, Code: island.Code, Lang: island.Lang, React: island.React}
	}
	for name, chunks := range bundle.IslandChunks(bundled, entries) {
		sidecar.Islands[name] = chunks
	}
	return sidecar
}
