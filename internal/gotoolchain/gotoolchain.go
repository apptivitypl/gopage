package gotoolchain

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	Version  = "go1.27.1"
	Override = "GOPAGE_GO"

	assetBase = "https://go.dev/dl/"
	homepage  = "https://go.dev/dl/"
)

type Build struct {
	Name   string
	Digest string
	Size   string
}

var assets = map[string]Build{
	"darwin/amd64":  {"go1.27.1.darwin-amd64.tar.gz", "8f8f52c6649542cf027bbc9b9c68d1ec042f9f34808a40413f0b8b3f66f3caa4", "72 MB"},
	"darwin/arm64":  {"go1.27.1.darwin-arm64.tar.gz", "ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12", "68 MB"},
	"linux/amd64":   {"go1.27.1.linux-amd64.tar.gz", "63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445", "71 MB"},
	"linux/arm64":   {"go1.27.1.linux-arm64.tar.gz", "3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec", "67 MB"},
	"windows/amd64": {"go1.27.1.windows-amd64.zip", "a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d", "79 MB"},
	"windows/arm64": {"go1.27.1.windows-arm64.zip", "13b69b87bb0e83f96bc68560a8cace7f0343b1e03469f1110ea18d17e3234069", "75 MB"},
}

func Asset(goos, arch string) (Build, error) {
	build, ok := assets[goos+"/"+arch]
	if !ok {
		return Build{}, fmt.Errorf("go ships no %s build for %s/%s", Version, goos, arch)
	}
	return build, nil
}

type Resolved struct {
	Path string
	Env  []string
}

func (r Resolved) Command() string {
	if r.Path == "" {
		return "go"
	}
	return r.Path
}

type Toolchain struct {
	Path     string
	CacheDir string
	GOOS     string
	GOARCH   string
	Lookup   func(string) (string, error)
	Fetch    func(url, target, digest string) error
	Unpack   func(archive, into string) error
	Announce func(string)
}

func (t Toolchain) Resolve() (Resolved, error) {
	if t.Path != "" {
		return Resolved{Path: t.Path}, nil
	}
	if named := os.Getenv(Override); named != "" {
		if _, err := os.Stat(named); err != nil {
			return Resolved{}, fmt.Errorf("%s names %s, which is not there: %w", Override, named, err)
		}
		return Resolved{Path: named}, nil
	}
	lookup := t.Lookup
	if lookup == nil {
		lookup = exec.LookPath
	}
	if found, err := lookup("go"); err == nil {
		return Resolved{Path: found}, nil
	}
	return t.managed()
}

func (t Toolchain) managed() (Resolved, error) {
	root, err := t.root()
	if err != nil {
		return Resolved{}, err
	}
	binary := filepath.Join(root, "go", "bin", t.command())
	held := Resolved{Path: binary, Env: []string{"GOTOOLCHAIN=local"}}
	if _, err := os.Stat(binary); err == nil {
		return held, nil
	}
	build, err := Asset(t.platform())
	if err != nil {
		return Resolved{}, err
	}
	if t.Fetch == nil || t.Unpack == nil {
		return Resolved{}, fmt.Errorf("no go toolchain on PATH and none in %s\n"+
			"install Go from %s, or point %s at a go binary this machine already has",
			root, homepage, Override)
	}
	if t.Announce != nil {
		t.Announce(fmt.Sprintf("no go toolchain on PATH; fetching %s once, %s, into %s",
			Version, build.Size, root))
	}
	if err := t.install(root, build); err != nil {
		return Resolved{}, fmt.Errorf("fetching %s: %w\n"+
			"install Go from %s, or point %s at a go binary this machine already has",
			Version, err, homepage, Override)
	}
	return held, nil
}

func (t Toolchain) install(root string, build Build) error {
	staging := root + ".partial"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	archive := filepath.Join(staging, build.Name)
	if err := t.Fetch(assetBase+build.Name, archive, build.Digest); err != nil {
		return err
	}
	unpacked := filepath.Join(staging, "unpacked")
	if err := t.Unpack(archive, unpacked); err != nil {
		return err
	}
	if err := os.Remove(archive); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	return os.Rename(unpacked, root)
}

func (t Toolchain) root() (string, error) {
	base := t.CacheDir
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(cache, "gopage")
	}
	return filepath.Join(base, "go", Version), nil
}

func (t Toolchain) platform() (string, string) {
	goos, arch := t.GOOS, t.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	return goos, arch
}

func (t Toolchain) command() string {
	if goos, _ := t.platform(); goos == "windows" {
		return "go.exe"
	}
	return "go"
}
