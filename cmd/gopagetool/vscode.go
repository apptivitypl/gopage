package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/apptivitypl/gopage/internal/compile"
	"github.com/apptivitypl/gopage/internal/runtime"
	"github.com/apptivitypl/gopage/internal/syntax"
	"github.com/apptivitypl/gopage/internal/tool/grammarcheck"
	"github.com/apptivitypl/gopage/internal/tool/release"
)

var unreleased = regexp.MustCompile(`("version"\s*:\s*)"` + regexp.QuoteMeta(grammarcheck.Unreleased) + `"`)

func vscodeCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("missing subcommand\n\n" + vscodeList())
	}
	switch args[0] {
	case "check":
		return vscodeCheck()
	case "version":
		return vscodeVersion(args[1:])
	default:
		return fmt.Errorf("unknown vscode subcommand %q\n\n%s", args[0], vscodeList())
	}
}

func vscodeList() string {
	return "vscode subcommands:\n" +
		"  check\n" +
		"  version --set VERSION [--pre-release]"
}

func vocabulary() map[string][]string {
	return map[string][]string{
		"directive-name":    syntax.Directives(),
		"filter-name":       runtime.FilterNames(),
		"strategy-value":    compile.Strategies(),
		"builtin-component": compile.BuiltinNames(),
	}
}

func vscodeCheck() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	grammar, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(grammarcheck.Path)))
	if err != nil {
		return fmt.Errorf("read %s: %w", grammarcheck.Path, err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(grammarcheck.ManifestPath)))
	if err != nil {
		return fmt.Errorf("read %s: %w", grammarcheck.ManifestPath, err)
	}
	snippets, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(grammarcheck.SnippetsPath)))
	if err != nil {
		return fmt.Errorf("read %s: %w", grammarcheck.SnippetsPath, err)
	}
	issues, err := grammarcheck.Check(grammar, vocabulary())
	if err != nil {
		return err
	}
	stamped, err := grammarcheck.CheckManifest(manifest)
	if err != nil {
		return err
	}
	unwritten, err := grammarcheck.CheckSnippets(snippets, syntax.Directives())
	if err != nil {
		return err
	}
	issues = append(issues, stamped...)
	issues = append(issues, unwritten...)
	for _, issue := range issues {
		fmt.Fprintln(os.Stderr, "vscode:", issue.Message())
	}
	if len(issues) > 0 {
		return fmt.Errorf("the vscode extension has drifted from the compiler in %d places", len(issues))
	}
	fmt.Println("vscode: ok")
	return nil
}

func vscodeVersion(args []string) error {
	var version string
	var preRelease bool
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.StringVar(&version, "set", "", "the version this build carries, for example 0.1.0")
	fs.BoolVar(&preRelease, "pre-release", false, "this build goes to the pre-release channel")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if version == "" {
		return errors.New("--set is required\n\n" + vscodeList())
	}
	if err := release.Valid(version); err != nil {
		return err
	}
	if err := onChannel(version, preRelease); err != nil {
		return err
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(grammarcheck.ManifestPath))
	source, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", grammarcheck.ManifestPath, err)
	}
	stamped, err := stampVersion(source, version)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, stamped, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s carries %s; the checkout is now dirty and must not be committed\n",
		grammarcheck.ManifestPath, version)
	return nil
}

func stampVersion(source []byte, version string) ([]byte, error) {
	stamped := unreleased.ReplaceAll(source, []byte(`${1}"`+version+`"`))
	if string(stamped) == string(source) {
		return nil, fmt.Errorf("%s does not carry version %q, so nothing was stamped",
			grammarcheck.ManifestPath, grammarcheck.Unreleased)
	}
	return stamped, nil
}

func onChannel(version string, preRelease bool) error {
	fields := strings.Split(version, ".")
	if len(fields) < 2 {
		return fmt.Errorf("%q names no minor version", version)
	}
	minor, err := strconv.Atoi(fields[1])
	if err != nil {
		return fmt.Errorf("%q names no minor version", version)
	}
	odd := minor%2 == 1
	if odd == preRelease {
		return nil
	}
	channel, want := "released", "an even"
	if preRelease {
		channel, want = "pre-release", "an odd"
	}
	return fmt.Errorf(
		"the marketplace has no pre-release versions, only channels: a %s build needs %s minor, and %s has %d",
		channel, want, version, minor)
}
