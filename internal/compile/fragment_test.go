package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/ir"
	"github.com/sonquer/rill/internal/runtime"
)

const fragmentPage = `---
type Props struct {
	ID    string
	Title string
	Note  string
}
---
<h1>{{ Title }}</h1>
{% fragment "reviews" cache="5m" stale="1h" %}<p>{{ Note }} for {{ ID }}</p>{% endfragment %}
<footer>{{ Title }}</footer>`

func compilePage(t *testing.T, source string) (Result, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": &fstest.MapFile{Data: []byte(source)}}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return result, &bag
}

func TestAFragmentCarriesItsWindowAndReads(t *testing.T) {
	result, bag := compilePage(t, fragmentPage)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	plan := result.Manifest.Plans[0]
	if len(plan.Fragments) != 1 {
		t.Fatalf("fragments = %+v", plan.Fragments)
	}
	fragment := plan.Fragments[0]
	if fragment.Name != "reviews" || fragment.TTL != int64(5*60)*1e9 || fragment.Stale != int64(3600)*1e9 {
		t.Errorf("fragment = %+v", fragment)
	}
	var read []string
	for _, index := range fragment.Paths {
		read = append(read, strings.Join(plan.Path(index), "."))
	}
	if strings.Join(read, ",") != "Note,ID" {
		t.Errorf("reads = %v, want only what the body touches", read)
	}
}

type keyRecorder struct {
	keys []string
}

func (k *keyRecorder) Load(_ ir.Fragment, key string) ([]byte, bool) {
	k.keys = append(k.keys, key)
	return nil, false
}

func (k *keyRecorder) Save(ir.Fragment, string, []byte) {}

func renderWithHook(t *testing.T, result Result, props runtime.Accessible) (string, []string) {
	t.Helper()
	chain := result.Manifest.Chain(result.Manifest.Routes[0])
	out := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(out)
	hook := &keyRecorder{}
	if err := runtime.RenderWith(chain, props, out, hook); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String(), hook.keys
}

func TestAFragmentRendersInPlaceAndKeysOnItsReads(t *testing.T) {
	result, _ := compilePage(t, fragmentPage)
	props := runtime.Map{"ID": runtime.String("7"), "Title": runtime.String("t"), "Note": runtime.String("n")}
	html, keys := renderWithHook(t, result, props)
	if !strings.Contains(html, "<h1>t</h1>") || !strings.Contains(html, "<p>n for 7</p>") ||
		!strings.Contains(html, "<footer>t</footer>") {
		t.Errorf("html = %q", html)
	}
	if len(keys) != 1 || !strings.HasPrefix(keys[0], "reviews") || !strings.Contains(keys[0], "n") {
		t.Errorf("keys = %q", keys)
	}
	other := runtime.Map{"ID": runtime.String("8"), "Title": runtime.String("t"), "Note": runtime.String("n")}
	_, second := renderWithHook(t, result, other)
	if keys[0] == second[0] {
		t.Errorf("a different read must change the key: %q", keys[0])
	}
}

func TestTheKeyIgnoresWhatTheFragmentDoesNotRead(t *testing.T) {
	result, _ := compilePage(t, fragmentPage)
	first := runtime.Map{"ID": runtime.String("7"), "Title": runtime.String("one"), "Note": runtime.String("n")}
	second := runtime.Map{"ID": runtime.String("7"), "Title": runtime.String("two"), "Note": runtime.String("n")}
	_, a := renderWithHook(t, result, first)
	_, b := renderWithHook(t, result, second)
	if a[0] != b[0] {
		t.Errorf("keys %q and %q differ on a value the fragment never reads", a[0], b[0])
	}
}

func TestAFragmentWithoutReadsKeysOnItsName(t *testing.T) {
	result, _ := compilePage(t, `{% fragment "static" cache="1m" %}<p>hello</p>{% endfragment %}`)
	_, keys := renderWithHook(t, result, runtime.Empty{})
	if len(keys) != 1 || keys[0] != "static" {
		t.Errorf("keys = %q", keys)
	}
}

func TestAFragmentWithoutAHookRendersInline(t *testing.T) {
	result, _ := compilePage(t, fragmentPage)
	chain := result.Manifest.Chain(result.Manifest.Routes[0])
	out := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(out)
	props := runtime.Map{"ID": runtime.String("7"), "Title": runtime.String("t"), "Note": runtime.String("n")}
	if err := runtime.Render(chain, props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "<p>n for 7</p>") {
		t.Errorf("html = %q", out.String())
	}
}

func TestFragmentsNest(t *testing.T) {
	result, bag := compilePage(t, `{% fragment "outer" cache="1m" %}a{% fragment "inner" cache="2m" %}b{% endfragment %}c{% endfragment %}`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	html, keys := renderWithHook(t, result, runtime.Empty{})
	if html != "abc" {
		t.Errorf("html = %q", html)
	}
	if len(keys) != 2 {
		t.Errorf("keys = %q, want the outer and the inner", keys)
	}
}

func TestAFragmentHoldsLoopsAndConditionals(t *testing.T) {
	result, bag := compilePage(t, `---
type Props struct {
	Names []string
	Show  bool
}
---
{% fragment "list" cache="1m" %}{% if Show %}{% for n in Names %}<li>{{ n }}</li>{% endfor %}{% endif %}{% endfragment %}after`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	html, _ := renderWithHook(t, result, runtime.Map{
		"Names": runtime.Seq(runtime.Values{runtime.String("a"), runtime.String("b")}),
		"Show":  runtime.Bool(true),
	})
	if html != "<li>a</li><li>b</li>after" {
		t.Errorf("html = %q", html)
	}
}

func TestFragmentDiagnostics(t *testing.T) {
	cases := map[string]string{
		"no name":       `{% fragment %}x{% endfragment %}`,
		"unquoted name": `{% fragment reviews %}x{% endfragment %}`,
		"bad duration":  `{% fragment "a" cache="5 minutes" %}x{% endfragment %}`,
		"bad stale":     `{% fragment "a" stale="soon" %}x{% endfragment %}`,
		"unknown key":   `{% fragment "a" wobble="1m" %}x{% endfragment %}`,
		"missing value": `{% fragment "a" cache %}x{% endfragment %}`,
		"raw value":     `{% fragment "a" cache=5m %}x{% endfragment %}`,
		"never closed":  `{% fragment "a" %}x`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := compilePage(t, source)
			if !bag.HasErrors() {
				t.Errorf("%q was accepted", source)
			}
		})
	}
}

func TestTwoFragmentsCannotShareAName(t *testing.T) {
	_, bag := compilePage(t, `{% fragment "a" %}x{% endfragment %}{% fragment "a" %}y{% endfragment %}`)
	if !hasCode(bag, diag.C314) {
		t.Errorf("diagnostics = %v, want C314", bag.Sorted())
	}
}

func TestABadDurationIsReportedAsC314(t *testing.T) {
	_, bag := compilePage(t, `{% fragment "a" cache="5 minutes" %}x{% endfragment %}`)
	if !hasCode(bag, diag.C314) {
		t.Errorf("diagnostics = %v, want C314", bag.Sorted())
	}
}

func TestATemplateCannotHoldEndlessFragments(t *testing.T) {
	var b strings.Builder
	for i := range MaxFragments + 1 {
		b.WriteString(`{% fragment "f`)
		b.WriteString(strings.Repeat("x", i+1))
		b.WriteString(`" %}y{% endfragment %}`)
	}
	_, bag := compilePage(t, b.String())
	if !hasCode(bag, diag.C314) {
		t.Errorf("diagnostics = %v, want C314", bag.Sorted())
	}
}

func TestABodyStillRendersWhenTheFragmentIsRejected(t *testing.T) {
	result, bag := compilePage(t, `{% fragment "a" cache="soon" %}kept{% endfragment %}`)
	if !bag.HasErrors() {
		t.Fatal("expected a diagnostic")
	}
	chain := result.Manifest.Chain(result.Manifest.Routes[0])
	out := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(out)
	if err := runtime.Render(chain, runtime.Empty{}, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.String() != "kept" {
		t.Errorf("html = %q, want the body kept so later errors still surface", out.String())
	}
}

func deferredProject(page string) fstest.MapFS {
	return fstest.MapFS{"app/page.rill": &fstest.MapFile{Data: []byte(page)}}
}

const deferredPage = `---
type Review struct {
	Author string
}

type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}
---
<h1>{{ Heading }}</h1>
{% fragment "Reviews" defer %}{% for review in Reviews %}<b>{{ review.Author }}</b>{% endfor %}{% endfragment %}
`

func TestADeferredFragmentIsMarkedInThePlan(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(deferredProject(deferredPage), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	plan := result.Manifest.Plans[result.Manifest.Routes[0].Plan]
	if len(plan.Fragments) != 1 || !plan.Fragments[0].Deferred {
		t.Fatalf("fragments = %+v, want one deferred", plan.Fragments)
	}
	if plan.Fragments[0].Name != "Reviews" {
		t.Errorf("name = %q", plan.Fragments[0].Name)
	}
}

func TestADeferredFragmentWithoutALoaderIsRejected(t *testing.T) {
	page := `---
type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}
---
{% fragment "Reviews" defer %}<b>x</b>{% endfragment %}
`
	var bag diag.Bag
	if _, err := Compile(deferredProject(page), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C318) {
		t.Fatalf("diagnostics = %v, want C318", bag.Sorted())
	}
}

func TestADeferredFragmentInsideALoopIsRejected(t *testing.T) {
	page := `---
type Review struct {
	Author string
}

type Props struct {
	Rows []Review
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}
---
{% for row in Rows %}{% fragment "Reviews" defer %}<b>x</b>{% endfragment %}{% endfor %}
`
	var bag diag.Bag
	if _, err := Compile(deferredProject(page), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C318) {
		t.Fatalf("diagnostics = %v, want C318 for a fragment in a loop", bag.Sorted())
	}
}

func TestADeferredFragmentInsideABranchIsRejected(t *testing.T) {
	page := `---
type Review struct {
	Author string
}

type Props struct {
	Ready bool
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}
---
{% if Ready %}{% fragment "Reviews" defer %}<b>x</b>{% endfragment %}{% endif %}
`
	var bag diag.Bag
	if _, err := Compile(deferredProject(page), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C318) {
		t.Fatalf("diagnostics = %v, want C318 for a fragment in a branch", bag.Sorted())
	}
}

const heldPage = `---
type Review struct {
	Author string
}

type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}
---
<h1>{{ Heading }}</h1>
{% fragment "Reviews" defer %}<b>body</b>{% placeholder %}<i>waiting</i>{% endfragment %}
<p>after</p>
`

func TestAPlaceholderGetsItsOwnRangeAndIsJumpedOver(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(deferredProject(heldPage), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	plan := result.Manifest.Plans[result.Manifest.Routes[0].Plan]
	fragment := plan.Fragments[0]
	if !fragment.Held() {
		t.Fatalf("fragment = %+v, want a placeholder range", fragment)
	}
	if fragment.BodyEnd >= fragment.Hold {
		t.Errorf("body ends at %d, placeholder starts at %d", fragment.BodyEnd, fragment.Hold)
	}
	jump := plan.Ops[fragment.BodyEnd]
	if jump.Kind != ir.OpJump || jump.B != fragment.HoldEnd {
		t.Errorf("op at the end of the body = %+v, want a jump to %d", jump, fragment.HoldEnd)
	}
}

func TestAPlaceholderOutsideADeferredFragmentIsRejected(t *testing.T) {
	page := `---
type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}
---
{% fragment "Reviews" %}<b>body</b>{% placeholder %}<i>waiting</i>{% endfragment %}
`
	var bag diag.Bag
	if _, err := Compile(deferredProject(page), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C319) {
		t.Fatalf("diagnostics = %v, want C319", bag.Sorted())
	}
}

func strategyPage(directive string) string {
	return `---
type Review struct {
	Author string
}

type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}
---
{% fragment "Reviews" ` + directive + ` %}{% for review in Reviews %}<b>{{ review.Author }}</b>{% endfor %}{% endfragment %}
`
}

func TestAFragmentStrategyIsCheckedAgainstTheDeliveryMode(t *testing.T) {
	cases := map[string]struct {
		directive string
		mode      string
		wants     bool
	}{
		"visible needs fetch mode":       {`defer="visible"`, "tail", true},
		"visible is fine in fetch mode":  {`defer="visible"`, "fetch", false},
		"idle is fine in fetch mode":     {`defer="idle"`, "fetch", false},
		"an unknown strategy is refused": {`defer="soon"`, "fetch", true},
		"plain defer still works":        {"defer", "tail", false},
	}
	for name, want := range cases {
		files := fstest.MapFS{
			"rill.jsonc":    &fstest.MapFile{Data: []byte(`{"fragments": {"deferred": "` + want.mode + `"}}`)},
			"app/page.rill": &fstest.MapFile{Data: []byte(strategyPage(want.directive))},
		}
		var bag diag.Bag
		if _, err := Compile(files, &bag); err != nil {
			t.Fatalf("%s: Compile: %v", name, err)
		}
		if got := hasCode(&bag, diag.C318); got != want.wants {
			t.Errorf("%s: C318 = %v, want %v (%v)", name, got, want.wants, bag.Sorted())
		}
	}
}

func TestTheStrategyReachesThePlan(t *testing.T) {
	files := fstest.MapFS{
		"rill.jsonc":    &fstest.MapFile{Data: []byte("{\"fragments\": {\"deferred\": \"fetch\"}}")},
		"app/page.rill": &fstest.MapFile{Data: []byte(strategyPage(`defer="visible"`))},
	}
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil || bag.HasErrors() {
		t.Fatalf("Compile: %v, %v", err, bag.Sorted())
	}
	plan := result.Manifest.Plans[result.Manifest.Routes[0].Plan]
	if len(plan.Fragments) != 1 || plan.Fragments[0].Strategy != "visible" {
		t.Errorf("fragments = %+v", plan.Fragments)
	}
}
