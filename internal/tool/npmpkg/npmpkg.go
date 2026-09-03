package npmpkg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

const (
	Scope      = "@apptivitypl"
	CLI        = Scope + "/rill"
	Create     = Scope + "/create-rill"
	CreateBin  = "create-rill"
	Repository = "https://github.com/apptivitypl/rill"
	License    = "MIT OR Apache-2.0"
	Engine     = ">=20"
)

type Platform struct {
	GOOS   string
	GOARCH string
	OS     string
	CPU    string
}

func Platforms() []Platform {
	return []Platform{
		{GOOS: "darwin", GOARCH: "amd64", OS: "darwin", CPU: "x64"},
		{GOOS: "darwin", GOARCH: "arm64", OS: "darwin", CPU: "arm64"},
		{GOOS: "linux", GOARCH: "amd64", OS: "linux", CPU: "x64"},
		{GOOS: "linux", GOARCH: "arm64", OS: "linux", CPU: "arm64"},
		{GOOS: "windows", GOARCH: "amd64", OS: "win32", CPU: "x64"},
		{GOOS: "windows", GOARCH: "arm64", OS: "win32", CPU: "arm64"},
	}
}

func (p Platform) Package() string {
	return fmt.Sprintf("%s-%s-%s", CLI, p.OS, p.CPU)
}

func (p Platform) Command() string {
	return fmt.Sprintf("rill-%s-%s", p.OS, p.CPU)
}

func (p Platform) Binary() string {
	if p.GOOS == "windows" {
		return "rill.exe"
	}
	return "rill"
}

func (p Platform) Archive(version string) string {
	if p.GOOS == "windows" {
		return fmt.Sprintf("rill_%s_%s_%s.zip", version, p.GOOS, p.GOARCH)
	}
	return fmt.Sprintf("rill_%s_%s_%s.tar.gz", version, p.GOOS, p.GOARCH)
}

type manifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	License      string            `json:"license"`
	Repository   string            `json:"repository"`
	Homepage     string            `json:"homepage"`
	Keywords     []string          `json:"keywords,omitempty"`
	Engines      map[string]string `json:"engines"`
	Bin          map[string]string `json:"bin,omitempty"`
	Files        []string          `json:"files"`
	OS           []string          `json:"os,omitempty"`
	CPU          []string          `json:"cpu,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Optional     map[string]string `json:"optionalDependencies,omitempty"`
}

func base(name, version, description string) manifest {
	return manifest{
		Name:        name,
		Version:     version,
		Description: description,
		License:     License,
		Repository:  Repository,
		Homepage:    Repository,
		Engines:     map[string]string{"node": Engine},
	}
}

func Launcher(version string) ([]byte, error) {
	optional := map[string]string{}
	for _, platform := range Platforms() {
		optional[platform.Package()] = version
	}
	entry := base(CLI, version, "The rill command line, as a prebuilt binary.")
	entry.Keywords = []string{"rill", "go", "web-framework", "cloudflare-workers"}
	entry.Bin = map[string]string{"rill": "bin/rill.js"}
	entry.Files = []string{"bin", "README.md"}
	entry.Optional = optional
	return encode(entry)
}

func Binary(version string, platform Platform) ([]byte, error) {
	entry := base(
		platform.Package(),
		version,
		fmt.Sprintf("The rill binary for %s %s.", platform.OS, platform.CPU),
	)
	entry.Bin = map[string]string{platform.Command(): "bin/" + platform.Binary()}
	entry.Files = []string{"bin"}
	entry.OS = []string{platform.OS}
	entry.CPU = []string{platform.CPU}
	return encode(entry)
}

func Scaffolder(version string) ([]byte, error) {
	entry := base(Create, version, "Scaffolds a rill project.")
	entry.Keywords = []string{"rill", "create", "scaffold"}
	entry.Bin = map[string]string{CreateBin: "bin/create-rill.js"}
	entry.Files = []string{"bin", "README.md"}
	entry.Dependencies = map[string]string{CLI: version}
	return encode(entry)
}

func Manifest(name, version string) ([]byte, error) {
	switch name {
	case CLI:
		return Launcher(version)
	case Create:
		return Scaffolder(version)
	}
	for _, platform := range Platforms() {
		if platform.Package() == name {
			return Binary(version, platform)
		}
	}
	return nil, fmt.Errorf("nothing knows how to package %q", name)
}

func Names() []string {
	names := []string{CLI, Create}
	for _, platform := range Platforms() {
		names = append(names, platform.Package())
	}
	sort.Strings(names)
	return names
}

func encode(entry manifest) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(entry); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
