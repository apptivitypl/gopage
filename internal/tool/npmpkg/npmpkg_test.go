package npmpkg

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func decode(t *testing.T, composed func() ([]byte, error)) map[string]any {
	t.Helper()
	data, err := composed()
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("the manifest is not valid JSON: %v", err)
	}
	return entry
}

func TestEveryPlatformGoreleaserBuildsIsCovered(t *testing.T) {
	platforms := Platforms()
	if len(platforms) != 6 {
		t.Fatalf("platforms = %d, want the six goreleaser builds", len(platforms))
	}
	for _, platform := range platforms {
		if platform.GOOS == "windows" {
			if platform.Binary() != "gopage.exe" || !strings.HasSuffix(platform.Archive("0.1.0"), ".zip") {
				t.Errorf("windows carries %q in %q", platform.Binary(), platform.Archive("0.1.0"))
			}
			continue
		}
		if platform.Binary() != "gopage" || !strings.HasSuffix(platform.Archive("0.1.0"), ".tar.gz") {
			t.Errorf("%s carries %q in %q", platform.GOOS, platform.Binary(), platform.Archive("0.1.0"))
		}
	}
}

func TestArchiveNamesMatchTheGoreleaserTemplate(t *testing.T) {
	platform := Platform{GOOS: "darwin", GOARCH: "arm64", OS: "darwin", CPU: "arm64"}
	if got := platform.Archive("0.1.0"); got != "gopage_0.1.0_darwin_arm64.tar.gz" {
		t.Errorf("Archive = %q", got)
	}
	if got := platform.Package(); got != "@apptivitypl/gopage-darwin-arm64" {
		t.Errorf("Package = %q", got)
	}
}

func TestTheLauncherAsksForEveryPlatformOptionally(t *testing.T) {
	entry := decode(t, func() ([]byte, error) { return Launcher("0.1.0") })
	optional, ok := entry["optionalDependencies"].(map[string]any)
	if !ok || len(optional) != len(Platforms()) {
		t.Fatalf("optionalDependencies = %v", entry["optionalDependencies"])
	}
	for name, version := range optional {
		if version != "0.1.0" {
			t.Errorf("%s is pinned to %v, want the same version", name, version)
		}
	}
	if _, ok := entry["dependencies"]; ok {
		t.Error("the launcher must not depend on anything that has to resolve")
	}
	if entry["bin"].(map[string]any)["gopage"] != "bin/gopage.js" {
		t.Errorf("bin = %v", entry["bin"])
	}
}

func TestABinaryPackageIsPinnedToItsPlatform(t *testing.T) {
	entry := decode(t, func() ([]byte, error) {
		return Binary("0.1.0", Platform{GOOS: "windows", GOARCH: "arm64", OS: "win32", CPU: "arm64"})
	})
	if entry["name"] != "@apptivitypl/gopage-win32-arm64" {
		t.Errorf("name = %v", entry["name"])
	}
	if !slices.Contains(entry["os"].([]any), any("win32")) {
		t.Errorf("os = %v", entry["os"])
	}
	if !slices.Contains(entry["cpu"].([]any), any("arm64")) {
		t.Errorf("cpu = %v", entry["cpu"])
	}
	if entry["bin"].(map[string]any)["gopage-win32-arm64"] != "bin/gopage.exe" {
		t.Errorf("bin = %v", entry["bin"])
	}
}

func TestManifestPicksTheRightShape(t *testing.T) {
	for _, name := range Names() {
		entry := decode(t, func() ([]byte, error) { return Manifest(name, "0.1.0") })
		if entry["name"] != name {
			t.Errorf("Manifest(%q) named itself %v", name, entry["name"])
		}
		if entry["license"] != License || entry["version"] != "0.1.0" {
			t.Errorf("Manifest(%q) = %v", name, entry)
		}
	}
	if _, err := Manifest("@apptivitypl/gopage-plan9-mips", "0.1.0"); err == nil {
		t.Error("Manifest should refuse a package it does not know")
	}
}

func TestNamesListsEveryPublishedPackage(t *testing.T) {
	names := Names()
	if len(names) != len(Platforms())+1 {
		t.Fatalf("names = %v", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("names = %v, want them sorted", names)
	}
}
