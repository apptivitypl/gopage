package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/runtime"
)

const statusBlock = `---
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

type Props struct {
	State  Status
	Title  string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}
---
`

func matched(t *testing.T, body string) *diag.Bag {
	t.Helper()
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.rill": file(statusBlock + body)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return &bag
}

func matchError(t *testing.T, body string) diag.Diagnostic {
	t.Helper()
	bag := matched(t, body)
	for _, d := range bag.Items() {
		if d.Code == diag.C309 {
			return d
		}
	}
	t.Fatalf("%q was accepted, diagnostics: %+v", body, bag.Items())
	return diag.Diagnostic{}
}

const allArms = `{% match State %}
	{% when StatusActive %}on
	{% when StatusPaused %}paused
	{% when StatusArchived %}gone
{% endmatch %}`

func TestExhaustiveMatchIsAccepted(t *testing.T) {
	if bag := matched(t, allArms); bag.HasErrors() {
		t.Errorf("diagnostics: %+v", bag.Items())
	}
}

func TestMatchPicksTheRightArm(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": file(statusBlock + "{% match State %}{% when StatusActive %}on{% when StatusPaused %}paused{% when StatusArchived %}gone{% endmatch %}")}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	route, _ := result.Manifest.Lookup("/")

	cases := map[string]string{
		"StatusActive":   "on",
		"StatusPaused":   "paused",
		"StatusArchived": "gone",
		"":               "",
	}
	for value, want := range cases {
		out := runtime.NewBuffer(64)
		props := runtime.Map{"State": runtime.String(value)}
		if err := runtime.Render(result.Manifest.Chain(route), props, out); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if out.String() != want {
			t.Errorf("state %q rendered %q, want %q", value, out.String(), want)
		}
	}
}

func TestMissingArmIsReported(t *testing.T) {
	d := matchError(t, "{% match State %}{% when StatusActive %}on{% when StatusPaused %}p{% endmatch %}")
	if !strings.Contains(d.Message, "does not handle StatusArchived") {
		t.Errorf("message = %q", d.Message)
	}
	if !strings.Contains(d.Help, "no fallthrough") {
		t.Errorf("help = %q", d.Help)
	}
}

func TestSeveralMissingArmsAreListed(t *testing.T) {
	d := matchError(t, "{% match State %}{% when StatusActive %}on{% endmatch %}")
	if !strings.Contains(d.Message, "StatusPaused, StatusArchived") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestUnknownArmIsReported(t *testing.T) {
	d := matchError(t, "{% match State %}{% when StatusNope %}x{% endmatch %}")
	if !strings.Contains(d.Message, "not a case of Status") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestMisspelledArmSuggestsTheRealCase(t *testing.T) {
	d := matchError(t, "{% match State %}{% when StatusActiv %}x{% endmatch %}")
	if !strings.Contains(d.Help, "StatusActive") {
		t.Errorf("help = %q", d.Help)
	}
}

func TestDuplicateArmIsReported(t *testing.T) {
	body := "{% match State %}{% when StatusActive %}a{% when StatusActive %}b{% when StatusPaused %}p{% when StatusArchived %}g{% endmatch %}"
	d := matchError(t, body)
	if !strings.Contains(d.Message, "handled twice") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestMatchOnANonEnumIsReported(t *testing.T) {
	d := matchError(t, "{% match Title %}{% when StatusActive %}x{% endmatch %}")
	if !strings.Contains(d.Message, "named constant type") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestTextBetweenMatchAndFirstWhenIsReported(t *testing.T) {
	var bag diag.Bag
	body := "{% match State %}stray{% when StatusActive %}a{% when StatusPaused %}p{% when StatusArchived %}g{% endmatch %}"
	if _, err := Compile(fstest.MapFS{"app/page.rill": file(statusBlock + body)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !bag.HasErrors() {
		t.Error("text outside a when must be reported")
	}
}

func TestWhitespaceBetweenMatchAndFirstWhenIsFine(t *testing.T) {
	if bag := matched(t, allArms); bag.HasErrors() {
		t.Errorf("diagnostics: %+v", bag.Items())
	}
}

func TestUnclosedMatchIsReported(t *testing.T) {
	var bag diag.Bag
	body := "{% match State %}{% when StatusActive %}a"
	if _, err := Compile(fstest.MapFS{"app/page.rill": file(statusBlock + body)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !bag.HasErrors() {
		t.Error("an unclosed match must be reported")
	}
}

func TestMalformedWhenIsReported(t *testing.T) {
	var bag diag.Bag
	body := "{% match State %}{% when %}a{% endmatch %}"
	if _, err := Compile(fstest.MapFS{"app/page.rill": file(statusBlock + body)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !bag.HasErrors() {
		t.Error("a when without a case name must be reported")
	}
}

func TestArmBodiesAreTypeChecked(t *testing.T) {
	body := "{% match State %}{% when StatusActive %}{{ Missing }}{% when StatusPaused %}p{% when StatusArchived %}g{% endmatch %}"
	bag := matched(t, body)
	var found bool
	for _, d := range bag.Items() {
		if d.Code == diag.C305 {
			found = true
		}
	}
	if !found {
		t.Errorf("arm bodies must be checked, diagnostics: %+v", bag.Items())
	}
}

func TestEnumWithoutConstantsIsNotAMatchSubject(t *testing.T) {
	source := "---\ntype Status string\ntype Props struct{ State Status }\n---\n{% match State %}{% when X %}a{% endmatch %}"
	var bag diag.Bag
	if _, err := Compile(fstest.MapFS{"app/page.rill": file(source)}, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !bag.HasErrors() {
		t.Error("an enum with no constants cannot be matched exhaustively")
	}
}
