package release

import (
	"errors"
	"strings"
	"testing"
)

func TestEveryPublishableArtifactIsNamed(t *testing.T) {
	artifacts := Artifacts()
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want the go module and the npm command line", artifacts)
	}
	if artifacts[0].Kind != KindGo || artifacts[1].Kind != KindNPM {
		t.Errorf("artifacts = %+v, want the go module first so it is tagged before npm resolves it", artifacts)
	}
}

func TestTheTagSaysWhichRegistryItBelongsTo(t *testing.T) {
	artifacts := Artifacts()
	if got := artifacts[0].Tag("0.2.2"); got != "v0.2.2" {
		t.Errorf("go tag = %q, want the form go modules resolve through", got)
	}
	if got := artifacts[1].Tag("0.2.2"); got != artifacts[1].Name+"@0.2.2" {
		t.Errorf("npm tag = %q", got)
	}
}

func TestOnlySemanticVersionsAreAccepted(t *testing.T) {
	for _, version := range []string{"0.2.2", "1.0.0", "0.3.0-rc.1"} {
		if err := Valid(version); err != nil {
			t.Errorf("Valid(%q) = %v, want it accepted", version, err)
		}
	}
	for _, version := range []string{"", "v0.2.2", "0.2", "latest", "0.2.2+dirty"} {
		if err := Valid(version); err == nil {
			t.Errorf("Valid(%q) accepted something that is not a version", version)
		}
	}
}

func TestPlanAsksAboutEveryArtifact(t *testing.T) {
	var asked []string
	statuses, err := Plan("0.2.2", func(a Artifact) (bool, error) {
		asked = append(asked, a.Name)
		return a.Kind == KindGo, nil
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(asked) != 2 {
		t.Errorf("asked = %v, want every artifact", asked)
	}
	pending := Pending(statuses)
	if len(pending) != 1 || pending[0].Kind != KindNPM {
		t.Errorf("pending = %+v, want only what is not out yet", pending)
	}
}

func TestPlanRefusesAVersionItCannotTrust(t *testing.T) {
	if _, err := Plan("latest", func(Artifact) (bool, error) { return false, nil }); err == nil {
		t.Error("a version that is not semantic must not reach the registries")
	}
}

func TestPlanReportsWhatItCannotAsk(t *testing.T) {
	_, err := Plan("0.2.2", func(Artifact) (bool, error) { return false, errors.New("the registry is down") })
	if err == nil {
		t.Fatal("a registry that will not answer must not read as published")
	}
}

func TestRenderNamesEveryArtifactAndItsState(t *testing.T) {
	statuses, err := Plan("0.2.2", func(a Artifact) (bool, error) { return a.Kind == KindGo, nil })
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	out := Render("0.2.2", statuses)
	for _, want := range []string{Module, "already out", "publish", "0.2.2"} {
		if !strings.Contains(out, want) {
			t.Errorf("Render = %q, want it to mention %q", out, want)
		}
	}
}
