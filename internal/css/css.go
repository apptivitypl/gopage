package css

import (
	"fmt"
	"github.com/apptivitypl/gopage/internal/paths"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	Version   = "v4.1.14"
	assetBase = "https://github.com/tailwindlabs/tailwindcss/releases/download/"
)

type Processor interface {
	Process(input, output, inventory string) error
}

type Passthrough struct{}

func (Passthrough) Process(input, output, _ string) error {
	source, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	return os.WriteFile(output, source, 0o644)
}

type Tailwind struct {
	Binary   string
	Fetch    Fetcher
	CacheDir string
	Minify   bool
}

type Fetcher func(url, target, digest string) error

func (t Tailwind) Process(input, output, inventory string) error {
	binary, err := t.resolve()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	args := []string{"--input", input, "--output", output}
	if t.Minify {
		args = append(args, "--minify")
	}
	command := exec.Command(binary, args...)
	command.Dir = base(input, inventory)
	out, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailwindcss: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func base(input, inventory string) string {
	if inventory == "" {
		return filepath.Dir(input)
	}
	return commonRoot(filepath.Dir(input), filepath.Dir(inventory))
}

func commonRoot(left, right string) string {
	for !strings.HasPrefix(right, left) {
		parent := filepath.Dir(left)
		if parent == left {
			return left
		}
		left = parent
	}
	return left
}

func (t Tailwind) resolve() (string, error) {
	if t.Binary != "" {
		return t.Binary, nil
	}
	target, err := t.path()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	if t.Fetch == nil {
		return "", fmt.Errorf("tailwind %s is missing from %s\n"+
			"run gopage css install to fetch it, or set \"css\": {\"engine\": \"plain\"} in %s", Version, target, paths.Config)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	build, err := Asset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if err := t.Fetch(assetBase+Version+"/"+build.Name, target, build.Digest); err != nil {
		return "", fmt.Errorf("downloading tailwind %s: %w\n"+
			"run gopage css install again, or set \"css\": {\"engine\": \"plain\"} in %s",
			Version, err, paths.Config)
	}
	return target, os.Chmod(target, 0o755)
}

func (t Tailwind) Install() (string, error) {
	return t.resolve()
}

func (t Tailwind) path() (string, error) {
	root := t.CacheDir
	if root == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(cache, "gopage")
	}
	return filepath.Join(root, "tailwind", Version, binaryName(runtime.GOOS)), nil
}

func binaryName(goos string) string {
	if goos == "windows" {
		return "tailwindcss.exe"
	}
	return "tailwindcss"
}

type Build struct {
	Name   string
	Digest string
}

var assets = map[string]Build{
	"darwin/arm64":  {"tailwindcss-macos-arm64", "e722b752f51def86d42e886b4c1171f2d09a4be1a7487a0a51e4aff8e7603ce3"},
	"darwin/amd64":  {"tailwindcss-macos-x64", "67b25b6103fa7677637e5a5de3327fec3335da316d90d3fdb1a4cd72bda41c0a"},
	"linux/arm64":   {"tailwindcss-linux-arm64", "314941f5f6e143e74e740c587ad1fbaaede5462572dd330bbe0937e611e966db"},
	"linux/amd64":   {"tailwindcss-linux-x64", "bc34c301b080b6e6b98ed24118419833f966f6f347e556945d6557d36a44a56e"},
	"windows/amd64": {"tailwindcss-windows-x64.exe", "ae892cdb0817fbe6b692fc67bb1339a728f21116020e620bc4b94d87d6ba1fee"},
}

func Asset(goos, arch string) (Build, error) {
	build, ok := assets[goos+"/"+arch]
	if !ok {
		return Build{}, fmt.Errorf("tailwind ships no standalone build for %s/%s", goos, arch)
	}
	return build, nil
}
