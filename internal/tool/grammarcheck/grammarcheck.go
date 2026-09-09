package grammarcheck

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const (
	Path         = "editors/vscode/syntaxes/gopage.tmLanguage.json"
	ManifestPath = "editors/vscode/package.json"
	SnippetsPath = "editors/vscode/snippets/gopage.code-snippets"
	Unreleased   = "0.0.0"
)

type IssueKind string

const (
	Missing   IssueKind = "missing"
	Extra     IssueKind = "extra"
	Malformed IssueKind = "malformed"
	Versioned IssueKind = "versioned"
	Unwritten IssueKind = "unwritten"
)

type Issue struct {
	Kind   IssueKind
	Rule   string
	Symbol string
}

func (i Issue) Message() string {
	switch i.Kind {
	case Missing:
		return fmt.Sprintf("%s: the compiler knows %q and the grammar does not", i.Rule, i.Symbol)
	case Extra:
		return fmt.Sprintf("%s: the grammar knows %q and the compiler does not", i.Rule, i.Symbol)
	case Unwritten:
		return fmt.Sprintf("%s: no snippet writes {%% %s %%}", SnippetsPath, i.Symbol)
	case Versioned:
		return fmt.Sprintf("%s: version is %q, want %q; the version a release carries lives in the tag",
			ManifestPath, i.Symbol, Unreleased)
	default:
		return fmt.Sprintf("%s: %s", i.Rule, i.Symbol)
	}
}

type rule struct {
	Match string `json:"match"`
}

type grammar struct {
	Repository map[string]rule `json:"repository"`
}

var alternation = regexp.MustCompile(`\(([A-Za-z][A-Za-z|]*)\)`)

func Check(source []byte, want map[string][]string) ([]Issue, error) {
	var parsed grammar
	if err := json.Unmarshal(source, &parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", Path, err)
	}
	issues := make([]Issue, 0)
	for _, name := range sorted(want) {
		issues = append(issues, compare(parsed, name, want[name])...)
	}
	return issues, nil
}

func CheckManifest(source []byte) ([]Issue, error) {
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(source, &manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestPath, err)
	}
	if manifest.Version == Unreleased {
		return nil, nil
	}
	return []Issue{{Kind: Versioned, Rule: ManifestPath, Symbol: manifest.Version}}, nil
}

func compare(parsed grammar, name string, want []string) []Issue {
	found, ok := parsed.Repository[name]
	if !ok {
		return []Issue{{Kind: Malformed, Rule: name, Symbol: "the grammar has no rule by this name"}}
	}
	match := alternation.FindStringSubmatch(found.Match)
	if match == nil {
		return []Issue{{Kind: Malformed, Rule: name,
			Symbol: fmt.Sprintf("%q is not an alternation of names", found.Match)}}
	}
	have := strings.Split(match[1], "|")
	var issues []Issue
	for _, symbol := range want {
		if !slices.Contains(have, symbol) {
			issues = append(issues, Issue{Kind: Missing, Rule: name, Symbol: symbol})
		}
	}
	for _, symbol := range have {
		if !slices.Contains(want, symbol) {
			issues = append(issues, Issue{Kind: Extra, Rule: name, Symbol: symbol})
		}
	}
	return issues
}

func sorted(want map[string][]string) []string {
	names := make([]string, 0, len(want))
	for name := range want {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type snippet struct {
	Prefix string   `json:"prefix"`
	Body   []string `json:"body"`
}

func CheckSnippets(source []byte, directives []string) ([]Issue, error) {
	var snippets map[string]snippet
	if err := json.Unmarshal(source, &snippets); err != nil {
		return nil, fmt.Errorf("%s: %w", SnippetsPath, err)
	}
	var written strings.Builder
	for _, entry := range snippets {
		for _, line := range entry.Body {
			written.WriteString(line)
			written.WriteByte('\n')
		}
	}
	body := written.String()
	var issues []Issue
	for _, directive := range directives {
		if !strings.Contains(body, "{% "+directive+" ") {
			issues = append(issues, Issue{Kind: Unwritten, Rule: SnippetsPath, Symbol: directive})
		}
	}
	sort.Slice(issues, func(a, b int) bool { return issues[a].Symbol < issues[b].Symbol })
	return issues, nil
}
