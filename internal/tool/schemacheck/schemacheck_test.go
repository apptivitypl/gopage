package schemacheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/config"
)

type inner struct {
	Flag bool `json:"flag,omitempty"`
}

type sample struct {
	Name    string  `json:"name,omitempty"`
	Nested  inner   `json:"nested,omitempty"`
	List    []inner `json:"list,omitempty"`
	hidden  string
	Skipped string `json:"-"`
}

const full = `{"type":"object","properties":{
  "name":{"type":"string"},
  "nested":{"type":"object","properties":{"flag":{"type":"boolean"}}},
  "list":{"type":"array","items":{"type":"object","properties":{"flag":{"type":"boolean"}}}}
}}`

func check(t *testing.T, schema string) []Issue {
	t.Helper()
	issues, err := Check([]byte(schema), sample{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return issues
}

func TestAMatchingSchemaHasNoIssues(t *testing.T) {
	if got := check(t, full); len(got) != 0 {
		t.Errorf("issues = %v, want none", got)
	}
}

func TestAFieldMissingFromTheSchemaIsReported(t *testing.T) {
	got := check(t, strings.Replace(full, `"name":{"type":"string"},`, "", 1))
	if len(got) != 1 || got[0].Kind != Undocumented || got[0].Field != "name" {
		t.Fatalf("issues = %v", got)
	}
	if !strings.Contains(got[0].Message(), "not in "+Path) {
		t.Errorf("message = %q", got[0].Message())
	}
}

func TestAPropertyMissingFromTheStructIsReported(t *testing.T) {
	got := check(t, strings.Replace(full, `"name":{"type":"string"},`, `"name":{"type":"string"},"extra":{"type":"string"},`, 1))
	if len(got) != 1 || got[0].Kind != Orphan || got[0].Field != "extra" {
		t.Fatalf("issues = %v", got)
	}
}

func TestAMismatchedTypeIsReported(t *testing.T) {
	got := check(t, strings.Replace(full, `"name":{"type":"string"}`, `"name":{"type":"integer"}`, 1))
	if len(got) != 1 || got[0].Kind != Untyped {
		t.Fatalf("issues = %v", got)
	}
	if got[0].Want != "string" || got[0].Got != "integer" {
		t.Errorf("want %q, got %q", got[0].Want, got[0].Got)
	}
}

func TestNestedObjectsAreWalked(t *testing.T) {
	got := check(t, strings.Replace(full, `"nested":{"type":"object","properties":{"flag":{"type":"boolean"}}}`,
		`"nested":{"type":"object","properties":{}}`, 1))
	if len(got) != 1 || got[0].Field != "nested.flag" {
		t.Fatalf("issues = %v", got)
	}
}

func TestArrayItemsAreWalked(t *testing.T) {
	got := check(t, strings.Replace(full, `"list":{"type":"array","items":{"type":"object","properties":{"flag":{"type":"boolean"}}}}`,
		`"list":{"type":"array","items":{"type":"object","properties":{}}}`, 1))
	if len(got) != 1 || got[0].Field != "list[].flag" {
		t.Fatalf("issues = %v", got)
	}
}

func TestUnexportedAndSkippedFieldsAreIgnored(t *testing.T) {
	issues, err := Check([]byte(full), sample{hidden: "not a config field"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("issues = %v, want the unexported and json:\"-\" fields left alone", issues)
	}
}

func TestABrokenSchemaIsReported(t *testing.T) {
	if _, err := Check([]byte("{not json"), sample{}); err == nil {
		t.Error("a schema that does not parse must be reported")
	}
}

func TestTheShippedSchemaMatchesTheConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(Path)))
	if err != nil {
		t.Fatalf("read %s: %v", Path, err)
	}
	issues, err := Check(data, config.Config{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, issue := range issues {
		t.Errorf("%s", issue.Message())
	}
}
