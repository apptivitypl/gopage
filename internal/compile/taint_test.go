package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sonquer/rill/internal/diag"
)

const privateProps = "---\n" + `type Props struct {
	Title   string
	Viewer  Viewer ` + "`rill:\"private\"`" + `
	Session Session
}

type Viewer struct {
	Email string
}

type Session struct {
	Email string ` + "`rill:\"private\"`" + `
	Theme string
}
` + "---\n"

func TestAPrivateReadInACachedFragmentIsRejected(t *testing.T) {
	_, bag := compilePage(t, privateProps+`{% fragment "leaky" cache="5m" %}<p>{{ Session.Email }}</p>{% endfragment %}`)
	if !hasCode(bag, diag.C503) {
		t.Fatalf("diagnostics = %v, want C503", bag.Sorted())
	}
	item := bag.Sorted()[0]
	if item.Span.Start == item.Span.End {
		t.Errorf("span = %+v, want the read pointed at", item.Span)
	}
	if !strings.Contains(item.Help, "leaky") {
		t.Errorf("help = %q, want the fragment named", item.Help)
	}
}

func TestPrivacyTravelsThroughTheWholePath(t *testing.T) {
	_, bag := compilePage(t, privateProps+`{% fragment "leaky" cache="5m" %}<p>{{ Viewer.Email }}</p>{% endfragment %}`)
	if !hasCode(bag, diag.C503) {
		t.Errorf("a private struct makes every field under it private: %v", bag.Sorted())
	}
}

func TestAnUncachedFragmentMayReadPrivateValues(t *testing.T) {
	_, bag := compilePage(t, privateProps+`{% fragment "live" %}<p>{{ Session.Email }}</p>{% endfragment %}`)
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v, want none", bag.Sorted())
	}
}

func TestAPublicReadInACachedFragmentIsFine(t *testing.T) {
	_, bag := compilePage(t, privateProps+`{% fragment "ok" cache="5m" %}<p>{{ Title }} {{ Session.Theme }}</p>{% endfragment %}`)
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v, want none", bag.Sorted())
	}
}

func TestPrivateValuesOutsideAFragmentAreFine(t *testing.T) {
	_, bag := compilePage(t, privateProps+`<p>{{ Session.Email }}</p>{% fragment "ok" cache="5m" %}<p>{{ Title }}</p>{% endfragment %}`)
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v, want none", bag.Sorted())
	}
}

func TestPrivacyIsCheckedInEveryPositionOfAFragment(t *testing.T) {
	sources := map[string]string{
		"interpolation": `{% fragment "a" cache="1m" %}{{ Session.Email }}{% endfragment %}`,
		"condition":     `{% fragment "a" cache="1m" %}{% if Session.Email %}x{% endif %}{% endfragment %}`,
		"let":           `{% fragment "a" cache="1m" %}{% let e = Session.Email %}{{ e }}{% endfragment %}`,
		"attribute":     `{% fragment "a" cache="1m" %}<p :title="Session.Email">x</p>{% endfragment %}`,
		"filter":        `{% fragment "a" cache="1m" %}{{ Session.Email | upper }}{% endfragment %}`,
		"binary":        `{% fragment "a" cache="1m" %}{{ Title + Session.Email }}{% endfragment %}`,
		"nested":        `{% fragment "a" cache="1m" %}{% fragment "b" %}{{ Session.Email }}{% endfragment %}{% endfragment %}`,
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			_, bag := compilePage(t, privateProps+source)
			if !hasCode(bag, diag.C503) {
				t.Errorf("diagnostics = %v, want C503", bag.Sorted())
			}
		})
	}
}

func TestALoopVariableIsNotAPropsPath(t *testing.T) {
	source := "---\n" + `type Props struct {
	Session []Entry
}

type Entry struct {
	Email string
}
` + "---\n" + `{% fragment "a" cache="1m" %}{% for Session in Session %}{{ Session.Email }}{% endfor %}{% endfragment %}`
	_, bag := compilePage(t, source)
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v, want none", bag.Sorted())
	}
}

func TestATemplateWithoutASchemaIsNotChecked(t *testing.T) {
	_, bag := compilePage(t, `{% fragment "a" cache="1m" %}<p>hello</p>{% endfragment %}`)
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v, want none", bag.Sorted())
	}
}

func islandLeak(t *testing.T, page string) *diag.Bag {
	t.Helper()
	files := islandProject(privateProps + page)
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return &bag
}

func TestAPrivateValueHandedToAnIslandIsRejected(t *testing.T) {
	bag := islandLeak(t, `<Search client="load" :Placeholder="Session.Email" />`)
	if !hasCode(bag, diag.C320) {
		t.Fatalf("diagnostics = %v, want C320", bag.Sorted())
	}
	item := coded(t, bag, diag.C320)
	if item.Span.Start == item.Span.End {
		t.Errorf("span = %+v, want the read pointed at", item.Span)
	}
	if !strings.Contains(item.Help, "Search") {
		t.Errorf("help = %q, want the island named", item.Help)
	}
}

func coded(t *testing.T, bag *diag.Bag, code diag.Code) diag.Diagnostic {
	t.Helper()
	for _, item := range bag.Sorted() {
		if item.Code == code {
			return item
		}
	}
	t.Fatalf("diagnostics = %v, want %s", bag.Sorted(), code)
	return diag.Diagnostic{}
}

func TestPrivacyReachesAnIslandThroughAWholeStruct(t *testing.T) {
	bag := islandLeak(t, `<Search client="load" :Placeholder="Viewer.Email" />`)
	if !hasCode(bag, diag.C320) {
		t.Errorf("a private struct taints every field under it: %v", bag.Sorted())
	}
}

func TestAPrivateValueParkedInALetStillCannotReachAnIsland(t *testing.T) {
	bag := islandLeak(t, `{% let held = Session.Email %}<Search client="load" :Placeholder="held" />`)
	if !hasCode(bag, diag.C320) {
		t.Errorf("a local must not launder a private value: %v", bag.Sorted())
	}
}

func TestAPublicValueReachesAnIslandFreely(t *testing.T) {
	bag := islandLeak(t, `<Search client="load" :Placeholder="Title" />`)
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v, want none", bag.Sorted())
	}
}

func TestAPrivateValueOnAPlainComponentIsFine(t *testing.T) {
	bag := islandLeak(t, `<Badge :Label="Session.Email" />`)
	if bag.HasErrors() {
		t.Errorf("a component that ships no javascript keeps its props on the server: %v", bag.Sorted())
	}
}

func TestAnIslandDeclaringAPrivatePropIsRejected(t *testing.T) {
	files := islandProject(`<Search client="load" Placeholder="find" />`)
	files["components/Search/props.go"] = &fstest.MapFile{Data: []byte(`package search

type Props struct {
	Placeholder string
	Token       string ` + "`rill:\"private\"`" + `
}
`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C320) {
		t.Errorf("diagnostics = %v, want C320", bag.Sorted())
	}
}

func TestAFormInACachedFragmentIsRejected(t *testing.T) {
	bag := islandLeak(t, `{% fragment "signup" cache="5m" %}<Form action="/apply"></Form>{% endfragment %}`)
	if !hasCode(bag, diag.C503) {
		t.Fatalf("diagnostics = %v, want C503", bag.Sorted())
	}
	item := coded(t, bag, diag.C503)
	if !strings.Contains(item.Help, "signup") {
		t.Errorf("help = %q, want the fragment named", item.Help)
	}
}

func TestACsrfTokenInACachedFragmentIsRejected(t *testing.T) {
	bag := islandLeak(t, `{% fragment "signup" cache="5m" %}{{ form.Token }}{% endfragment %}`)
	if !hasCode(bag, diag.C503) {
		t.Errorf("diagnostics = %v, want C503", bag.Sorted())
	}
}

func TestAFlashInACachedFragmentIsRejected(t *testing.T) {
	bag := islandLeak(t, `{% fragment "banner" cache="5m" %}<p>{{ flash }}</p>{% endfragment %}`)
	if !hasCode(bag, diag.C503) {
		t.Errorf("diagnostics = %v, want C503", bag.Sorted())
	}
}

func TestAnUncachedFragmentMayCarryAForm(t *testing.T) {
	bag := islandLeak(t, `{% fragment "signup" %}<Form action="/apply"></Form>{{ flash }}{% endfragment %}`)
	if hasCode(bag, diag.C503) {
		t.Errorf("diagnostics = %v, want nothing rejected without cache=", bag.Sorted())
	}
}

func TestALocalNamedLikeARootIsNotTheRoot(t *testing.T) {
	bag := islandLeak(t, `{% fragment "banner" cache="5m" %}{% let flash = "hi" %}<p>{{ flash }}</p>{% endfragment %}`)
	if hasCode(bag, diag.C503) {
		t.Errorf("diagnostics = %v, want the local to shadow the root", bag.Sorted())
	}
}
