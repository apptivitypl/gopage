package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/apptivitypl/gopage/internal/build"
	"github.com/apptivitypl/gopage/internal/compile"
	"github.com/apptivitypl/gopage/internal/css"
	"github.com/apptivitypl/gopage/internal/demo"
	"github.com/apptivitypl/gopage/internal/devserver"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/fetch"
	"github.com/apptivitypl/gopage/internal/gotoolchain"
	"github.com/apptivitypl/gopage/internal/initui"
	"github.com/apptivitypl/gopage/internal/lsp"
	"github.com/apptivitypl/gopage/internal/paths"
	"github.com/apptivitypl/gopage/internal/scaffold"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var buildErr *build.Error
		if errors.As(err, &buildErr) {
			fmt.Fprint(os.Stderr, buildErr.Render())
		}
		console(os.Stderr).failure(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fail("gopage needs a command").
			showing(usage()).
			try("gopage new my-site --module example.com/my-site", "gopage dev")
	}
	switch args[0] {
	case "new":
		return newProject(args[1:])
	case "build":
		return buildProject(args[1:])
	case "dev":
		return dev(args[1:])
	case "routes":
		return routes(args[1:])
	case "i18n":
		return i18nCommand(args[1:])
	case "check":
		return check(args[1:])
	case "css":
		return cssCommand(args[1:])
	case "lsp":
		return lspCommand(args[1:])
	case "help", "-h", "--help":
		_, _ = fmt.Fprintln(os.Stdout, usage())
		return nil
	case "version", "-v", "--version":
		_, _ = fmt.Fprintln(os.Stdout, release())
		return nil
	default:
		return fail("there is no %q command", args[0]).
			showing(usage()).
			try(nearest(args[0])...)
	}
}

func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

type command struct {
	name  string
	args  string
	about string
}

func commands() []command {
	return []command{
		{"new", "<dir>", "write a project and resolve its dependencies"},
		{"dev", "", "serve the project, rebuilding as it changes"},
		{"build", "", "compile the project for a deployment target"},
		{"check", "", "compile the project without writing anything"},
		{"routes", "", "list the routes the compiler found"},
		{"i18n", "<action>", "work with the message catalogs"},
		{"css", "install", "fetch the tailwind binary this project pins"},
		{"lsp", "", "run the language server on stdin and stdout"},
		{"version", "", "print the version this binary was built from"},
	}
}

func usage() string {
	out := console(os.Stderr)
	var b strings.Builder
	fmt.Fprintf(&b, "  %s %s\n\n", out.paint(bold, "gopage"), out.paint(dim, "a web framework in go"))
	fmt.Fprintf(&b, "  %s\n    gopage <command> [flags]\n\n", out.paint(dim, "usage"))
	fmt.Fprintf(&b, "  %s\n", out.paint(dim, "commands"))
	for _, c := range commands() {
		fmt.Fprintf(&b, "    %s %s  %s\n",
			out.paint(cyan, padTo(c.name, 7)), padTo(c.args, 8), out.paint(dim, c.about))
	}
	fmt.Fprintf(&b, "\n  %s\n    %s\n", out.paint(dim, "templates"), strings.Join(scaffold.Names(), ", "))
	return strings.TrimRight(b.String(), "\n")
}

func padTo(text string, width int) string {
	for len(text) < width {
		text += " "
	}
	return text
}

func cssCommand(args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return fail("css has one subcommand, and it is install").
			try("gopage css install")
	}
	path, err := css.Tailwind{Fetch: fetch.Download}.Install()
	if err != nil {
		return err
	}
	sum, err := fetch.Checksum(path)
	if err != nil {
		return err
	}
	fmt.Printf("tailwind %s is at %s\nsha256: %s\n", css.Version, path, sum)
	return nil
}

func newProject(args []string) error {
	var cfg scaffold.Config
	var yes bool
	var locales string
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.StringVar(&cfg.Module, "module", "", "go module path for the new project")
	fs.StringVar(&cfg.Template, "template", "hello-world", "project template")
	fs.StringVar(&cfg.Name, "name", "", "project name, defaults to the directory")
	fs.StringVar(&cfg.GopagePath, "gopage-path", "", "local path to the gopage checkout, adds a replace directive")
	fs.StringVar(&cfg.GopageVersion, "gopage-version", "", "version of gopage the project requires, defaults to this binary's")
	fs.StringVar(&cfg.CompatDate, "compat-date", "", "cloudflare compatibility date")
	fs.StringVar(&locales, "locales", "", "comma separated languages, the first one is the default")
	fs.StringVar(&cfg.Nav, "nav", "", "navigation mode: partial or off")
	fs.StringVar(&cfg.CSS, "css", "", "css engine: plain or tailwind")
	fs.StringVar(&cfg.Theme, "theme", "", "theme: "+strings.Join(scaffold.Themes(), ", "))
	fs.StringVar(&cfg.React, "react", "", "react in the browser: "+strings.Join(scaffold.Reacts(), ", "))
	fs.BoolVar(&yes, "yes", false, "accept the defaults instead of prompting")
	var skipTidy bool
	fs.BoolVar(&skipTidy, "no-tidy", false, "skip go mod tidy, leaving the dependencies unresolved")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if cfg.GopageVersion == "" {
		cfg.GopageVersion = ownVersion()
	}
	if len(positional) != 1 {
		return fail("gopage new takes one directory, and was given %d", len(positional)).
			because("The directory is where the project is written; every other setting is a flag.").
			try("gopage new my-site --module example.com/my-site")
	}
	cfg.Dir = positional[0]
	if cfg.Name == "" {
		cfg.Name = scaffold.DefaultName(cfg.Dir)
	}
	cfg.Locales = scaffold.SplitLocales(locales)
	if len(cfg.Locales) > 0 {
		cfg.DefaultLocale = cfg.Locales[0]
	}
	if !yes {
		answered, err := initui.Run(cfg, os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		cfg = answered
	}
	out := console(os.Stderr)
	if cfg.GopageVersion == "" && cfg.GopagePath == "" {
		out.note("this build carries no released version, so the project will require " +
			scaffold.DefaultGopageVersion + ", which no proxy serves; pass --gopage-version or --gopage-path")
	}
	if err := scaffold.Create(cfg); err != nil {
		if errors.Is(err, scaffold.ErrNotEmpty) {
			return fail("%s already holds files", cfg.Dir).
				because("gopage new writes a whole project tree and will not merge into a directory that is already in use, so nothing here is overwritten.").
				try("gopage new "+cfg.Dir+"-2 --module "+moduleOrPlaceholder(cfg), "rm -rf "+cfg.Dir+" && gopage new "+cfg.Dir+" --module "+moduleOrPlaceholder(cfg))
		}
		return err
	}
	out.step("created", cfg.Dir+"  from the "+cfg.Template+" template")
	if err := build.Bootstrap(cfg.Dir); err != nil {
		return err
	}
	if !skipTidy {
		tool, err := gotool(out)
		if err != nil {
			return err
		}
		if err := build.Tidy(cfg.Dir, tool, build.ExecRunner{Out: out.tagged("go")}); err != nil {
			return fmt.Errorf("go mod tidy in %s: %w", cfg.Dir, err)
		}
		out.step("resolved", "the go module dependencies")
		if err := installPackages(cfg, out); err != nil {
			return err
		}
	}
	out.hint("cd " + cfg.Dir + " && gopage dev")
	return nil
}

func installPackages(cfg scaffold.Config, out *printer) error {
	if _, err := os.Stat(filepath.Join(cfg.Dir, "package.json")); err != nil || !cfg.UsesReact() {
		return nil
	}
	manager := build.PackageManager(exec.LookPath)
	if manager == "" {
		out.note("no package manager on PATH; run npm install in " + cfg.Dir + " before the first build")
		return nil
	}
	runner := build.ExecRunner{Out: out.tagged(manager), Color: out.color}
	if err := build.Install(cfg.Dir, manager, runner); err != nil {
		return fmt.Errorf("%s install in %s: %w", manager, cfg.Dir, err)
	}
	out.step("installed", "the browser packages with "+manager)
	return nil
}

type buildFlags struct {
	dir        string
	target     string
	name       string
	compatDate string
	verbose    bool
}

func parseBuildFlags(name string, args []string) (buildFlags, error) {
	var flags buildFlags
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&flags.dir, "dir", ".", "project directory")
	fs.StringVar(&flags.target, "target", string(build.TargetNative), "build target: "+strings.Join(build.Targets(), ", "))
	fs.StringVar(&flags.name, "name", "", "worker name, defaults to the directory")
	fs.StringVar(&flags.compatDate, "compat-date", "", "cloudflare compatibility date")
	fs.BoolVar(&flags.verbose, "v", false, "echo the toolchain commands")
	if err := fs.Parse(args); err != nil {
		return flags, err
	}
	return flags, nil
}

func buildProject(args []string) error {
	flags, err := parseBuildFlags("build", args)
	if err != nil {
		return err
	}
	target := build.Target(flags.target)
	if !slices.Contains(build.Targets(), flags.target) {
		return fail("there is no %q target", flags.target).
			because("A build targets one of: %s.", strings.Join(build.Targets(), ", ")).
			try("gopage build --target native", "gopage build --target workers", "gopage build --target demo")
	}
	tool, err := gotool(console(os.Stderr))
	if err != nil {
		return err
	}
	report, err := build.Run(build.Options{
		Dir:        flags.dir,
		Target:     target,
		Name:       flags.name,
		CompatDate: flags.compatDate,
		Runner:     build.ExecRunner{Verbose: flags.verbose},
		Go:         tool,
	})
	if err != nil {
		return err
	}
	printWarnings(flags.dir, report.Diagnostics)
	fmt.Printf("manifest: %d bytes, %d routes\n", report.ManifestSize, len(report.Routes))
	fmt.Printf("static pages: %d\n", len(report.StaticPages))
	for _, page := range report.StaticPages {
		fmt.Println("  ", filepath.ToSlash(page))
	}
	switch target {
	case build.TargetWorkers:
		fmt.Println("wrangler config:", paths.Wrangler)
	case build.TargetDemo:
		fmt.Printf("demo: %s, start it with `node %s/%s`\n", paths.DemoDir, paths.DemoDir, demo.Entry)
	}
	return nil
}

func printWarnings(dir string, diagnostics []diag.Diagnostic) {
	sources := readSources(dir, diagnostics)
	for _, item := range diagnostics {
		if item.Severity != diag.Warning {
			continue
		}
		fmt.Fprint(os.Stderr, diag.Render(item, sources[item.File]))
	}
}

func dev(args []string) error {
	var dir, addr string
	var quiet, host bool
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	fs.StringVar(&dir, "dir", ".", "project directory")
	fs.StringVar(&addr, "addr", "", "listen address, a free port on localhost by default")
	fs.BoolVar(&host, "host", false, "answer on every interface, not only localhost")
	fs.BoolVar(&quiet, "quiet", false, "banner without the tagline and the route line")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out := console(os.Stderr)
	started := time.Now()

	about := &summary{quiet: quiet, width: terminalWidth()}
	rebuild, stop := devBuild(dir, out, about)
	defer stop()
	server := devserver.New(rebuild, func(format string, args ...any) {
		out.broken(fmt.Sprintf(format, args...))
	})
	if !server.Rebuild() {
		out.note("the last good pages stay up behind the overlay")
	}

	listener, err := devserver.Listen(addr, host)
	if err != nil {
		return err
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		_ = listener.Close()
		stop()
		os.Exit(0)
	}()

	changed := make(chan string, 1)
	done := make(chan struct{})
	defer close(done)
	if err := devserver.Watch(dir, changed, done); err != nil {
		return err
	}
	go func() {
		for name := range changed {
			at := time.Now()
			if server.Rebuild() {
				out.rebuilt(name, time.Since(at))
				continue
			}
			out.note("the last good pages stay up behind the overlay")
		}
	}()

	local, network := devserver.Addresses(listener)
	out.banner(local, network, time.Since(started), *about)
	return devserver.HTTP(server).Serve(listener)
}

func devBuild(dir string, out *printer, about *summary) (devserver.Build, func()) {
	var running *devserver.App
	tool, resolving := gotool(out)
	build := func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		if resolving != nil {
			return nil, nil, nil, resolving
		}
		report, err := build.Run(build.Options{
			Dir:    dir,
			Target: build.TargetNative,
			Runner: build.ExecRunner{},
			Go:     tool,
		})
		var failed *build.Error
		if errors.As(err, &failed) {
			return nil, failed.Diagnostics, failed.Sources, err
		}
		if err != nil {
			return nil, nil, nil, err
		}
		about.pages, about.api = patterns(report)
		about.islands = report.Islands
		next, err := devserver.Start(devserver.Launch{
			Dir:    report.Dir,
			Binary: filepath.Join(report.Dir, filepath.FromSlash(paths.Server())),
			Output: out.child(),
		})
		if err != nil {
			return nil, report.Diagnostics, nil, err
		}
		running.Stop()
		running = next
		return next.Handler(), report.Diagnostics, nil, nil
	}
	return build, func() { running.Stop() }
}

func lspCommand(args []string) error {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return lsp.NewServer(os.Stdin, os.Stdout).Serve()
}

func check(args []string) error {
	var dir string
	var strict bool
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.StringVar(&dir, "dir", ".", "project directory")
	fs.BoolVar(&strict, "strict", false, "treat warnings as failures")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var bag diag.Bag
	if _, err := compile.Compile(os.DirFS(dir), &bag); err != nil {
		return err
	}
	items := bag.Sorted()
	sources := readSources(dir, items)
	for _, item := range items {
		fmt.Fprint(os.Stderr, diag.Render(item, sources[item.File]))
	}
	errors, warnings := count(items)
	fmt.Printf("check: %d errors, %d warnings\n", errors, warnings)
	if errors > 0 {
		return fail("the project does not compile").
			because("Every diagnostic above has a page at docs/errors, named by its code.")
	}
	if strict && warnings > 0 {
		return fmt.Errorf("%d warnings with --strict", warnings)
	}
	return nil
}

func count(items []diag.Diagnostic) (int, int) {
	var errors, warnings int
	for _, item := range items {
		if item.Severity == diag.Error {
			errors++
			continue
		}
		warnings++
	}
	return errors, warnings
}

func routes(args []string) error {
	var dir string
	var coverage bool
	fs := flag.NewFlagSet("routes", flag.ContinueOnError)
	fs.StringVar(&dir, "dir", ".", "project directory")
	fs.BoolVar(&coverage, "coverage", false, "show which routes have a loader, meta and a submit handler")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if coverage {
		return routeCoverage(dir)
	}
	discovered, err := discover(dir)
	if err != nil {
		return err
	}
	for _, route := range discovered {
		kind := "page"
		if route.Kind == compile.RouteAPI {
			kind = "api"
		}
		fmt.Printf("%-6s %-28s %-20s %s\n", kind, route.Pattern, route.Name, route.File)
	}
	return nil
}

func routeCoverage(dir string) error {
	var bag diag.Bag
	result, err := compile.Compile(os.DirFS(dir), &bag)
	if err != nil {
		return err
	}
	if bag.HasErrors() {
		return &build.Error{Diagnostics: bag.Sorted(), Sources: readSources(dir, bag.Sorted())}
	}
	fmt.Printf("%-6s %-28s %-8s %-6s %-6s %s\n", "kind", "pattern", "class", "load", "meta", "submit")
	for _, route := range result.Routes {
		template := result.Templates[route.File]
		kind := "page"
		if route.Kind == compile.RouteAPI {
			kind = "api"
		}
		fmt.Printf("%-6s %-28s %-8s %-6s %-6s %s\n", kind, route.Pattern, classOf(result, route),
			mark(template.HasLoader()), mark(template.HasMeta()), mark(template.HasSubmit()))
	}
	return nil
}

func classOf(result compile.Result, route compile.Route) string {
	for _, entry := range result.Manifest.Routes {
		if entry.Name == route.Name {
			return entry.Class.String()
		}
	}
	return "-"
}

func mark(present bool) string {
	if present {
		return "yes"
	}
	return "-"
}

func discover(dir string) ([]compile.Route, error) {
	var bag diag.Bag
	discovered := compile.Discover(os.DirFS(dir), &bag)
	if bag.HasErrors() {
		return nil, &build.Error{Diagnostics: bag.Sorted()}
	}
	return discovered, nil
}

func readSources(dir string, diagnostics []diag.Diagnostic) map[string]string {
	sources := map[string]string{}
	for _, d := range diagnostics {
		if _, done := sources[d.File]; done {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(d.File))); err == nil {
			sources[d.File] = string(data)
		}
	}
	return sources
}

func patterns(report build.Report) (pages, api []string) {
	for _, route := range report.Routes {
		if route.Kind == compile.RouteAPI || strings.HasPrefix(route.Pattern, compile.APIPrefix) {
			api = append(api, route.Pattern)
			continue
		}
		pages = append(pages, route.Pattern)
	}
	return pages, api
}

func terminalWidth() int {
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}
	if width, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && width > 0 {
		return width
	}
	return 0
}

func moduleOrPlaceholder(cfg scaffold.Config) string {
	if cfg.Module != "" {
		return cfg.Module
	}
	return "example.com/" + scaffold.DefaultName(cfg.Dir)
}

func nearest(typed string) []string {
	var close []string
	for _, known := range commands() {
		if within(typed, known.name) {
			close = append(close, "gopage "+known.name)
		}
	}
	if len(close) == 0 {
		return []string{"gopage new my-site --module example.com/my-site", "gopage dev"}
	}
	return close
}

func within(typed, known string) bool {
	if strings.HasPrefix(known, typed) || strings.HasPrefix(typed, known) {
		return true
	}
	return len(typed) > 2 && sorted(typed) == sorted(known)
}

func sorted(text string) string {
	letters := strings.Split(text, "")
	sort.Strings(letters)
	return strings.Join(letters, "")
}

func gotool(out *printer) (gotoolchain.Resolved, error) {
	return gotoolchain.Toolchain{
		Fetch:    fetch.Download,
		Unpack:   fetch.Unpack,
		Announce: out.note,
	}.Resolve()
}
