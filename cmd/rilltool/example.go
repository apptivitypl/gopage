package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apptivitypl/rill/internal/build"
	"github.com/apptivitypl/rill/internal/scaffold"
	"github.com/apptivitypl/rill/internal/tool/examplecheck"
	"github.com/apptivitypl/rill/internal/tool/release"
)

const exampleModule = "github.com/apptivitypl/rill"

var workspaceEnv = []string{"GOWORK="}

func exampleCmd(args []string) error {
	var update, workspace bool
	fs := flag.NewFlagSet("example", flag.ContinueOnError)
	fs.BoolVar(&update, "update", false, "rewrite the committed examples from the templates")
	fs.BoolVar(&workspace, "workspace", false, "write go.work so the examples build against this checkout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	version, err := requiredVersion()
	if err != nil {
		return err
	}
	if workspace {
		return writeWorkspace(root, version)
	}
	var differences []examplecheck.Difference
	for _, example := range examplecheck.Examples() {
		found, err := oneExample(root, example, version, update)
		if err != nil {
			return err
		}
		differences = append(differences, found...)
	}
	if update {
		if err := refreshWorkspace(root, version); err != nil {
			return err
		}
		fmt.Println("example: written")
		return nil
	}
	fmt.Print(examplecheck.Render(differences))
	if len(differences) > 0 {
		return fmt.Errorf("example: %d differences from the templates", len(differences))
	}
	return nil
}

func requiredVersion() (string, error) {
	manifest, err := loadManifest()
	if err != nil {
		return "", err
	}
	pkg, ok := manifest.Package(versionPackage)
	if !ok {
		return "", fmt.Errorf("%s has no package %q", release.FileName, versionPackage)
	}
	return pkg.TagName(), nil
}

func oneExample(root string, example examplecheck.Example, version string, update bool) ([]examplecheck.Difference, error) {
	staging, err := os.MkdirTemp("", "rill-example-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	config := example.Config(version)
	config.Dir = filepath.Join(staging, example.Name)
	if err := scaffold.Create(config); err != nil {
		return nil, err
	}
	committed := filepath.Join(root, filepath.FromSlash(example.Dir()))
	if update {
		if err := replaceTree(committed, config.Dir); err != nil {
			return nil, err
		}
		return nil, resolve(committed)
	}
	if _, err := os.Stat(committed); err != nil {
		return []examplecheck.Difference{{
			Example: example.Name,
			Path:    example.Dir(),
			Kind:    examplecheck.Missing,
		}}, nil
	}
	return examplecheck.Compare(example.Name, os.DirFS(committed), os.DirFS(config.Dir))
}

func replaceTree(committed, generated string) error {
	if err := clearGenerated(committed); err != nil {
		return err
	}
	if err := os.MkdirAll(committed, 0o755); err != nil {
		return err
	}
	return copyDir(generated, committed)
}

func clearGenerated(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if kept(entry.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func kept(name string) bool {
	for _, extra := range append(examplecheck.Extras(), examplecheck.Skipped()...) {
		if name == extra {
			return true
		}
	}
	return false
}

func resolve(dir string) error {
	if err := build.Bootstrap(dir); err != nil {
		return err
	}
	tidy := build.Command{
		Dir:  dir,
		Env:  []string{"GOWORK=off"},
		Name: "go",
		Args: []string{"mod", "tidy"},
	}
	if err := (build.ExecRunner{}).Run(tidy); err != nil {
		fmt.Fprintf(os.Stderr, "example: %s has no go.sum yet, because %s is not published: %v\n",
			dir, versionPackage, err)
		if err := os.Remove(filepath.Join(dir, "go.sum")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func refreshWorkspace(root, version string) error {
	if _, err := os.Stat(filepath.Join(root, "go.work")); err != nil {
		return nil
	}
	return writeWorkspace(root, version)
}

func writeWorkspace(root, version string) error {
	uses := []string{"work", "init", "."}
	for _, example := range examplecheck.Examples() {
		uses = append(uses, "./"+example.Dir())
	}
	path := filepath.Join(root, "go.work")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	runner := build.ExecRunner{Verbose: true}
	if err := runner.Run(build.Command{Dir: root, Env: workspaceEnv, Name: "go", Args: uses}); err != nil {
		return err
	}
	replacement := fmt.Sprintf("%s@%s=.", exampleModule, version)
	edit := []string{"work", "edit", "-replace", replacement}
	if err := runner.Run(build.Command{Dir: root, Env: workspaceEnv, Name: "go", Args: edit}); err != nil {
		return err
	}
	fmt.Printf("example: go.work points %s %s at this checkout\n", exampleModule, version)
	return nil
}
