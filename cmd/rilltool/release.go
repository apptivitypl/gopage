package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apptivitypl/rill/internal/tool/npmpkg"
	"github.com/apptivitypl/rill/internal/tool/release"
	"github.com/apptivitypl/rill/internal/tool/shell"
)

const (
	trustRepository = "apptivitypl/rill"
	trustWorkflow   = "publish.yml"
)

func releaseCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("missing subcommand\n\n" + releaseList())
	}
	switch args[0] {
	case "plan":
		return releasePlan(args[1:])
	case "check":
		return releaseCheck()
	case "run":
		return releaseRun(args[1:])
	case "trust":
		return releaseTrust()
	case "tags":
		return releaseTags()
	default:
		return fmt.Errorf("unknown release subcommand %q\n\n%s", args[0], releaseList())
	}
}

func releaseList() string {
	return "release subcommands:\n  plan [--json]\n  check\n  run [package] [--from DIR] [--out DIR] [--publish]\n  trust\n  tags"
}

func loadManifest() (*release.Manifest, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	text, err := os.ReadFile(filepath.Join(root, release.FileName))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", release.FileName, err)
	}
	return release.Parse(string(text))
}

func releaseStatuses() ([]release.Status, error) {
	manifest, err := loadManifest()
	if err != nil {
		return nil, err
	}
	return release.Plan(manifest, release.Published, release.Changed)
}

func releasePlan(args []string) error {
	var asJSON bool
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.BoolVar(&asJSON, "json", false, "write the plan as json for a workflow matrix")
	if err := fs.Parse(args); err != nil {
		return err
	}
	statuses, err := releaseStatuses()
	if err != nil {
		return err
	}
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(release.Pending(statuses))
	}
	fmt.Print(release.Render(statuses))
	return nil
}

func releaseCheck() error {
	statuses, err := releaseStatuses()
	if err != nil {
		return err
	}
	problems := release.Problems(statuses)
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	if len(problems) > 0 {
		return fmt.Errorf("release: %d packages changed without a version to publish them under", len(problems))
	}
	fmt.Println("release: ok")
	return nil
}

func releaseRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var name, from, out string
	var publish bool
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	fs.StringVar(&from, "from", "", "directory holding the release archives the binaries come from")
	fs.StringVar(&out, "out", npmOut, "directory to assemble the packages in")
	fs.BoolVar(&publish, "publish", false, "publish instead of stopping at the assembled folder")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("release run takes at most one package name\n\n" + releaseList())
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	manifest, err := loadManifest()
	if err != nil {
		return err
	}
	wanted, err := chosen(manifest, name)
	if err != nil {
		return err
	}
	for _, pkg := range wanted {
		if err := runOne(root, pkg, from, out, publish); err != nil {
			return err
		}
	}
	return nil
}

func chosen(manifest *release.Manifest, name string) ([]release.Package, error) {
	if name != "" {
		pkg, ok := manifest.Package(name)
		if !ok {
			return nil, fmt.Errorf("%s has no package %q", release.FileName, name)
		}
		return []release.Package{pkg}, nil
	}
	statuses, err := release.Plan(manifest, release.Published, release.Changed)
	if err != nil {
		return nil, err
	}
	var pending []release.Package
	for _, status := range release.Pending(statuses) {
		pending = append(pending, status.Package)
	}
	return pending, nil
}

func runOne(root string, pkg release.Package, from, out string, publish bool) error {
	if pkg.Kind == release.KindGo {
		fmt.Printf("%s is released by tagging %s and letting goreleaser run; this tool does not touch git\n", pkg.Name, pkg.TagName())
		return nil
	}
	if pkg.Kind != release.KindNPM {
		return fmt.Errorf("nothing here knows how to publish a %s package", pkg.Kind)
	}
	for _, member := range npmMembers(pkg.Name) {
		dir, err := assembleNPM(root, member, pkg.Version, from, resolveOut(root, out))
		if err != nil {
			return err
		}
		fmt.Printf("assembled %s %s in %s\n", member, pkg.Version, dir)
		if !publish {
			fmt.Printf("  npm publish %s --access public\n", dir)
			continue
		}
		if err := shell.Run("npm", "publish", dir, "--access", "public"); err != nil {
			return err
		}
	}
	return nil
}

func npmMembers(name string) []string {
	if name != npmpkg.CLI {
		return []string{name}
	}
	members := []string{npmpkg.CLI}
	for _, platform := range npmpkg.Platforms() {
		members = append(members, platform.Package())
	}
	return members
}

func resolveOut(root, out string) string {
	if filepath.IsAbs(out) {
		return out
	}
	return filepath.Join(root, out)
}

func releaseTrust() error {
	manifest, err := loadManifest()
	if err != nil {
		return err
	}
	for _, pkg := range manifest.Packages() {
		if pkg.Kind != release.KindNPM {
			continue
		}
		for _, member := range npmMembers(pkg.Name) {
			if err := trustOne(member); err != nil {
				return err
			}
		}
	}
	return nil
}

func trustOne(name string) error {
	if held, err := shell.Capture("npm", "trust", "list", name); err == nil && strings.Contains(held, npmpkg.Repository) {
		fmt.Printf("%s already trusts this repository\n", name)
		return nil
	}
	return shell.Run("npm", "trust", "github", name,
		"--repo", trustRepository,
		"--file", trustWorkflow,
		"--allow-publish",
		"--yes")
}

func releaseTags() error {
	manifest, err := loadManifest()
	if err != nil {
		return err
	}
	for _, pkg := range manifest.Packages() {
		if pkg.Kind == release.KindGo {
			continue
		}
		published, err := release.Published(pkg)
		if err != nil {
			return err
		}
		if !published {
			continue
		}
		tagged, err := release.Tagged(pkg.TagName())
		if err != nil {
			return err
		}
		if !tagged {
			fmt.Println(pkg.TagName())
		}
	}
	return nil
}
