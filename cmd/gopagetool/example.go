package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apptivitypl/gopage/internal/build"
	"github.com/apptivitypl/gopage/internal/scaffold"
	"github.com/apptivitypl/gopage/internal/tool/examplecheck"
	"github.com/apptivitypl/gopage/internal/tool/release"
	"github.com/apptivitypl/gopage/internal/tool/shell"
)

var workspaceEnv = []string{"GOWORK="}

var outsideEnv = []string{"GOWORK=off"}

func exampleCmd(args []string) error {
	var update, workspace, verify bool
	fs := flag.NewFlagSet("example", flag.ContinueOnError)
	fs.BoolVar(&update, "update", false, "rewrite the committed examples from the templates")
	fs.BoolVar(&workspace, "workspace", false, "write go.work so the examples build against this checkout")
	fs.BoolVar(&verify, "verify", false, "build the committed examples the way someone outside this repo does")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if verify {
		return verifyExamples(root)
	}
	if workspace {
		version, err := examplecheck.Examples()[0].PinnedVersion(root)
		if err != nil {
			return err
		}
		return writeWorkspace(root, version)
	}
	var differences []examplecheck.Difference
	for _, example := range examplecheck.Examples() {
		version, err := exampleVersion(root, example, update)
		if err != nil {
			return err
		}
		found, err := oneExample(root, example, version, update)
		if err != nil {
			return err
		}
		differences = append(differences, found...)
	}
	if update {
		version, err := examplecheck.Examples()[0].PinnedVersion(root)
		if err != nil {
			return err
		}
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

func exampleVersion(root string, example examplecheck.Example, update bool) (string, error) {
	if update {
		return releasedVersion()
	}
	return example.PinnedVersion(root)
}

func releasedVersion() (string, error) {
	out, err := shell.Capture("git", "tag", "--list", "v*", "--sort=-v:refname")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if tag := strings.TrimSpace(line); tag != "" {
				return tag, nil
			}
		}
	}
	return requiredVersion()
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

func verifyExamples(root string) error {
	runner := build.ExecRunner{Verbose: true}
	for _, example := range examplecheck.Examples() {
		dir := filepath.Join(root, filepath.FromSlash(example.Dir()))
		if _, err := os.Stat(filepath.Join(dir, "go.sum")); err != nil {
			return fmt.Errorf("example: %s has no go.sum, so nobody outside this repo can build it: %w", example.Dir(), err)
		}
		command := build.Command{
			Dir:  root,
			Env:  outsideEnv,
			Name: "go",
			Args: []string{"run", "./cmd/gopage", "build", "--dir", example.Dir()},
		}
		if err := runner.Run(command); err != nil {
			return fmt.Errorf("example: %s does not build without the workspace: %w", example.Dir(), err)
		}
	}
	fmt.Println("example: every committed example builds from the registry, without the workspace")
	return nil
}

func oneExample(root string, example examplecheck.Example, version string, update bool) ([]examplecheck.Difference, error) {
	staging, err := os.MkdirTemp("", "gopage-example-")
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
		Env:  outsideEnv,
		Name: "go",
		Args: []string{"mod", "tidy"},
	}
	if err := (build.ExecRunner{}).Run(tidy); err != nil {
		return fmt.Errorf("example: %s cannot resolve its modules, so it would ship without a go.sum "+
			"and nobody outside this repo could build it; the version it pins must be published first: %w", dir, err)
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
	replacement := fmt.Sprintf("%s@%s=.", examplecheck.Module, version)
	edit := []string{"work", "edit", "-replace", replacement}
	if err := runner.Run(build.Command{Dir: root, Env: workspaceEnv, Name: "go", Args: edit}); err != nil {
		return err
	}
	fmt.Printf("example: go.work points %s %s at this checkout\n", examplecheck.Module, version)
	return nil
}
