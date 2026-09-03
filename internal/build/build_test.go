package build

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/apptivitypl/rill/internal/demo"
	"github.com/apptivitypl/rill/internal/gotoolchain"
	"github.com/apptivitypl/rill/internal/paths"
)

type recorder struct {
	commands []Command
	fail     error
}

func (r *recorder) Run(command Command) error {
	r.commands = append(r.commands, command)
	return r.fail
}

func (r *recorder) names() []string {
	out := make([]string, len(r.commands))
	for i, command := range r.commands {
		out[i] = command.Name + " " + strings.Join(command.Args, " ")
	}
	return out
}

func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func helloWorld() map[string]string {
	return map[string]string{
		"app/layout.rill":     "<html><body>{% outlet %}</body></html>",
		"app/page.rill":       "<h1>hello</h1>",
		"app/about/page.rill": "<h1>about</h1>",
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestTargetsAreListed(t *testing.T) {
	if got := Targets(); !slices.Contains(got, "workers") || !slices.Contains(got, "native") {
		t.Errorf("Targets() = %v", got)
	}
}

func TestBuildWritesTheManifest(t *testing.T) {
	dir := project(t, helloWorld())
	report, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.ManifestSize == 0 {
		t.Error("the manifest is empty")
	}
	if len(read(t, dir, paths.Manifest)) != report.ManifestSize {
		t.Error("the manifest on disk does not match the report")
	}
}

func TestBuildRendersStaticPages(t *testing.T) {
	dir := project(t, helloWorld())
	report, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.StaticPages) != 2 {
		t.Errorf("static pages = %v, want the root and about", report.StaticPages)
	}
	if got := read(t, dir, "dist/assets/index.html"); got != "<html><body><h1>hello</h1></body></html>" {
		t.Errorf("index.html = %q", got)
	}
	if got := read(t, dir, "dist/assets/about/index.html"); !strings.Contains(got, "about") {
		t.Errorf("about/index.html = %q", got)
	}
}

func TestDynamicRoutesAreNotPrerendered(t *testing.T) {
	files := helloWorld()
	files["app/listings/[id]/page.rill"] = "<p>detail</p>"
	dir := project(t, files)

	report, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, page := range report.StaticPages {
		if strings.Contains(page, "listings") {
			t.Errorf("a dynamic route was prerendered: %q", page)
		}
	}
}

func TestNativeTargetBuildsTheServerBinary(t *testing.T) {
	runner := &recorder{}
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetNative, Runner: runner}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "go build -o " + paths.Server() + " ./cmd/server"
	if got := runner.names(); len(got) != 1 || got[0] != want {
		t.Errorf("commands = %v, want %q", got, want)
	}
}

func TestWorkersTargetGeneratesAssetsAndWasm(t *testing.T) {
	runner := &recorder{}
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, Runner: runner}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %v, want the shim generator and the wasm build", runner.names())
	}
	if !strings.Contains(runner.names()[0], "-mode=go") {
		t.Errorf("the shim must be generated in go mode, got %q", runner.names()[0])
	}
	env := runner.commands[1].Env
	if !slices.Contains(env, "GOOS=js") || !slices.Contains(env, "GOARCH=wasm") {
		t.Errorf("wasm build env = %v", env)
	}
}

func TestWorkersTargetWritesWranglerConfig(t *testing.T) {
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, Name: "demo", Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var config wrangler
	if err := json.Unmarshal([]byte(read(t, dir, paths.Wrangler)), &config); err != nil {
		t.Fatalf("wrangler config is not valid JSON: %v", err)
	}
	if config.Name != "demo" {
		t.Errorf("name = %q", config.Name)
	}
	if config.Assets.Binding != "ASSETS" || config.Assets.Directory != "./"+paths.AssetsDir {
		t.Errorf("assets = %+v", config.Assets)
	}
	if !slices.Contains(config.RunWorkerFirst, "/api/*") {
		t.Errorf("run_worker_first = %v, want the api namespace", config.RunWorkerFirst)
	}
	if config.CompatibilityDate == "" {
		t.Error("a compatibility date is required")
	}
}

func TestCompatibilityDateCanBePinned(t *testing.T) {
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, CompatDate: "2026-02-03", Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(read(t, dir, paths.Wrangler), "2026-02-03") {
		t.Error("the pinned compatibility date was ignored")
	}
}

func TestNameDefaultsToTheDirectory(t *testing.T) {
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(read(t, dir, paths.Wrangler), filepath.Base(dir)) {
		t.Error("the worker name should fall back to the project directory")
	}
}

func TestBuildStopsOnDiagnostics(t *testing.T) {
	dir := project(t, map[string]string{"app/page.rill": "<p>{{ "})
	_, err := Run(Options{Dir: dir, Runner: &recorder{}})

	var buildErr *Error
	if !errors.As(err, &buildErr) {
		t.Fatalf("err = %v, want a build error carrying diagnostics", err)
	}
	if len(buildErr.Diagnostics) == 0 {
		t.Error("the build error carries no diagnostics")
	}
	rendered := buildErr.Render()
	if !strings.Contains(rendered, "RILL-C002") || !strings.Contains(rendered, "app/page.rill") {
		t.Errorf("rendered diagnostics =\n%s", rendered)
	}
}

func TestBuildDoesNotRunToolsAfterADiagnostic(t *testing.T) {
	runner := &recorder{}
	dir := project(t, map[string]string{"app/page.rill": "{% nope %}"})
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, Runner: runner}); err == nil {
		t.Fatal("expected the build to stop")
	}
	if len(runner.commands) != 0 {
		t.Errorf("commands = %v, want none", runner.names())
	}
}

func TestRunnerFailureIsReported(t *testing.T) {
	dir := project(t, helloWorld())
	runner := &recorder{fail: os.ErrPermission}
	if _, err := Run(Options{Dir: dir, Target: TargetNative, Runner: runner}); err == nil {
		t.Error("a failing toolchain command must fail the build")
	}
}

func TestAssetPathLayout(t *testing.T) {
	cases := map[string]string{
		"/":           "index.html",
		"/about":      filepath.Join("about", "index.html"),
		"/docs/intro": filepath.Join("docs", "intro", "index.html"),
	}
	for pattern, want := range cases {
		if got := assetPath(pattern); got != want {
			t.Errorf("assetPath(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func unmarshal(text string, out *wrangler) error {
	return json.Unmarshal([]byte(text), out)
}

func TestARelativeDirectoryBuildsTheSameWayAsAnAbsoluteOne(t *testing.T) {
	dir := project(t, helloWorld())
	parent, base := filepath.Split(strings.TrimSuffix(dir, string(filepath.Separator)))
	restore, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(parent); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(restore) }()

	if _, err := Run(Options{Dir: base, Target: TargetWorkers, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(read(t, dir, paths.Wrangler), base) {
		t.Error("the worker name should come from the resolved directory")
	}
}

func TestTheCurrentDirectoryIsNamedAfterItself(t *testing.T) {
	dir := project(t, helloWorld())
	restore, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(restore) }()

	if _, err := Run(Options{Dir: ".", Target: TargetWorkers, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	config := read(t, dir, paths.Wrangler)
	if strings.Contains(config, `"name": "."`) {
		t.Errorf("the worker name is the literal dot: %s", config)
	}
}

func TestABuildDoesNotTouchTheSourcesItReads(t *testing.T) {
	sources := helloWorld()
	sources["components/Counter/props.go"] = "package counter\n\ntype Props struct {\n\tStart int\n}\n"
	sources["components/Counter/template.rill"] = "<div>{{ Start }}</div>\n"
	sources["components/Counter/client.ts"] = "export function mount() { return () => {} }\n"
	sources["app/page.rill"] = `<h1>hello</h1><Counter client="load" :Start="1" />`
	dir := project(t, sources)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	before := watchedTimes(t, dir)
	if len(before) < len(sources) {
		t.Fatalf("only %d watched files, want at least %d", len(before), len(sources))
	}

	time.Sleep(10 * time.Millisecond)
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for path, stamp := range watchedTimes(t, dir) {
		if !before[path].Equal(stamp) {
			t.Errorf("the build rewrote %s, which makes the dev watcher loop", path)
		}
	}
}

func watchedTimes(t *testing.T, dir string) map[string]time.Time {
	t.Helper()
	stamps := map[string]time.Time{}
	for _, watched := range []string{"app", "components", "locales", "styles"} {
		root := filepath.Join(dir, watched)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			stamps[path] = info.ModTime()
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", watched, err)
		}
	}
	return stamps
}

func TestTidyResolvesTheDependenciesOfANewProject(t *testing.T) {
	runner := &recorder{}
	if err := Tidy("/tmp/demo", gotoolchain.Resolved{}, runner); err != nil {
		t.Fatalf("Tidy: %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %v, want one", runner.commands)
	}
	command := runner.commands[0]
	if command.Dir != "/tmp/demo" || command.Name != "go" || strings.Join(command.Args, " ") != "mod tidy" {
		t.Errorf("command = %+v, want go mod tidy in the project", command)
	}
}

func TestTidyReportsAFailingToolchain(t *testing.T) {
	if err := Tidy("/tmp/demo", gotoolchain.Resolved{}, &recorder{fail: errors.New("no network")}); err == nil {
		t.Error("Tidy should report a toolchain failure")
	}
}

func TestThePackageManagerIsTheFirstOneOnThePath(t *testing.T) {
	only := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, known := range names {
				if known == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	if got := PackageManager(only("npm", "pnpm")); got != "pnpm" {
		t.Errorf("manager = %q, want pnpm preferred", got)
	}
	if got := PackageManager(only("bun")); got != "bun" {
		t.Errorf("manager = %q", got)
	}
	if got := PackageManager(only()); got != "" {
		t.Errorf("manager = %q, want none", got)
	}
}

func TestInstallRunsTheManagerInTheProject(t *testing.T) {
	runner := &recorder{}
	if err := Install("/tmp/demo", "npm", runner); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(runner.commands) != 1 || runner.commands[0].Name != "npm" || runner.commands[0].Dir != "/tmp/demo" {
		t.Errorf("commands = %+v", runner.commands)
	}
	if got := strings.Join(runner.commands[0].Args, " "); got != "install --no-fund --no-audit" {
		t.Errorf("args = %q", got)
	}
	if err := Install("/tmp/demo", "yarn", runner); err == nil {
		t.Error("an unknown manager must be refused, not guessed")
	}
}

func TestAColouredRunnerAsksTheChildForColour(t *testing.T) {
	plain := ExecRunner{}
	if plain.Color {
		t.Error("colour is off unless the caller asks")
	}
	cmd := Command{Dir: t.TempDir(), Name: "go", Args: []string{"version"}}
	if err := (ExecRunner{Color: true, Out: io.Discard}).Run(cmd); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDemoTargetGeneratesTheBrowserShimAndWasm(t *testing.T) {
	runner := &recorder{}
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetDemo, Runner: runner}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %v, want the shim generator and the wasm build", runner.names())
	}
	if !strings.Contains(runner.names()[0], "-runtime=browser") {
		t.Errorf("the shim must be generated for the browser runtime, got %q", runner.names()[0])
	}
	env := runner.commands[1].Env
	if !slices.Contains(env, "GOOS=js") || !slices.Contains(env, "GOARCH=wasm") {
		t.Errorf("wasm build env = %v", env)
	}
	if !strings.Contains(runner.names()[1], paths.DemoBinary) {
		t.Errorf("the wasm build must land in %s, got %q", paths.DemoBinary, runner.names()[1])
	}
}

func TestDemoTargetWritesAFolderNodeCanRun(t *testing.T) {
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetDemo, Name: "shop", Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, name := range []string{"server.mjs", "serve.mjs", "runtime.mjs", "package.json"} {
		if read(t, dir, paths.DemoDir+"/"+name) == "" {
			t.Errorf("%s is missing or empty", name)
		}
	}
	var meta demo.Meta
	if err := json.Unmarshal([]byte(read(t, dir, paths.DemoDir+"/"+demo.MetaFile)), &meta); err != nil {
		t.Fatalf("%s is not valid JSON: %v", demo.MetaFile, err)
	}
	if meta.Name != "shop" {
		t.Errorf("name = %q", meta.Name)
	}
	if !slices.Contains(meta.WorkerFirst, "/api/*") {
		t.Errorf("workerFirst = %v, want the api namespace", meta.WorkerFirst)
	}
}

func TestDemoTargetCarriesTheAssetsWithIt(t *testing.T) {
	dir := project(t, helloWorld())
	if _, err := Run(Options{Dir: dir, Target: TargetDemo, Runner: &recorder{}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if read(t, dir, paths.DemoAssets+"/about/index.html") == "" {
		t.Error("a static page did not travel into the demo folder")
	}
}

func TestCopyingReportsATreeThatIsNotThere(t *testing.T) {
	if err := copyTree(filepath.Join(t.TempDir(), "gone"), t.TempDir()); err == nil {
		t.Error("copyTree should report a source it cannot walk")
	}
}

func TestDemoTargetStopsWhenAStepFails(t *testing.T) {
	dir := project(t, helloWorld())
	runner := &recorder{fail: errors.New("no toolchain")}
	if _, err := Run(Options{Dir: dir, Target: TargetDemo, Runner: runner}); err == nil {
		t.Error("Run should report the step that failed")
	}
}

func TestDemoTargetReportsAFolderItCannotWrite(t *testing.T) {
	dir := project(t, helloWorld())
	taken := filepath.Join(dir, filepath.FromSlash(paths.DemoDir), demo.Entry)
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := Run(Options{Dir: dir, Target: TargetDemo, Runner: &recorder{}}); err == nil {
		t.Error("Run should report the demo folder it could not write")
	}
}
