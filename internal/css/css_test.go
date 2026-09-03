package css

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPassthroughCopiesTheStylesheet(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "app.css")
	output := filepath.Join(dir, "out", "app.css")
	if err := os.WriteFile(input, []byte("body{color:red}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (Passthrough{}).Process(input, output, ""); err != nil {
		t.Fatalf("Process: %v", err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "body{color:red}" {
		t.Errorf("output = %q, want the input unchanged", got)
	}
}

func TestPassthroughReportsAMissingInput(t *testing.T) {
	if err := (Passthrough{}).Process(filepath.Join(t.TempDir(), "absent.css"), "out.css", ""); err == nil {
		t.Error("a missing stylesheet must be reported")
	}
}

func fakeTailwind(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script, and windows has no shebang")
	}
	binary := filepath.Join(t.TempDir(), "tailwindcss")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	return binary
}

func TestTailwindCallsTheBinaryWithInputAndOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "app.css")
	output := filepath.Join(dir, "gen", "app.css")
	if err := os.WriteFile(input, []byte("@import \"tailwindcss\";"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	binary := fakeTailwind(t, `echo "args: $@" > "$4"`)

	processor := Tailwind{Binary: binary, Minify: true}
	if err := processor.Process(input, output, filepath.Join(dir, "inventory.txt")); err != nil {
		t.Fatalf("Process: %v", err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"--input", "--output", "--minify"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("args = %q, want %q", got, want)
		}
	}
}

func TestTailwindReportsWhatTheBinarySaid(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "app.css")
	if err := os.WriteFile(input, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	binary := fakeTailwind(t, `echo "unknown at-rule @source" >&2; exit 2`)
	err := Tailwind{Binary: binary}.Process(input, filepath.Join(dir, "out.css"), "")
	if err == nil || !strings.Contains(err.Error(), "unknown at-rule") {
		t.Errorf("err = %v, want the binary's own message", err)
	}
}

func TestTailwindUsesTheCachedBinary(t *testing.T) {
	cache := t.TempDir()
	target := filepath.Join(cache, "tailwind", Version, binaryName(runtime.GOOS))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	var fetched bool
	processor := Tailwind{CacheDir: cache, Fetch: func(string, string, string) error {
		fetched = true
		return nil
	}}
	path, err := processor.Install()
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if path != target {
		t.Errorf("path = %q, want the cached binary", path)
	}
	if fetched {
		t.Error("a cached binary must not be downloaded again")
	}
}

func TestTailwindDownloadsWhenTheCacheIsEmpty(t *testing.T) {
	cache := t.TempDir()
	var asked, wanted string
	processor := Tailwind{CacheDir: cache, Fetch: func(url, target, digest string) error {
		asked, wanted = url, digest
		return os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}}
	if _, err := processor.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !strings.Contains(asked, Version) {
		t.Errorf("url = %q, want the pinned version", asked)
	}
	build, err := Asset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("no standalone build for this platform: %v", err)
	}
	if !strings.HasSuffix(asked, build.Name) {
		t.Errorf("url = %q, want the asset for this platform", asked)
	}
	if wanted != build.Digest || len(wanted) != 64 {
		t.Errorf("digest = %q, want the pinned sha256 handed to the fetcher", wanted)
	}
}

func TestTailwindWithoutADownloaderExplainsItself(t *testing.T) {
	_, err := Tailwind{CacheDir: t.TempDir()}.Install()
	if err == nil || !strings.Contains(err.Error(), "rill css install") {
		t.Errorf("err = %v, want the instruction", err)
	}
}

func TestTailwindReportsAFailedDownload(t *testing.T) {
	processor := Tailwind{CacheDir: t.TempDir(), Fetch: func(string, string, string) error {
		return errors.New("no route to host")
	}}
	_, err := processor.Install()
	if err == nil || !strings.Contains(err.Error(), "no route to host") {
		t.Errorf("err = %v, want the download failure", err)
	}
	if !strings.Contains(err.Error(), `"engine": "plain"`) {
		t.Errorf("err = %v, want the way out", err)
	}
}

func TestAssetKnowsTheSupportedPlatforms(t *testing.T) {
	cases := map[string]string{
		"darwin/arm64":  "tailwindcss-macos-arm64",
		"linux/amd64":   "tailwindcss-linux-x64",
		"windows/amd64": "tailwindcss-windows-x64.exe",
	}
	for pair, want := range cases {
		goos, arch, _ := strings.Cut(pair, "/")
		got, err := Asset(goos, arch)
		if err != nil || got.Name != want {
			t.Errorf("Asset(%q) = %q, %v, want %q", pair, got.Name, err, want)
		}
	}
	if _, err := Asset("plan9", "mips"); err == nil {
		t.Error("an unsupported platform must be reported")
	}
}

func TestTheCacheFallsBackToTheUserDirectory(t *testing.T) {
	path, err := Tailwind{}.path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if !strings.Contains(path, filepath.Join("rill", "tailwind", Version)) {
		t.Errorf("path = %q, want the versioned cache", path)
	}
}

func TestTheWorkingDirectoryCoversBothTheInputAndTheInventory(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "static", "app.css")
	inventory := filepath.Join(root, "gen", ".cache", "inventory.txt")
	if got := base(input, inventory); got != root {
		t.Errorf("base = %q, want %q so tailwind can read both", got, root)
	}
	if got := base(input, ""); got != filepath.Dir(input) {
		t.Errorf("base = %q, want the input directory", got)
	}
	if got := base("/one/a.css", "/two/b.txt"); got != string(filepath.Separator) {
		t.Errorf("base = %q, want the filesystem root for unrelated paths", got)
	}
}

func TestPassthroughReportsATargetItCannotCreate(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "app.css")
	if err := os.WriteFile(input, []byte("body{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (Passthrough{}).Process(input, filepath.Join(blocked, "out.css"), ""); err == nil {
		t.Error("a target under a file must be reported")
	}
}

func TestTailwindReportsATargetItCannotCreate(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "app.css")
	if err := os.WriteFile(input, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	binary := fakeTailwind(t, "exit 0")
	if err := (Tailwind{Binary: binary}).Process(input, filepath.Join(blocked, "out.css"), ""); err == nil {
		t.Error("a target under a file must be reported")
	}
}

func TestTailwindReportsACacheItCannotCreate(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Tailwind{CacheDir: blocked, Fetch: func(string, string, string) error { return nil }}.Install()
	if err == nil {
		t.Error("a cache directory that cannot be created must be reported")
	}
}

func TestTailwindWithoutMinifyOmitsTheFlag(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "app.css")
	if err := os.WriteFile(input, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	output := filepath.Join(dir, "out.css")
	binary := fakeTailwind(t, `echo "$@" > "$4"`)
	if err := (Tailwind{Binary: binary}).Process(input, output, ""); err != nil {
		t.Fatalf("Process: %v", err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(got), "--minify") {
		t.Errorf("args = %q, want no minify flag", got)
	}
}

func TestTheCacheDirectoryIsHonoured(t *testing.T) {
	cache := t.TempDir()
	path, err := Tailwind{CacheDir: cache}.path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if !strings.HasPrefix(path, cache) {
		t.Errorf("path = %q, want it under %q", path, cache)
	}
}

func TestCommonRootWalksUpUntilItContains(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if got := commonRoot(deep, filepath.Join(deep, "d")); got != deep {
		t.Errorf("commonRoot = %q, want %q", got, deep)
	}
	want := filepath.Join(root, "a")
	if got := commonRoot(deep, filepath.Join(want, "x")); got != want {
		t.Errorf("commonRoot = %q, want %q", got, want)
	}
}

func TestTheCachedBinaryCarriesAnExtensionWhereWindowsNeedsOne(t *testing.T) {
	if got := binaryName("windows"); got != "tailwindcss.exe" {
		t.Errorf("binaryName(windows) = %q", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := binaryName(goos); got != "tailwindcss" {
			t.Errorf("binaryName(%s) = %q", goos, got)
		}
	}
}

func TestEveryPlatformCarriesAPinnedDigest(t *testing.T) {
	for pair, build := range assets {
		if len(build.Digest) != 64 {
			t.Errorf("%s: digest = %q, want a sha256", pair, build.Digest)
			continue
		}
		for _, letter := range build.Digest {
			if !strings.ContainsRune("0123456789abcdef", letter) {
				t.Errorf("%s: digest = %q, want lowercase hex", pair, build.Digest)
				break
			}
		}
	}
}
