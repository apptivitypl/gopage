package diagcheck

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type IssueKind string

const (
	Undocumented IssueKind = "undocumented"
	Untested     IssueKind = "untested"
	OrphanPage   IssueKind = "orphan"
	Unindexed    IssueKind = "unindexed"
)

type Issue struct {
	Kind IssueKind
	Code string
}

func (i Issue) Message() string {
	switch i.Kind {
	case Undocumented:
		return fmt.Sprintf("%s: no page at docs/errors/%s.md", i.Code, i.Code)
	case Untested:
		return fmt.Sprintf("%s: no test produces this code", i.Code)
	case Unindexed:
		return fmt.Sprintf("%s: docs/errors/README.md does not list it", i.Code)
	default:
		return fmt.Sprintf("%s: docs/errors/%s.md has no matching code in the registry", i.Code, i.Code)
	}
}

func IsCode(token string) bool {
	if len(token) != 4 {
		return false
	}
	if token[0] != 'C' && token[0] != 'W' {
		return false
	}
	for _, c := range token[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func CodesIn(text string) map[string]bool {
	codes := map[string]bool{}
	for _, token := range strings.FieldsFunc(text, isNotAlphanumeric) {
		if IsCode(token) {
			codes[token] = true
		}
	}
	return codes
}

func isNotAlphanumeric(r rune) bool {
	return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
}

func Analyze(declared, documented, tested map[string]bool) []Issue {
	var issues []Issue
	for code := range declared {
		if !documented[code] {
			issues = append(issues, Issue{Undocumented, code})
		}
		if !tested[code] {
			issues = append(issues, Issue{Untested, code})
		}
	}
	for code := range documented {
		if !declared[code] {
			issues = append(issues, Issue{OrphanPage, code})
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Kind < issues[j].Kind
	})
	return issues
}

func Render(issues []Issue) string {
	if len(issues) == 0 {
		return "diag: ok\n"
	}
	var b strings.Builder
	b.WriteString("diag: inconsistencies\n")
	for _, issue := range issues {
		fmt.Fprintf(&b, "  - %s\n", issue.Message())
	}
	return b.String()
}

func Check(root fs.FS) ([]Issue, error) {
	declared, err := declaredCodes(root)
	if err != nil {
		return nil, err
	}
	documented, err := documentedCodes(root)
	if err != nil {
		return nil, err
	}
	tested, err := testedCodes(root)
	if err != nil {
		return nil, err
	}
	issues := Analyze(declared, documented, tested)
	indexed, found := indexedCodes(root)
	if !found {
		return issues, nil
	}
	return append(issues, missingFromIndex(declared, indexed)...), nil
}

func indexedCodes(root fs.FS) (map[string]bool, bool) {
	data, err := fs.ReadFile(root, "docs/errors/README.md")
	if err != nil {
		return nil, false
	}
	return CodesIn(string(data)), true
}

func missingFromIndex(declared, indexed map[string]bool) []Issue {
	var issues []Issue
	for code := range declared {
		if !indexed[code] {
			issues = append(issues, Issue{Kind: Unindexed, Code: code})
		}
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Code < issues[j].Code })
	return issues
}

func declaredCodes(root fs.FS) (map[string]bool, error) {
	data, err := fs.ReadFile(root, "internal/diag/codes.go")
	if err != nil {
		return map[string]bool{}, nil
	}
	return CodesIn(string(data)), nil
}

func documentedCodes(root fs.FS) (map[string]bool, error) {
	codes := map[string]bool{}
	entries, err := fs.ReadDir(root, "docs/errors")
	if err != nil {
		return codes, nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".md" {
			continue
		}
		if stem := strings.TrimSuffix(name, ".md"); IsCode(stem) {
			codes[stem] = true
		}
	}
	return codes, nil
}

func testedCodes(root fs.FS) (map[string]bool, error) {
	codes := map[string]bool{}
	err := fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, readErr := fs.ReadFile(root, path)
		if readErr != nil {
			return readErr
		}
		for code := range CodesIn(string(data)) {
			codes[code] = true
		}
		return nil
	})
	return codes, err
}
