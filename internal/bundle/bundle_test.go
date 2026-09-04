package bundle

import (
	"os"
	"runtime"

	"github.com/evanw/esbuild/pkg/api"
	"path/filepath"
	"strings"
	"testing"
)

func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const island = `export function mount(element: HTMLElement, props: { Start: number }): () => void {
	element.dataset.start = String(props.Start);
	return () => {};
}
`

func build(t *testing.T, files map[string]string, islands map[string]string) []Result {
	t.Helper()
	out, err := Build(Options{Dir: project(t, files), Islands: sources(islands), Minify: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return out.Files
}

func sources(islands map[string]string) map[string]Island {
	out := make(map[string]Island, len(islands))
	for name, source := range islands {
		out[name] = Island{Source: source}
	}
	return out
}

func TestTheRuntimeAndEveryIslandAreBundled(t *testing.T) {
	out := build(t,
		map[string]string{"components/Counter/client.ts": island},
		map[string]string{"Counter": "components/Counter/client.ts"})
	if len(out) != 2 {
		t.Fatalf("bundles = %+v", bundleNames(out))
	}
	var entry, chunk Result
	for _, item := range out {
		if strings.HasPrefix(item.Name, RuntimePrefix) {
			entry = item
			continue
		}
		chunk = item
	}
	if entry.Name == "" || chunk.Name == "" {
		t.Fatalf("bundles = %v", bundleNames(out))
	}
	if !strings.Contains(string(entry.Bytes), `"Counter"`) {
		t.Errorf("entry = %s", entry.Bytes)
	}
	if !strings.Contains(string(entry.Bytes), "/assets/"+chunk.Name) {
		t.Errorf("entry = %s, want it to import %s by its public path", entry.Bytes, chunk.Name)
	}
	if !strings.Contains(string(chunk.Bytes), "dataset") {
		t.Errorf("chunk = %s", chunk.Bytes)
	}
}

func TestTheEntryNameCarriesAContentHash(t *testing.T) {
	out := build(t,
		map[string]string{"components/Counter/client.ts": island},
		map[string]string{"Counter": "components/Counter/client.ts"})
	for _, item := range out {
		if strings.HasPrefix(item.Name, RuntimePrefix) && item.Name == RuntimePrefix+"js" {
			t.Errorf("entry %q carries no hash", item.Name)
		}
	}
}

func TestTwoIslandsBecomeTwoChunks(t *testing.T) {
	out := build(t, map[string]string{
		"components/A/client.ts": island,
		"components/B/client.ts": island,
	}, map[string]string{"A": "components/A/client.ts", "B": "components/B/client.ts"})
	if len(out) != 3 {
		t.Errorf("bundles = %v, want the entry plus one chunk each", bundleNames(out))
	}
}

func TestAProjectWithoutIslandsBundlesNothing(t *testing.T) {
	out, err := Build(Options{Dir: t.TempDir()})
	if err != nil || out.Files != nil {
		t.Errorf("out = %v, err = %v", out, err)
	}
}

func TestABrokenIslandIsReported(t *testing.T) {
	_, err := Build(Options{
		Dir:     project(t, map[string]string{"components/A/client.ts": "export function mount( {"}),
		Islands: sources(map[string]string{"A": "components/A/client.ts"}),
	})
	if err == nil || !strings.Contains(err.Error(), "client.ts") {
		t.Errorf("err = %v, want the file named", err)
	}
}

func TestAMissingIslandFileIsReported(t *testing.T) {
	_, err := Build(Options{Dir: t.TempDir(), Islands: sources(map[string]string{"A": "components/A/client.ts"})})
	if err == nil {
		t.Error("an island with no source must be reported")
	}
}

func TestAnUnwritableDirectoryIsReported(t *testing.T) {
	dir := project(t, map[string]string{"components/A/client.ts": island})
	denyWrites(t, dir)
	_, err := Build(Options{Dir: dir, Islands: sources(map[string]string{"A": "components/A/client.ts"})})
	if err == nil {
		t.Error("a directory that cannot hold the entry must be reported")
	}
}

func TestTheRuntimeSourceIsEmbedded(t *testing.T) {
	if !strings.Contains(RuntimeSource(), "export function register") {
		t.Error("the embedded runtime must export register")
	}
	if !strings.Contains(RuntimeSource(), "gopage-island") {
		t.Error("the embedded runtime must look for islands")
	}
}

func TestTemporaryFilesAreRemoved(t *testing.T) {
	dir := project(t, map[string]string{"components/A/client.ts": island})
	if _, err := Build(Options{Dir: dir, Islands: sources(map[string]string{"A": "components/A/client.ts"})}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, name := range []string{entryName, runtimeName} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s survived the build", name)
		}
	}
}

func bundleNames(out []Result) []string {
	list := make([]string, 0, len(out))
	for _, item := range out {
		list = append(list, item.Name)
	}
	return list
}

func TestAnExplicitTargetIsKept(t *testing.T) {
	out, err := Build(Options{
		Dir:     project(t, map[string]string{"components/A/client.ts": island}),
		Islands: sources(map[string]string{"A": "components/A/client.ts"}),
		Target:  api.ES2018,
	})
	if err != nil || len(out.Files) == 0 {
		t.Fatalf("out = %v, err = %v", bundleNames(out.Files), err)
	}
}

func TestErrorsWithoutALocationStillRead(t *testing.T) {
	if got := describe(api.Message{Text: "no location"}); got != "no location" {
		t.Errorf("describe = %q", got)
	}
	located := api.Message{Text: "bad", Location: &api.Location{File: "a.ts", Line: 3}}
	if got := describe(located); got != "a.ts:3: bad" {
		t.Errorf("describe = %q", got)
	}
}

func TestNavigationStartsFromTheEntry(t *testing.T) {
	out, err := Build(Options{Dir: t.TempDir(), Navigation: true, Minify: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("bundles = %v, want only the runtime", bundleNames(out.Files))
	}
	if !strings.Contains(string(out.Files[0].Bytes), "addEventListener") {
		t.Errorf("entry = %s, want the navigation runtime wired", out.Files[0].Bytes)
	}
}

func TestTheEntryPointStaysOutOfTheWatchedTree(t *testing.T) {
	dir := t.TempDir()
	entry, err := writeEntry(Options{
		Dir:        dir,
		Islands:    sources(map[string]string{"Counter": "components/Counter/client.ts"}),
		Navigation: true,
	})
	if err != nil {
		t.Fatalf("writeEntry: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(filepath.Join(dir, workDir))
	}()

	work := filepath.Join(dir, workDir)
	if filepath.Dir(entry) != work {
		t.Errorf("the entry point is at %s, want it inside %s so the dev watcher ignores it", entry, work)
	}
	if _, err := os.Stat(filepath.Join(work, runtimeName)); err != nil {
		t.Errorf("the runtime should sit next to the entry point: %v", err)
	}
	for _, name := range []string{entryName, runtimeName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s was written into the project root", name)
		}
	}
	source, err := os.ReadFile(entry)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if !strings.Contains(string(source), `"../../components/Counter/client.ts"`) {
		t.Errorf("entry = %q, want the island imported relative to the work directory", source)
	}
}

func TestAWorkDirectoryThatCannotBeCreatedIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, workDir)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, workDir), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Build(Options{Dir: dir, Islands: sources(map[string]string{"Counter": "components/Counter/client.ts"})})
	if err == nil {
		t.Fatal("Build should fail when the work directory cannot be created")
	}
}

func TestAnUnwritableEntryPointIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, workDir, entryName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Build(Options{Dir: dir, Islands: sources(map[string]string{"Counter": "components/Counter/client.ts"})})
	if err == nil {
		t.Fatal("Build should fail when the entry point cannot be written")
	}
}

func TestAnInlineIslandIsStagedAndImported(t *testing.T) {
	dir := t.TempDir()
	results, err := Build(Options{
		Dir:     dir,
		Islands: map[string]Island{"Ticker": {Source: "components/Ticker.gopage", Code: "export function mount() { return () => {} }"}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var found bool
	for _, result := range results.Files {
		if strings.Contains(string(result.Bytes), "mount") {
			found = true
		}
	}
	if !found {
		t.Errorf("results = %d, want the island code bundled", len(results.Files))
	}
	staged := filepath.Join(dir, workDir, islandDir, "Ticker.ts")
	if _, err := os.Stat(staged); err != nil {
		t.Errorf("the inline island was not staged: %v", err)
	}
}

func TestThePropsAliasResolvesToAnEmptyModule(t *testing.T) {
	dir := t.TempDir()
	results, err := Build(Options{
		Dir: dir,
		Islands: map[string]Island{"Ticker": {Source: "components/Ticker.gopage", Code: `import type { Props } from "gopage:props/Ticker";
export function mount(element: HTMLElement, props: Props): () => void {
	element.dataset.start = String(props.Start);
	return () => {};
}`}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, result := range results.Files {
		if strings.Contains(string(result.Bytes), "gopage:props") {
			t.Errorf("%s still imports the alias", result.Name)
		}
	}
}

func TestAPropsAliasWithAPathIsRejected(t *testing.T) {
	_, err := Build(Options{
		Dir: t.TempDir(),
		Islands: map[string]Island{"Ticker": {Source: "components/Ticker.gopage", Code: `import "gopage:props/nested/Thing";
export function mount() { return () => {}; }`}},
	})
	if err == nil {
		t.Error("an alias that is not a component name must fail the bundle")
	}
}

func TestAnEmptyProjectBundlesNothing(t *testing.T) {
	results, err := Build(Options{Dir: t.TempDir()})
	if err != nil || results.Files != nil {
		t.Errorf("results = %v, err = %v, want nothing to bundle", results, err)
	}
}

func TestAnEmptyPropsAliasIsRejected(t *testing.T) {
	_, err := Build(Options{
		Dir: t.TempDir(),
		Islands: map[string]Island{"Ticker": {Source: "components/Ticker.gopage", Code: `import "gopage:props/";
export function mount() { return () => {}; }`}},
	})
	if err == nil {
		t.Error("an alias without a component name must fail the bundle")
	}
}

func TestAnInlineIslandThatCannotBeStagedIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, workDir, islandDir, "Ticker.ts"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Build(Options{
		Dir:     dir,
		Islands: map[string]Island{"Ticker": {Source: "components/Ticker.gopage", Code: "export function mount() { return () => {} }"}},
	})
	if err == nil {
		t.Error("an island that cannot be staged must fail the bundle")
	}
}

func TestAValueImportedFromThePropsAliasResolvesToNothing(t *testing.T) {
	results, err := Build(Options{
		Dir: t.TempDir(),
		Islands: map[string]Island{"Ticker": {Source: "components/Ticker.gopage", Code: `import * as props from "gopage:props/Ticker";
export function mount(element: HTMLElement): () => void {
	element.dataset.keys = Object.keys(props).join(",");
	return () => {};
}`}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, result := range results.Files {
		if strings.Contains(string(result.Bytes), `from "gopage:props`) {
			t.Errorf("%s still imports the alias at runtime", result.Name)
		}
	}
}

const fakeReact = `export function createElement(type, props) { return { type, props }; }
export const shared = "react-shared-marker";
`

const fakeReactDom = `import { shared } from "react";
export function createRoot(element) {
	return {
		render(node) { element.dataset.rendered = node.type.name + ":" + shared; },
		unmount() { delete element.dataset.rendered; },
	};
}
`

const reactComponent = `import type { Props } from "gopage:props/Chart";
export default function Chart(props: Props) { return <div className="chart">{props.Points.length}</div>; }
`

func reactProject(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"node_modules/react/package.json":     `{"name":"react","type":"module","exports":{".":"./index.js","./jsx-runtime":"./jsx-runtime.js"}}`,
		"node_modules/react/index.js":         fakeReact,
		"node_modules/react/jsx-runtime.js":   "export function jsx(type, props) { return { type, props }; }\nexport const jsxs = jsx;\n",
		"node_modules/react-dom/package.json": `{"name":"react-dom","type":"module","exports":{"./client":"./client.js"}}`,
		"node_modules/react-dom/client.js":    fakeReactDom,
	}
	for name, body := range extra {
		files[name] = body
	}
	return project(t, files)
}

func TestAReactIslandIsWrappedIntoAMount(t *testing.T) {
	dir := reactProject(t, nil)
	out, err := Build(Options{
		Dir:     dir,
		Islands: map[string]Island{"Chart": {Source: "components/Chart.gopage", Code: reactComponent, Lang: "tsx", React: true}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	joined := ""
	for _, file := range out.Files {
		joined += string(file.Bytes)
	}
	for _, want := range []string{"createRoot", "unmount", "react-shared-marker", `"chart"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("bundle lacks %q", want)
		}
	}
	if strings.Contains(joined, `from "gopage:react"`) {
		t.Error("the adapter alias must be resolved at build time")
	}
	wrapper := filepath.Join(dir, workDir, islandDir, "Chart"+mountSuffix)
	if _, err := os.Stat(wrapper); err != nil {
		t.Errorf("the wrapper was not staged: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, workDir, islandDir, "Chart.tsx")); err != nil {
		t.Errorf("the tsx body was not staged under its own extension: %v", err)
	}
}

func TestADirectoryReactIslandResolvesFromItsOwnFile(t *testing.T) {
	dir := reactProject(t, map[string]string{"components/Chart/client.tsx": reactComponent})
	out, err := Build(Options{
		Dir:     dir,
		Islands: map[string]Island{"Chart": {Source: "components/Chart/client.tsx", Lang: "tsx", React: true}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(out.Files) < 2 {
		t.Errorf("bundles = %v", bundleNames(out.Files))
	}
}

func TestTwoReactIslandsShareOneReactChunk(t *testing.T) {
	dir := reactProject(t, map[string]string{
		"components/A/client.tsx": reactComponent,
		"components/B/client.tsx": reactComponent,
	})
	out, err := Build(Options{Dir: dir, Islands: map[string]Island{
		"A": {Source: "components/A/client.tsx", Lang: "tsx", React: true},
		"B": {Source: "components/B/client.tsx", Lang: "tsx", React: true},
	}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	shared := 0
	for _, file := range out.Files {
		if strings.Contains(string(file.Bytes), "react-shared-marker") {
			shared++
		}
	}
	if shared != 1 {
		t.Errorf("react appears in %d chunks, want exactly one shared chunk: %v", shared, bundleNames(out.Files))
	}
	if out.Metafile == "" {
		t.Error("the metafile must be returned for the size gate")
	}
}

func TestTheReactAdapterMountsAndUnmounts(t *testing.T) {
	if !strings.Contains(reactAdapter, "createRoot(element)") || !strings.Contains(reactAdapter, "root.unmount()") {
		t.Errorf("adapter = %q", reactAdapter)
	}
}

func TestPreactSwapsInThroughAliases(t *testing.T) {
	dir := reactProject(t, map[string]string{
		"node_modules/preact/package.json":   `{"name":"preact","type":"module","exports":{".":"./index.js","./compat":"./compat.js","./jsx-runtime":"./jsx-runtime.js"}}`,
		"node_modules/preact/index.js":       `export const preact = "preact-marker";`,
		"node_modules/preact/compat.js":      "export function createElement(t, p) { return { t, p }; }\nexport function createRoot(el) { return { render() { el.dataset.engine = \"preact-marker\"; }, unmount() {} }; }",
		"node_modules/preact/jsx-runtime.js": "export function jsx(t, p) { return { t, p }; }\nexport const jsxs = jsx;",
		"components/Chart/client.tsx":        reactComponent,
	})
	out, err := Build(Options{
		Dir:     dir,
		Engine:  EnginePreact,
		Islands: map[string]Island{"Chart": {Source: "components/Chart/client.tsx", Lang: "tsx", React: true}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	joined := ""
	for _, file := range out.Files {
		joined += string(file.Bytes)
	}
	if !strings.Contains(joined, "preact-marker") || strings.Contains(joined, "react-shared-marker") {
		t.Error("with the preact engine every react import must land in preact/compat")
	}
}

func TestAMissingReactIsNamedWithTheInstallCommand(t *testing.T) {
	dir := project(t, map[string]string{"components/Chart/client.tsx": reactComponent})
	_, err := Build(Options{
		Dir:     dir,
		Islands: map[string]Island{"Chart": {Source: "components/Chart/client.tsx", Lang: "tsx", React: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "pnpm add react react-dom") {
		t.Errorf("err = %v, want the install command", err)
	}
	_, err = Build(Options{
		Dir:     dir,
		Engine:  EnginePreact,
		Islands: map[string]Island{"Chart": {Source: "components/Chart/client.tsx", Lang: "tsx", React: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "pnpm add preact") {
		t.Errorf("err = %v, want the preact install command", err)
	}
}

func TestChunksTheEntryImportsEagerlyAreListed(t *testing.T) {
	dir := reactProject(t, map[string]string{
		"node_modules/react/package.json":   `{"name":"react","main":"index.js"}`,
		"node_modules/react/index.js":       "module.exports = { createElement(type, props) { return { type, props }; }, shared: 'react-shared-marker' };\n",
		"node_modules/react/jsx-runtime.js": "module.exports = { jsx(type, props) { return { type, props }; }, jsxs(type, props) { return { type, props }; } };\n",
		"components/Chart/client.tsx":       reactComponent,
	})
	out, err := Build(Options{
		Dir:     dir,
		Minify:  true,
		Islands: map[string]Island{"Chart": {Source: "components/Chart/client.tsx", Lang: "tsx", React: true}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var entry string
	for _, file := range out.Files {
		if strings.HasPrefix(file.Name, RuntimePrefix) {
			entry = string(file.Bytes)
		}
	}
	eager := EagerChunks(out)
	for name := range eager {
		if !strings.Contains(entry, `import"/assets/`+name+`"`) {
			t.Errorf("%s is listed as eager but the entry does not import it statically:\n%s", name, entry)
		}
	}
	if strings.Contains(entry, `import"/assets/`) && len(eager) == 0 {
		t.Errorf("the entry imports a chunk statically but none was listed:\n%s", entry)
	}
	if got := EagerChunks(Output{Metafile: "not json"}); len(got) != 0 {
		t.Errorf("eager = %v, want nothing from a broken metafile", got)
	}
}

func TestIslandChunksFollowTheStaticImportsOfEachIsland(t *testing.T) {
	dir := reactProject(t, map[string]string{
		"components/A/client.tsx":    reactComponent,
		"components/B/client.tsx":    reactComponent,
		"components/Plain/client.ts": island,
	})
	islands := map[string]Island{
		"A":     {Source: "components/A/client.tsx", Lang: "tsx", React: true},
		"B":     {Source: "components/B/client.tsx", Lang: "tsx", React: true},
		"Plain": {Source: "components/Plain/client.ts"},
	}
	out, err := Build(Options{Dir: dir, Islands: islands})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	chunks := IslandChunks(out, islands)
	if len(chunks["A"]) != 2 || len(chunks["B"]) != 2 {
		t.Errorf("chunks = %v, want each react island to name its own chunk and the shared react chunk", chunks)
	}
	if len(chunks["Plain"]) != 1 {
		t.Errorf("chunks = %v, want a plain island to name only itself", chunks)
	}
	shared := map[string]int{}
	for _, name := range append(chunks["A"], chunks["B"]...) {
		shared[name]++
	}
	var common int
	for _, count := range shared {
		if count == 2 {
			common++
		}
	}
	if common != 1 {
		t.Errorf("chunks = %v, want exactly one chunk shared by both react islands", chunks)
	}
	if got := IslandChunks(Output{Metafile: "nope"}, islands); got != nil {
		t.Errorf("chunks = %v, want nothing from a broken metafile", got)
	}
}

func TestEveryIslandShapeMapsToItsOwnModule(t *testing.T) {
	cases := map[string]Island{
		".gopage/cache/islands/Chart.mount.ts": {Source: "components/Chart.gopage", Code: "x", Lang: "tsx", React: true},
		".gopage/cache/islands/Chart.tsx":      {Source: "components/Chart.gopage", Code: "x", Lang: "tsx"},
		".gopage/cache/islands/Chart.ts":       {Source: "components/Chart.gopage", Code: "x"},
		"components/Chart/client.ts":           {Source: "components/Chart/client.ts"},
	}
	for want, island := range cases {
		if got := moduleOf(island, "Chart"); got != want {
			t.Errorf("moduleOf(%+v) = %q, want %q", island, got, want)
		}
	}
	meta := metafile{Outputs: map[string]metaOutput{
		"out/a.js": {Imports: []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		}{{Path: "out/b.js", Kind: "import-statement"}, {Path: "out/c.js", Kind: "dynamic-import"}}},
		"out/b.js": {Imports: []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		}{{Path: "out/a.js", Kind: "import-statement"}}},
	}}
	if got := closure(meta, "out/a.js"); len(got) != 2 || got[0] != "a.js" || got[1] != "b.js" {
		t.Errorf("closure = %v, want the static imports once each and no dynamic ones", got)
	}
	out, _ := Build(Options{Dir: t.TempDir(), Navigation: true})
	if got := IslandChunks(out, map[string]Island{"Missing": {Source: "components/Missing/client.ts"}}); len(got) != 0 {
		t.Errorf("chunks = %v, want an island that is not in the bundle skipped", got)
	}
}

func denyWrites(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("windows leaves the owner able to write into a read-only directory")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skipf("cannot make %s read-only: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}
