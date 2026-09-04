package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apptivitypl/gopage/internal/tool/npmpkg"
	"github.com/apptivitypl/gopage/internal/tool/release"
	"github.com/apptivitypl/gopage/internal/tool/shell"
)

const (
	trustRepository = "apptivitypl/gopage"
	trustWorkflow   = "publish.yml"
)

func releaseCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("missing subcommand\n\n" + releaseList())
	}
	switch args[0] {
	case "plan":
		return releasePlan(args[1:])
	case "run":
		return releaseRun(args[1:])
	case "trust":
		return releaseTrust()
	case "tags":
		return releaseTags(args[1:])
	default:
		return fmt.Errorf("unknown release subcommand %q\n\n%s", args[0], releaseList())
	}
}

func releaseList() string {
	return "release subcommands:\n" +
		"  plan --version VERSION [--json]\n" +
		"  run --version VERSION [package] [--from DIR] [--out DIR] [--publish]\n" +
		"  trust\n" +
		"  tags --version VERSION"
}

func wanted(fs *flag.FlagSet, version *string) {
	fs.StringVar(version, "version", "", "the version being released, for example 0.2.2")
}

func given(version string) error {
	if version == "" {
		return errors.New("--version is required; the release carries no version of its own until you name one")
	}
	return release.Valid(version)
}

func publishedFully(artifact release.Artifact, version string) (bool, error) {
	if artifact.Kind != release.KindNPM {
		return release.Tagged(artifact.Tag(version))
	}
	for _, member := range npmMembers(artifact.Name) {
		on, err := release.OnRegistry(member, version)
		if err != nil {
			return false, err
		}
		if !on {
			return false, nil
		}
	}
	return true, nil
}

func releasePlan(args []string) error {
	var version string
	var asJSON bool
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	wanted(fs, &version)
	fs.BoolVar(&asJSON, "json", false, "write the plan as json for a workflow matrix")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := given(version); err != nil {
		return err
	}
	statuses, err := release.Plan(version, func(artifact release.Artifact) (bool, error) {
		return publishedFully(artifact, version)
	})
	if err != nil {
		return err
	}
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(release.Pending(statuses))
	}
	fmt.Print(release.Render(version, statuses))
	return nil
}

func releaseRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var name, from, out, version string
	var publish bool
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	wanted(fs, &version)
	fs.StringVar(&from, "from", "", "directory holding the release archives the binaries come from")
	fs.StringVar(&out, "out", npmOut, "directory to assemble the packages in")
	fs.BoolVar(&publish, "publish", false, "publish instead of stopping at the assembled folder")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("release run takes at most one package name\n\n" + releaseList())
	}
	if err := given(version); err != nil {
		return err
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	for _, artifact := range chosen(name) {
		if err := runOne(root, artifact, version, from, out, publish); err != nil {
			return err
		}
	}
	return nil
}

func chosen(name string) []release.Artifact {
	if name == "" {
		return release.Artifacts()
	}
	for _, artifact := range release.Artifacts() {
		if artifact.Name == name {
			return []release.Artifact{artifact}
		}
	}
	return []release.Artifact{{Name: name, Kind: release.KindNPM}}
}

func runOne(root string, artifact release.Artifact, version, from, out string, publish bool) error {
	if artifact.Kind == release.KindGo {
		fmt.Printf("%s is released by tagging %s and letting goreleaser run; this tool does not touch git\n",
			artifact.Name, artifact.Tag(version))
		return nil
	}
	target := resolveOut(root, out)
	job := npmRelease{
		version: version,
		assemble: func(member string) (string, error) {
			return assembleNPM(root, member, version, from, target)
		},
		present:  func(member string) (bool, error) { return release.OnRegistry(member, version) },
		publish:  func(dir string) (string, error) { return shell.Run("npm", "publish", dir, "--access", "public") },
		pause:    time.Sleep,
		out:      os.Stdout,
		verify:   true,
		patience: reviewPatience,
		interval: reviewInterval,
	}
	if !publish {
		job.present = func(string) (bool, error) { return false, nil }
		job.publish = func(dir string) (string, error) {
			fmt.Printf("  npm publish %s --access public\n", dir)
			return "", nil
		}
		job.verify = false
	}
	return job.run(npmMembers(artifact.Name))
}

func npmMembers(name string) []string {
	if name != npmpkg.CLI {
		return []string{name}
	}
	var members []string
	for _, platform := range npmpkg.Platforms() {
		members = append(members, platform.Package())
	}
	return append(members, npmpkg.CLI)
}

func resolveOut(root, out string) string {
	if filepath.IsAbs(out) {
		return out
	}
	return filepath.Join(root, out)
}

func releaseTrust() error {
	for _, artifact := range release.Artifacts() {
		if artifact.Kind != release.KindNPM {
			continue
		}
		for _, member := range npmMembers(artifact.Name) {
			if err := trustOne(member); err != nil {
				return err
			}
		}
	}
	return nil
}

func trustOne(name string) error {
	held, err := shell.Capture("npm", "trust", "list", name)
	if err == nil && strings.Contains(held, trustRepository) && strings.Contains(held, trustWorkflow) {
		fmt.Printf("%s already trusts %s through %s\n", name, trustRepository, trustWorkflow)
		return nil
	}
	_, err = shell.Run("npm", "trust", "github", name,
		"--repo", trustRepository,
		"--file", trustWorkflow,
		"--allow-publish",
		"--yes")
	return err
}

func releaseTags(args []string) error {
	var version string
	fs := flag.NewFlagSet("tags", flag.ContinueOnError)
	wanted(fs, &version)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := given(version); err != nil {
		return err
	}
	for _, artifact := range release.Artifacts() {
		if artifact.Kind == release.KindGo {
			continue
		}
		published, err := publishedFully(artifact, version)
		if err != nil {
			return err
		}
		if !published {
			continue
		}
		tag := artifact.Tag(version)
		tagged, err := release.Tagged(tag)
		if err != nil {
			return err
		}
		if !tagged {
			fmt.Println(tag)
		}
	}
	return nil
}
