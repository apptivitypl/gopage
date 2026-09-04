package gotoolchain

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func absent(string) (string, error) {
	return "", errors.New("not on PATH")
}

func planted(t *testing.T, cache string) string {
	t.Helper()
	binary := filepath.Join(cache, "go", Version, "go", "bin", "go")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(binary, nil, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	return binary
}

func TestEveryPlatformCarriesAPinnedDigest(t *testing.T) {
	for pair, build := range assets {
		if len(build.Digest) != 64 {
			t.Errorf("%s: digest = %q, want a sha256", pair, build.Digest)
			continue
		}
		if strings.ToLower(build.Digest) != build.Digest {
			t.Errorf("%s: digest = %q, want lowercase hex", pair, build.Digest)
		}
		if !strings.HasPrefix(build.Name, Version+".") {
			t.Errorf("%s: name = %q, want it to carry %s", pair, build.Name, Version)
		}
		if build.Size == "" {
			t.Errorf("%s: the size the user is told about is missing", pair)
		}
	}
}

func TestAssetRefusesAPlatformGoDoesNotShip(t *testing.T) {
	if _, err := Asset("linux", "amd64"); err != nil {
		t.Fatalf("Asset: %v", err)
	}
	if _, err := Asset("plan9", "mips"); err == nil {
		t.Error("a platform go does not ship must be refused")
	}
}

func TestAnUnsetToolchainIsThePlainCommand(t *testing.T) {
	if got := (Resolved{}).Command(); got != "go" {
		t.Errorf("Command() = %q", got)
	}
	if got := (Resolved{Path: "/opt/go/bin/go"}).Command(); got != "/opt/go/bin/go" {
		t.Errorf("Command() = %q", got)
	}
}

func TestAnExplicitPathWins(t *testing.T) {
	t.Setenv(Override, "")
	tool, err := Toolchain{Path: "/opt/go/bin/go", Lookup: absent}.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tool.Path != "/opt/go/bin/go" || tool.Env != nil {
		t.Errorf("tool = %+v, want the named binary and no environment of its own", tool)
	}
}

func TestTheEnvironmentOverrideIsHonoured(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(binary, nil, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv(Override, binary)
	tool, err := Toolchain{Lookup: absent}.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tool.Path != binary {
		t.Errorf("path = %q, want %q", tool.Path, binary)
	}
}

func TestTheEnvironmentOverrideMustExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "go")
	t.Setenv(Override, missing)
	_, err := Toolchain{Lookup: absent}.Resolve()
	if err == nil || !strings.Contains(err.Error(), Override) {
		t.Errorf("err = %v, want %s named", err, Override)
	}
}

func TestAToolchainOnThePathIsUsedAsItIs(t *testing.T) {
	t.Setenv(Override, "")
	tool, err := Toolchain{Lookup: func(string) (string, error) { return "/usr/bin/go", nil }}.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tool.Path != "/usr/bin/go" {
		t.Errorf("path = %q, want the one on PATH", tool.Path)
	}
	if tool.Env != nil {
		t.Errorf("env = %v, want a toolchain gopage did not fetch left alone", tool.Env)
	}
}

func TestTheCachedToolchainIsUsedWithoutFetchingAgain(t *testing.T) {
	t.Setenv(Override, "")
	cache := t.TempDir()
	binary := planted(t, cache)
	tool, err := Toolchain{CacheDir: cache, Lookup: absent}.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tool.Path != binary {
		t.Errorf("path = %q, want %q", tool.Path, binary)
	}
	if len(tool.Env) != 1 || tool.Env[0] != "GOTOOLCHAIN=local" {
		t.Errorf("env = %v, want the fetched toolchain pinned to itself", tool.Env)
	}
}

func TestAMissingToolchainWithNoWayToFetchExplainsItself(t *testing.T) {
	t.Setenv(Override, "")
	_, err := Toolchain{CacheDir: t.TempDir(), Lookup: absent}.Resolve()
	if err == nil {
		t.Fatal("a toolchain gopage cannot find or fetch must be reported")
	}
	for _, want := range []string{Override, "go.dev"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q in it", err, want)
		}
	}
}

func TestAPlatformGoDoesNotShipIsReportedBeforeFetching(t *testing.T) {
	t.Setenv(Override, "")
	_, err := Toolchain{CacheDir: t.TempDir(), GOOS: "plan9", GOARCH: "mips", Lookup: absent}.Resolve()
	if err == nil || !strings.Contains(err.Error(), "plan9") {
		t.Errorf("err = %v, want the platform named", err)
	}
}

func TestTheToolchainIsFetchedOnceAndAnnounced(t *testing.T) {
	t.Setenv(Override, "")
	cache := t.TempDir()
	var said []string
	var asked string
	tool, err := Toolchain{
		CacheDir: cache,
		Lookup:   absent,
		Fetch: func(url, target, digest string) error {
			asked = url
			if digest == "" {
				t.Error("the fetch must be given a digest to check")
			}
			return os.WriteFile(target, []byte("archive"), 0o644)
		},
		Unpack: func(archive, into string) error {
			if _, err := os.Stat(archive); err != nil {
				return err
			}
			binary := filepath.Join(into, "go", "bin", "go")
			if runtime.GOOS == "windows" {
				binary += ".exe"
			}
			if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
				return err
			}
			return os.WriteFile(binary, nil, 0o755)
		},
		Announce: func(message string) { said = append(said, message) },
	}.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := os.Stat(tool.Path); err != nil {
		t.Errorf("the fetched toolchain is not where it was reported: %v", err)
	}
	if !strings.HasPrefix(asked, "https://go.dev/dl/"+Version) {
		t.Errorf("url = %q, want the pinned archive", asked)
	}
	if len(said) != 1 || !strings.Contains(said[0], Version) {
		t.Errorf("announced %v, want one line naming %s", said, Version)
	}
	if _, err := os.Stat(filepath.Join(cache, "go", Version+".partial")); err == nil {
		t.Error("the staging directory must not be left behind")
	}
}

func TestAFailedFetchNamesTheWayOut(t *testing.T) {
	t.Setenv(Override, "")
	_, err := Toolchain{
		CacheDir: t.TempDir(),
		Lookup:   absent,
		Fetch:    func(string, string, string) error { return errors.New("no network") },
		Unpack:   func(string, string) error { return nil },
	}.Resolve()
	if err == nil {
		t.Fatal("a fetch that fails must be reported")
	}
	for _, want := range []string{"no network", Override, "go.dev"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q in it", err, want)
		}
	}
}

func TestAFailedUnpackIsReported(t *testing.T) {
	t.Setenv(Override, "")
	_, err := Toolchain{
		CacheDir: t.TempDir(),
		Lookup:   absent,
		Fetch:    func(_, target, _ string) error { return os.WriteFile(target, nil, 0o644) },
		Unpack:   func(string, string) error { return errors.New("truncated") },
	}.Resolve()
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Errorf("err = %v, want the unpack failure named", err)
	}
}

func TestAnUnpackThatWritesNothingIsReported(t *testing.T) {
	t.Setenv(Override, "")
	_, err := Toolchain{
		CacheDir: t.TempDir(),
		Lookup:   absent,
		Fetch:    func(_, target, _ string) error { return os.WriteFile(target, nil, 0o644) },
		Unpack:   func(string, string) error { return nil },
	}.Resolve()
	if err == nil {
		t.Error("an archive that unpacks to nothing must be reported")
	}
}

func TestACacheItCannotWriteIsReported(t *testing.T) {
	t.Setenv(Override, "")
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Toolchain{
		CacheDir: blocked,
		Lookup:   absent,
		Fetch:    func(string, string, string) error { return nil },
		Unpack:   func(string, string) error { return nil },
	}.Resolve()
	if err == nil {
		t.Error("a cache directory that cannot be created must be reported")
	}
}

func TestTheCacheFallsBackToTheUserDirectory(t *testing.T) {
	root, err := Toolchain{}.root()
	if err != nil {
		t.Skip("this machine has no user cache directory")
	}
	if !strings.HasSuffix(root, filepath.Join("gopage", "go", Version)) {
		t.Errorf("root = %q, want it under the gopage cache", root)
	}
}

func TestTheCachedBinaryCarriesAnExtensionWhereWindowsNeedsOne(t *testing.T) {
	if got := (Toolchain{GOOS: "windows"}).command(); got != "go.exe" {
		t.Errorf("command() = %q", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := (Toolchain{GOOS: goos}).command(); got != "go" {
			t.Errorf("command() on %s = %q", goos, got)
		}
	}
	goos, arch := (Toolchain{}).platform()
	if goos != runtime.GOOS || arch != runtime.GOARCH {
		t.Errorf("platform() = %s/%s, want this machine", goos, arch)
	}
}
