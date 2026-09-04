package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	appconfig "github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/tool/benchlog"
	"github.com/apptivitypl/gopage/internal/tool/config"
	"github.com/apptivitypl/gopage/internal/tool/covprofile"
	"github.com/apptivitypl/gopage/internal/tool/diagcheck"
	"github.com/apptivitypl/gopage/internal/tool/gate"
	"github.com/apptivitypl/gopage/internal/tool/gitdiff"
	"github.com/apptivitypl/gopage/internal/tool/render"
	"github.com/apptivitypl/gopage/internal/tool/schemacheck"
	"github.com/apptivitypl/gopage/internal/tool/shell"
)

const (
	coverageDir = ".coverage"
	profileName = "cover.out"
	htmlName    = "coverage.html"
	configName  = config.FileName
	lockName    = "dev.lock.json"
)

type devLock struct {
	Coverage gate.Lock       `json:"coverage"`
	Bench    benchlog.Record `json:"bench"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gopagetool:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "coverage":
		return coverage(args[1:])
	case "ci":
		return ci()
	case "diag":
		return diag()
	case "schema":
		return schemaCmd()
	case "bench":
		return bench(args[1:])
	case "smoke":
		return smokeCmd(args[1:])
	case "release":
		return releaseCmd(args[1:])
	case "example":
		return exampleCmd(args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], commandList())
	}
}

func usage() error {
	return errors.New("missing command\n\n" + commandList())
}

func commandList() string {
	return strings.Join([]string{
		"commands:",
		"  coverage [--ci] [--diff-base REF] [--update-lock] [--accept-drop] [--no-run]",
		"  ci",
		"  diag",
		"  schema",
		"  bench [--check] [--record] [--accept]",
		"  smoke [--keep]",
		"  release plan --version V [--json] | run --version V [PACKAGE] [--from DIR] [--publish] | trust | tags --version V",
		"  example [--update] [--workspace] [--verify]",
	}, "\n")
}

type coverageOptions struct {
	enforce    bool
	diffBase   string
	updateLock bool
	acceptDrop bool
	noRun      bool
}

func coverage(args []string) error {
	var opts coverageOptions
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	fs.BoolVar(&opts.enforce, "ci", false, "exit with a non-zero status when a gate fails")
	fs.StringVar(&opts.diffBase, "diff-base", "", "reference to measure changed lines against")
	fs.BoolVar(&opts.updateLock, "update-lock", false, "raise the ratchet after an improvement")
	fs.BoolVar(&opts.acceptDrop, "accept-drop", false, "write the current numbers even where they dropped")
	fs.BoolVar(&opts.noRun, "no-run", false, "reuse the profile already in .coverage")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runCoverage(opts)
}

func runCoverage(opts coverageOptions) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	modulePath, err := modulePath(root)
	if err != nil {
		return err
	}

	profilePath := filepath.Join(root, coverageDir, profileName)
	if !opts.noRun {
		if err := measure(root, profilePath); err != nil {
			return err
		}
	}

	profile, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("read the coverage profile: %w", err)
	}
	blocks, err := covprofile.Parse(string(profile))
	if err != nil {
		return err
	}
	report := covprofile.Aggregate(blocks, modulePath, cfg)

	lockPath := filepath.Join(root, cfg.Lock)
	lock, err := readLock(lockPath)
	if err != nil {
		return err
	}
	outcome := gate.Evaluate(report, cfg, lock)

	if opts.diffBase != "" {
		failure, err := diffGate(report, cfg, opts.diffBase)
		if err != nil {
			return err
		}
		if failure != "" {
			outcome.Failures = append(outcome.Failures, failure)
		}
	}

	printReport(outcome, report)
	if err := writeStepSummary(render.SummaryMarkdown(outcome, report)); err != nil {
		return err
	}

	if opts.updateLock || opts.acceptDrop {
		next := gate.Raise(lock, report)
		if opts.acceptDrop {
			next = gate.Reset(report)
		}
		if err := writeLock(lockPath, next); err != nil {
			return err
		}
		fmt.Println("ratchet updated:", lockPath)
	}

	if !outcome.Passed() {
		fmt.Println("\nviolations:")
		for _, failure := range outcome.Failures {
			fmt.Println("  -", failure)
		}
		if opts.enforce {
			return fmt.Errorf("the coverage gate rejected this change (%d violations)", len(outcome.Failures))
		}
		return nil
	}
	fmt.Println("\ncoverage gate: ok")
	return nil
}

func measure(root, profilePath string) error {
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		return err
	}
	if _, err := shell.Run("go", "test", "./...", "-covermode=atomic", "-coverprofile="+profilePath); err != nil {
		return err
	}
	_, err := shell.Run("go", "tool", "cover", "-html="+profilePath, "-o="+filepath.Join(root, coverageDir, htmlName))
	return err
}

func diffGate(report covprofile.Report, cfg *config.Config, base string) (string, error) {
	diff, err := shell.Capture("git", "diff", "-U0", base+"...HEAD", "--", "*.go")
	if err != nil {
		return "", err
	}
	covered, total := gitdiff.Coverage(report.Blocks, gitdiff.ParseUnified(diff))
	fmt.Printf("changed measurable lines: %d/%d\n", covered, total)
	failure, _ := gate.CheckDiff(covered, total, cfg.DiffStatements)
	return failure, nil
}

func printReport(outcome gate.Outcome, report covprofile.Report) {
	fmt.Print(render.Table(outcome))
	fmt.Printf("\noverall: %.2f%% of statements (%d/%d)\n",
		report.Total.Percent(), report.Total.Covered, report.Total.Total)
	if worst := render.WorstFiles(report); worst != "" {
		fmt.Printf("\nleast covered files:\n%s", worst)
	}
}

func ci() error {
	if err := checkFormat(); err != nil {
		return err
	}
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return errors.New("golangci-lint is not on PATH; install it from https://golangci-lint.run/docs/welcome/install/")
	}
	steps := [][]string{
		{"go", "vet", "./..."},
		{"golangci-lint", "run", "./..."},
		{"go", "test", "./..."},
	}
	for _, step := range steps {
		if _, err := shell.Run(step[0], step[1:]...); err != nil {
			return err
		}
	}
	if err := diag(); err != nil {
		return err
	}
	if err := schemaCmd(); err != nil {
		return err
	}
	if err := exampleCmd(nil); err != nil {
		return err
	}
	return runCoverage(coverageOptions{enforce: true})
}

func checkFormat() error {
	out, err := shell.Capture("gofmt", "-l", ".")
	if err != nil {
		return err
	}
	unformatted := strings.Fields(out)
	if len(unformatted) == 0 {
		fmt.Println("gofmt: ok")
		return nil
	}
	return fmt.Errorf("gofmt: %d files need formatting: %s",
		len(unformatted), strings.Join(unformatted, ", "))
}

func diag() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	issues, err := diagcheck.Check(os.DirFS(root))
	if err != nil {
		return err
	}
	fmt.Print(diagcheck.Render(issues))
	if len(issues) > 0 {
		return fmt.Errorf("diag: %d inconsistencies", len(issues))
	}
	return nil
}

func bench(args []string) error {
	var check, record, accept bool
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.BoolVar(&check, "check", false, "fail when a tracked metric regressed")
	fs.BoolVar(&record, "record", false, "write the measurement into the performance log")
	fs.BoolVar(&accept, "accept", false, "adopt the current numbers even where they regressed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	output, err := shell.Capture("go", "test", "./...", "-run=NONE", "-bench=.", "-benchmem", "-p=1")
	if err != nil {
		return err
	}
	current := benchlog.Record{Benchmarks: benchlog.Parse(output)}
	if len(current.Benchmarks) == 0 {
		return errors.New("bench: no benchmarks ran")
	}
	fmt.Print(benchlog.Format(current))

	path := filepath.Join(root, lockName)
	baseline, err := readBaseline(path)
	if err != nil {
		return err
	}
	regressions := benchlog.Compare(baseline, current)
	for _, regression := range regressions {
		fmt.Println("regression:", regression)
	}
	for _, observation := range benchlog.Slower(baseline, current) {
		fmt.Println("slower:", observation)
	}
	if check && len(regressions) > 0 {
		return fmt.Errorf("bench: %d regressions", len(regressions))
	}
	if record || accept {
		next := benchlog.Merge(baseline, current)
		if accept {
			next = current
			next.WorkerSize = baseline.WorkerSize
			next.ClientRuntime = baseline.ClientRuntime
			next.ClientChunks = baseline.ClientChunks
		}
		if err := writeBaseline(path, next); err != nil {
			return err
		}
		fmt.Println("performance log updated:", path)
	}
	return nil
}

func readBaseline(path string) (benchlog.Record, error) {
	lock, err := readDevLock(path)
	if err != nil {
		return benchlog.Record{}, err
	}
	record := lock.Bench
	if record.Benchmarks == nil {
		record.Benchmarks = map[string]benchlog.Result{}
	}
	return record, nil
}

func writeBaseline(path string, record benchlog.Record) error {
	lock, err := readDevLock(path)
	if err != nil {
		return err
	}
	lock.Bench = record
	return writeDevLock(path, lock)
}

func repoRoot() (string, error) {
	out, err := shell.Capture("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func loadConfig(root string) (*config.Config, error) {
	text, err := os.ReadFile(filepath.Join(root, configName))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", configName, err)
	}
	return config.Parse(string(text))
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path), nil
		}
	}
	return "", errors.New("go.mod has no module directive")
}

func readDevLock(path string) (devLock, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return devLock{}, nil
	}
	if err != nil {
		return devLock{}, err
	}
	var lock devLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return devLock{}, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	return lock, nil
}

func writeDevLock(path string, lock devLock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func readLock(path string) (gate.Lock, error) {
	lock, err := readDevLock(path)
	return lock.Coverage, err
}

func writeLock(path string, coverage gate.Lock) error {
	lock, err := readDevLock(path)
	if err != nil {
		return err
	}
	lock.Coverage = coverage
	return writeDevLock(path, lock)
}

func writeStepSummary(markdown string) (err error) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	_, err = file.WriteString(markdown + "\n")
	return err
}

func schemaCmd() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(schemacheck.Path)))
	if err != nil {
		return fmt.Errorf("read %s: %w", schemacheck.Path, err)
	}
	issues, err := schemacheck.Check(data, appconfig.Config{})
	if err != nil {
		return err
	}
	for _, issue := range issues {
		fmt.Fprintln(os.Stderr, "schema:", issue.Message())
	}
	if len(issues) > 0 {
		return fmt.Errorf("%s and internal/config have drifted apart in %d places", schemacheck.Path, len(issues))
	}
	fmt.Println("schema: ok")
	return nil
}
