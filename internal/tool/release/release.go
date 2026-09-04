package release

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/apptivitypl/gopage/internal/tool/npmpkg"
)

type Kind string

const (
	KindGo  Kind = "go"
	KindNPM Kind = "npm"
)

const Module = "gopage"

var semantic = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)

type Artifact struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
}

func Artifacts() []Artifact {
	return []Artifact{
		{Name: Module, Kind: KindGo},
		{Name: npmpkg.CLI, Kind: KindNPM},
	}
}

func Valid(version string) error {
	if !semantic.MatchString(version) {
		return fmt.Errorf("%q is not a semantic version, for example 0.2.2", version)
	}
	return nil
}

func (a Artifact) Tag(version string) string {
	if a.Kind == KindGo {
		return "v" + version
	}
	return a.Name + "@" + version
}

type Lookup func(Artifact) (bool, error)

type Status struct {
	Artifact
	Published bool `json:"published"`
}

func Plan(version string, published Lookup) ([]Status, error) {
	if err := Valid(version); err != nil {
		return nil, err
	}
	statuses := make([]Status, 0, len(Artifacts()))
	for _, artifact := range Artifacts() {
		out, err := published(artifact)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, Status{Artifact: artifact, Published: out})
	}
	return statuses, nil
}

func Pending(statuses []Status) []Artifact {
	pending := make([]Artifact, 0, len(statuses))
	for _, status := range statuses {
		if !status.Published {
			pending = append(pending, status.Artifact)
		}
	}
	return pending
}

func Render(version string, statuses []Status) string {
	width := 0
	for _, status := range statuses {
		if len(status.Name) > width {
			width = len(status.Name)
		}
	}
	var b strings.Builder
	for _, status := range statuses {
		state := "publish"
		if status.Published {
			state = "already out"
		}
		fmt.Fprintf(&b, "%-*s  %-4s  %-8s  %s\n", width, status.Name, status.Kind, version, state)
	}
	return b.String()
}
