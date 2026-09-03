package main

import (
	"strings"
	"testing"
)

func stamp(t *testing.T, name, pinned string) {
	t.Helper()
	heldVersion, heldModule := version, module
	t.Cleanup(func() { version, module = heldVersion, heldModule })
	version, module = name, pinned
}

func TestAReleasedBinaryPinsTheVersionItWasCutFrom(t *testing.T) {
	stamp(t, "0.1.2", "v0.1.2")
	if got := ownVersion(); got != "v0.1.2" {
		t.Errorf("ownVersion() = %q, want the module version goreleaser stamped", got)
	}
}

func TestANightlyBinaryPinsTheLastRelease(t *testing.T) {
	stamp(t, "nightly", "v0.1.2")
	if got := ownVersion(); got != "v0.1.2" {
		t.Errorf("ownVersion() = %q, want the last release rather than the build's own name", got)
	}
}

func TestAVersionWithoutAModuleStampStillWorks(t *testing.T) {
	stamp(t, "v0.1.2", "")
	if got := ownVersion(); got != "v0.1.2" {
		t.Errorf("ownVersion() = %q, want the stamped version", got)
	}
}

func TestNothingPinnableIsReportedRatherThanGuessed(t *testing.T) {
	for _, name := range []string{"0.1.2", "nightly", "v0.1.3-0.20260903194428-d821507e8c8f+dirty", ""} {
		stamp(t, name, "")
		if got := ownVersion(); got != "" {
			t.Errorf("ownVersion() with version %q = %q, want nothing", name, got)
		}
	}
}

func TestOnlyAFetchableVersionCountsAsPinnable(t *testing.T) {
	for _, name := range []string{"v0.1.2", "v1.0.0-rc.1"} {
		if !pinnable(name) {
			t.Errorf("pinnable(%q) = false", name)
		}
	}
	for _, name := range []string{"", "v", "0.1.2", "nightly", "vnightly", "v0.1.2+dirty", "v0.1.2 "} {
		if pinnable(name) {
			t.Errorf("pinnable(%q) = true", name)
		}
	}
}

func TestVersionPrintsWhatTheBuildWasStampedWith(t *testing.T) {
	stamp(t, "nightly", "v0.1.2")
	heldCommit, heldDate := commit, date
	t.Cleanup(func() { commit, date = heldCommit, heldDate })
	commit, date = "abc123456789", "2026-09-03T19:44:28Z"
	got := release()
	for _, want := range []string{"rill nightly", "abc123456789", "2026-09-03"} {
		if !strings.Contains(got, want) {
			t.Errorf("release() = %q, want %q in it", got, want)
		}
	}
}
