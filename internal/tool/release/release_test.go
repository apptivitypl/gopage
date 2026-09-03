package release

import (
	"errors"
	"strings"
	"testing"
)

const oneGoPackage = `{
  "packages": {
    "rill": {
      "kind": "go",
      "version": "0.1.0",
      "tag": "v{version}",
      "paths": ["*.go", "internal/**"],
      "exclude": ["internal/tool/**"]
    }
  }
}`

func parse(t *testing.T, text string) *Manifest {
	t.Helper()
	manifest, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return manifest
}

func parseErr(t *testing.T, text string) error {
	t.Helper()
	if _, err := Parse(text); err != nil {
		return err
	}
	t.Fatal("expected an error, got none")
	return nil
}

func packaged(t *testing.T, manifest *Manifest, name string) Package {
	t.Helper()
	pkg, ok := manifest.Package(name)
	if !ok {
		t.Fatalf("no package %q", name)
	}
	return pkg
}

func always(value bool) Lookup {
	return func(Package) (bool, error) { return value, nil }
}

func changing(value bool) Changes {
	return func(Package) (bool, error) { return value, nil }
}

func TestAGoPackageParses(t *testing.T) {
	pkg := packaged(t, parse(t, oneGoPackage), "rill")
	if pkg.Kind != KindGo {
		t.Errorf("Kind = %q, want %q", pkg.Kind, KindGo)
	}
	if pkg.Version != "0.1.0" {
		t.Errorf("Version = %q", pkg.Version)
	}
	if pkg.TagName() != "v0.1.0" {
		t.Errorf("TagName = %q, want v0.1.0", pkg.TagName())
	}
}

func TestATagTemplateDefaultsToNameAndVersion(t *testing.T) {
	manifest := parse(t, `{"packages": {"@apptivitypl/rill": {"kind": "npm", "version": "1.2.3", "dir": "npm/rill", "paths": ["npm/rill/**"]}}}`)
	if got := packaged(t, manifest, "@apptivitypl/rill").TagName(); got != "@apptivitypl/rill@1.2.3" {
		t.Errorf("TagName = %q", got)
	}
}

func TestPrereleaseVersionsAreAllowed(t *testing.T) {
	parse(t, `{"packages": {"rill": {"kind": "go", "version": "0.1.0-rc.1", "paths": ["*.go"]}}}`)
}

func TestAnUnknownKindIsRefused(t *testing.T) {
	err := parseErr(t, `{"packages": {"rill": {"kind": "deb", "version": "0.1.0", "paths": ["*.go"]}}}`)
	if !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("error = %q", err)
	}
}

func TestAVersionThatIsNotSemanticIsRefused(t *testing.T) {
	err := parseErr(t, `{"packages": {"rill": {"kind": "go", "version": "v0.1", "paths": ["*.go"]}}}`)
	if !strings.Contains(err.Error(), "semantic version") {
		t.Errorf("error = %q", err)
	}
}

func TestAPackageWithoutPathsIsRefused(t *testing.T) {
	err := parseErr(t, `{"packages": {"rill": {"kind": "go", "version": "0.1.0", "paths": []}}}`)
	if !strings.Contains(err.Error(), "owns no paths") {
		t.Errorf("error = %q", err)
	}
}

func TestAnInvalidPatternIsRefused(t *testing.T) {
	err := parseErr(t, `{"packages": {"rill": {"kind": "go", "version": "0.1.0", "paths": ["["]}}}`)
	if !strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("error = %q", err)
	}
}

func TestAPackageThatIsNotGoNeedsADir(t *testing.T) {
	err := parseErr(t, `{"packages": {"@apptivitypl/rill": {"kind": "npm", "version": "0.1.0", "paths": ["npm/**"]}}}`)
	if !strings.Contains(err.Error(), "needs a dir") {
		t.Errorf("error = %q", err)
	}
}

func TestAnEmptyManifestIsRefused(t *testing.T) {
	err := parseErr(t, `{"packages": {}}`)
	if !strings.Contains(err.Error(), "no packages") {
		t.Errorf("error = %q", err)
	}
}

func TestBrokenJsonIsRefused(t *testing.T) {
	if _, err := Parse("/* never closed"); err == nil {
		t.Error("expected an error from an unterminated comment")
	}
	if _, err := Parse("{"); err == nil {
		t.Error("expected an error from truncated json")
	}
	if _, err := Parse(`{"packages": {"rill": {"kind": "go", "version": "0.1.0", "paths": ["*.go"], "what": 1}}}`); err == nil {
		t.Error("expected an error from an unknown field")
	}
	if _, err := Parse(`{"packages": []}`); err == nil {
		t.Error("expected an error from the wrong shape")
	}
}

func TestANeedThatIsNotInTheManifestIsRefused(t *testing.T) {
	err := parseErr(t, `{"packages": {"@apptivitypl/rill": {"kind": "npm", "version": "0.1.0", "dir": "npm/rill", "needs": ["rill"], "paths": ["npm/rill/**"]}}}`)
	if !strings.Contains(err.Error(), "not in the manifest") {
		t.Errorf("error = %q", err)
	}
}

func TestACycleInNeedsIsRefused(t *testing.T) {
	err := parseErr(t, `{"packages": {
    "a": {"kind": "npm", "version": "0.1.0", "dir": "npm/a", "needs": ["b"], "paths": ["npm/a/**"]},
    "b": {"kind": "npm", "version": "0.1.0", "dir": "npm/b", "needs": ["a"], "paths": ["npm/b/**"]}
  }}`)
	if !strings.Contains(err.Error(), "needs itself") {
		t.Errorf("error = %q", err)
	}
}

func TestPackagesComeOutInDependencyOrder(t *testing.T) {
	manifest := parse(t, `{"packages": {
    "create-rill": {"kind": "npm", "version": "0.1.0", "dir": "npm/create-rill", "needs": ["@apptivitypl/rill"], "paths": ["npm/create-rill/**"]},
    "@apptivitypl/rill": {"kind": "npm", "version": "0.1.0", "dir": "npm/rill", "needs": ["rill"], "paths": ["npm/rill/**"]},
    "rill": {"kind": "go", "version": "0.1.0", "paths": ["*.go"]}
  }}`)
	var names []string
	for _, pkg := range manifest.Packages() {
		names = append(names, pkg.Name)
	}
	want := []string{"rill", "@apptivitypl/rill", "create-rill"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", names, want)
	}
}

func TestAMissingPackageIsReported(t *testing.T) {
	if _, ok := parse(t, oneGoPackage).Package("nothing"); ok {
		t.Error("Package should not invent entries")
	}
}

func TestOwnershipFollowsPathsAndExcludes(t *testing.T) {
	pkg := packaged(t, parse(t, oneGoPackage), "rill")
	for _, path := range []string{"rill.go", "./serve.go", "internal/build/build.go"} {
		if !pkg.Owns(path) {
			t.Errorf("Owns(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"internal/tool/release/release.go", "README.md", "cmd/rill/main.go"} {
		if pkg.Owns(path) {
			t.Errorf("Owns(%q) = true, want false", path)
		}
	}
}

func TestAnUnpublishedVersionIsPlannedForPublication(t *testing.T) {
	statuses, err := Plan(parse(t, oneGoPackage), always(false), changing(false))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if statuses[0].State != Publish {
		t.Errorf("State = %q, want %q", statuses[0].State, Publish)
	}
	if len(Pending(statuses)) != 1 {
		t.Errorf("Pending = %d, want 1", len(Pending(statuses)))
	}
	if len(Problems(statuses)) != 0 {
		t.Errorf("Problems = %v, want none", Problems(statuses))
	}
}

func TestAPublishedVersionWithoutChangesIsCurrent(t *testing.T) {
	statuses, err := Plan(parse(t, oneGoPackage), always(true), changing(false))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if statuses[0].State != Current {
		t.Errorf("State = %q, want %q", statuses[0].State, Current)
	}
	if len(Pending(statuses)) != 0 {
		t.Errorf("Pending = %d, want 0", len(Pending(statuses)))
	}
}

func TestChangesOnTopOfAPublishedVersionAreAProblem(t *testing.T) {
	statuses, err := Plan(parse(t, oneGoPackage), always(true), changing(true))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if statuses[0].State != Unreleased {
		t.Errorf("State = %q, want %q", statuses[0].State, Unreleased)
	}
	problems := Problems(statuses)
	if len(problems) != 1 || !strings.Contains(problems[0], "v0.1.0") {
		t.Errorf("Problems = %v", problems)
	}
}

func TestAPackageHeldBackIsNotAProblem(t *testing.T) {
	manifest := parse(t, `{"packages": {"rill": {"kind": "go", "version": "0.1.0", "tag": "v{version}", "paths": ["*.go"], "unreleased": true}}}`)
	statuses, err := Plan(manifest, always(true), changing(true))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if statuses[0].State != Current {
		t.Errorf("State = %q, want %q", statuses[0].State, Current)
	}
}

func TestPlanReportsWhatItCannotAsk(t *testing.T) {
	failure := errors.New("registry is down")
	manifest := parse(t, oneGoPackage)
	if _, err := Plan(manifest, func(Package) (bool, error) { return false, failure }, changing(false)); !errors.Is(err, failure) {
		t.Errorf("err = %v, want the lookup failure", err)
	}
	if _, err := Plan(manifest, always(true), func(Package) (bool, error) { return false, failure }); !errors.Is(err, failure) {
		t.Errorf("err = %v, want the history failure", err)
	}
}

func TestRenderNamesEveryPackage(t *testing.T) {
	statuses, err := Plan(parse(t, oneGoPackage), always(false), changing(false))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	out := Render(statuses)
	if !strings.Contains(out, "rill") || !strings.Contains(out, string(Publish)) {
		t.Errorf("Render = %q", out)
	}
}
