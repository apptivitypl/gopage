package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/runtime"
)

func app(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for name, content := range files {
		out[name] = file(content)
	}
	return out
}

func renderApp(t *testing.T, files map[string]string, props runtime.Accessible) (string, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(app(files), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		return "", &bag
	}
	route, ok := result.Manifest.Lookup("/")
	if !ok {
		t.Fatal("no root route")
	}
	out := runtime.NewBuffer(512)
	if err := runtime.Render(result.Manifest.Chain(route), props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String(), &bag
}

func renderOK(t *testing.T, files map[string]string, props runtime.Accessible) string {
	t.Helper()
	out, bag := renderApp(t, files, props)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	return out
}

func componentError(t *testing.T, files map[string]string, want diag.Code) diag.Diagnostic {
	t.Helper()
	var bag diag.Bag
	if _, err := Compile(app(files), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, d := range bag.Items() {
		if d.Code == want {
			return d
		}
	}
	t.Fatalf("diagnostics = %+v, want %s", bag.Items(), want)
	return diag.Diagnostic{}
}

const badgeProps = `package badge

type Props struct {
	Label string
	Kind  string ` + "`rill:\"default=info\"`" + `
	Loud  bool
}
`

const badgeTemplate = `<span class="badge-{{ Kind }}">{% if Loud %}!{% endif %}{{ Label }}</span>`

func TestComponentRendersWithLiteralAttributes(t *testing.T) {
	got := renderOK(t, map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge Label="new" Kind="ok" Loud />`,
	}, nil)
	if got != `<span class="badge-ok">!new</span>` {
		t.Errorf("render = %q", got)
	}
}

func TestComponentUsesDefaults(t *testing.T) {
	got := renderOK(t, map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge Label="new" />`,
	}, nil)
	if got != `<span class="badge-info">new</span>` {
		t.Errorf("render = %q, want the default kind and no loud marker", got)
	}
}

func TestBoundAttributeReadsAnExpression(t *testing.T) {
	files := map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge :Label="Title" :Loud="Count > 1" />`,
	}
	props := runtime.Map{"Title": runtime.String("hi"), "Count": runtime.Int(5)}
	if got := renderOK(t, files, props); got != `<span class="badge-info">!hi</span>` {
		t.Errorf("render = %q", got)
	}
}

func TestComponentInsideALoopSeesTheLoopVariable(t *testing.T) {
	files := map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `{% for t in Tags %}<Badge :Label="t" />{% endfor %}`,
	}
	props := runtime.Map{"Tags": runtime.Seq(runtime.Values{runtime.String("a"), runtime.String("b")})}
	want := `<span class="badge-info">a</span><span class="badge-info">b</span>`
	if got := renderOK(t, files, props); got != want {
		t.Errorf("render = %q", got)
	}
}

func TestComponentPropsDoNotLeakIntoTheCaller(t *testing.T) {
	files := map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge Label="x" />{{ Label }}`,
	}
	props := runtime.Map{"Label": runtime.String("from props")}
	if got := renderOK(t, files, props); !strings.HasSuffix(got, "from props") {
		t.Errorf("render = %q, want the caller to see its own props", got)
	}
}

func TestCallerLocalsAreInvisibleInsideTheComponent(t *testing.T) {
	files := map[string]string{
		"components/Leak/props.go":      "package leak\n\ntype Props struct{ A string }",
		"components/Leak/template.rill": "{{ Hidden }}",
		"app/page.rill":                 "{% let Hidden = 'secret' %}<Leak A=\"x\" />",
	}
	var bag diag.Bag
	result, err := Compile(app(files), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	route, _ := result.Manifest.Lookup("/")
	err = runtime.Render(result.Manifest.Chain(route), runtime.Map{}, runtime.NewBuffer(64))
	if err == nil {
		t.Error("a component must not see the caller's locals")
	}
}

const cardProps = `package card

type Props struct {
	Title string
}
`

const cardTemplate = `<article><h2>{{ Title }}</h2><div>{% children %}</div><footer>{% slot "footer" %}</footer></article>`

func TestChildrenAreSpliced(t *testing.T) {
	got := renderOK(t, map[string]string{
		"components/Card/props.go":      cardProps,
		"components/Card/template.rill": cardTemplate,
		"app/page.rill":                 `<Card Title="t"><p>body</p></Card>`,
	}, nil)
	if got != `<article><h2>t</h2><div><p>body</p></div><footer></footer></article>` {
		t.Errorf("render = %q", got)
	}
}

func TestNamedSlotIsFilled(t *testing.T) {
	page := `<Card Title="t"><p>body</p><Template #footer><b>end</b></Template></Card>`
	got := renderOK(t, map[string]string{
		"components/Card/props.go":      cardProps,
		"components/Card/template.rill": cardTemplate,
		"app/page.rill":                 page,
	}, nil)
	if got != `<article><h2>t</h2><div><p>body</p></div><footer><b>end</b></footer></article>` {
		t.Errorf("render = %q", got)
	}
}

func TestChildrenSeeTheCallerScope(t *testing.T) {
	files := map[string]string{
		"components/Card/props.go":      cardProps,
		"components/Card/template.rill": cardTemplate,
		"app/page.rill":                 `{% for t in Tags %}<Card Title="c">{{ t }}</Card>{% endfor %}`,
	}
	props := runtime.Map{"Tags": runtime.Seq(runtime.Values{runtime.String("a")})}
	if got := renderOK(t, files, props); !strings.Contains(got, "<div>a</div>") {
		t.Errorf("render = %q, want the loop variable visible in the children", got)
	}
}

func TestComponentsNest(t *testing.T) {
	files := map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"components/Card/props.go":       cardProps,
		"components/Card/template.rill":  cardTemplate,
		"app/page.rill":                  `<Card Title="t"><Badge Label="inner" /></Card>`,
	}
	if got := renderOK(t, files, nil); !strings.Contains(got, `<span class="badge-info">inner</span>`) {
		t.Errorf("render = %q", got)
	}
}

func TestComponentWithoutPropsRenders(t *testing.T) {
	got := renderOK(t, map[string]string{
		"components/Rule/template.rill": "<hr>",
		"app/page.rill":                 "<Rule />",
	}, nil)
	if got != "<hr>" {
		t.Errorf("render = %q", got)
	}
}

func TestUnknownComponentReportsC307(t *testing.T) {
	d := componentError(t, map[string]string{"app/page.rill": "<Missing />"}, diag.C307)
	if !strings.Contains(d.Message, "no component named Missing") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestUnknownComponentSuggestsANearMiss(t *testing.T) {
	d := componentError(t, map[string]string{
		"components/Badge/template.rill": "<b></b>",
		"app/page.rill":                  "<Badeg />",
	}, diag.C307)
	if !strings.Contains(d.Help, "Badge") {
		t.Errorf("help = %q", d.Help)
	}
}

func TestRecursiveComponentIsRejected(t *testing.T) {
	d := componentError(t, map[string]string{
		"components/Loop/template.rill": "<Loop />",
		"app/page.rill":                 "<Loop />",
	}, diag.C307)
	if !strings.Contains(d.Message, "renders itself") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestMutuallyRecursiveComponentsAreRejected(t *testing.T) {
	componentError(t, map[string]string{
		"components/A/template.rill": "<B />",
		"components/B/template.rill": "<A />",
		"app/page.rill":              "<A />",
	}, diag.C307)
}

func TestUnknownPropReportsC308(t *testing.T) {
	d := componentError(t, map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge Label="x" Nope="y" />`,
	}, diag.C308)
	if !strings.Contains(d.Message, "no prop Nope") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestMisspelledPropSuggestsTheRealOne(t *testing.T) {
	d := componentError(t, map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge Labl="x" />`,
	}, diag.C308)
	if !strings.Contains(d.Help, "Label") {
		t.Errorf("help = %q", d.Help)
	}
}

func TestMissingRequiredPropReportsC308(t *testing.T) {
	d := componentError(t, map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge />`,
	}, diag.C308)
	if !strings.Contains(d.Message, "needs the prop Label") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestOptionalPropMayBeOmitted(t *testing.T) {
	renderOK(t, map[string]string{
		"components/Note/props.go":      "package note\n\ntype Props struct{ Text *string }",
		"components/Note/template.rill": "<i></i>",
		"app/page.rill":                 "<Note />",
	}, nil)
}

func TestNumericPropsAreConverted(t *testing.T) {
	got := renderOK(t, map[string]string{
		"components/Meter/props.go":      "package meter\n\ntype Props struct{ Value int\n Ratio float64 }",
		"components/Meter/template.rill": "{{ Value + 1 }}/{{ Ratio }}",
		"app/page.rill":                  `<Meter Value="41" Ratio="0.5" />`,
	}, nil)
	if got != "42/0.5" {
		t.Errorf("render = %q", got)
	}
}

func TestMalformedNumericPropFallsBackToZero(t *testing.T) {
	got := renderOK(t, map[string]string{
		"components/Meter/props.go":      "package meter\n\ntype Props struct{ Value int\n Ratio float64 }",
		"components/Meter/template.rill": "{{ Value }}/{{ Ratio }}",
		"app/page.rill":                  `<Meter Value="nope" Ratio="nope" />`,
	}, nil)
	if got != "0/0" {
		t.Errorf("render = %q", got)
	}
}

func TestComponentsAreDiscovered(t *testing.T) {
	found := DiscoverComponents(app(map[string]string{
		"components/Badge/template.rill": "<b></b>",
		"components/Card/template.rill":  "<div></div>",
		"components/Card/props.go":       cardProps,
		"app/page.rill":                  "x",
	}))
	if len(found) != 2 || found["Badge"] == "" || found["Card"] == "" {
		t.Errorf("components = %v", found)
	}
}

func TestComponentNamesAreSorted(t *testing.T) {
	names := ComponentNames(map[string]Component{"Z": {}, "A": {}})
	if len(names) != 2 || names[0] != "A" {
		t.Errorf("names = %v", names)
	}
}

func TestComponentPropsFileIsRead(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(app(map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  `<Badge Label="x" />`,
	}), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	component := result.Components["Badge"]
	if len(component.Fields()) != 3 {
		t.Errorf("fields = %+v", component.Fields())
	}
}

func TestComponentWithoutASchemaHasNoFields(t *testing.T) {
	if fields := (Component{}).Fields(); fields != nil {
		t.Errorf("fields = %v", fields)
	}
}

func TestBoundAttributeIsCheckedAgainstTheCallerProps(t *testing.T) {
	d := componentError(t, map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  propsBlock + `<Badge :Label="Missing" />`,
	}, diag.C305)
	if !strings.Contains(d.Message, "no field Missing") {
		t.Errorf("message = %q", d.Message)
	}
}

func TestChildrenAreCheckedAgainstTheCallerProps(t *testing.T) {
	componentError(t, map[string]string{
		"components/Card/props.go":      cardProps,
		"components/Card/template.rill": cardTemplate,
		"app/page.rill":                 propsBlock + `<Card Title="t">{{ Missing }}</Card>`,
	}, diag.C305)
}

func TestSlotContentIsCheckedAgainstTheCallerProps(t *testing.T) {
	componentError(t, map[string]string{
		"components/Card/props.go":      cardProps,
		"components/Card/template.rill": cardTemplate,
		"app/page.rill":                 propsBlock + `<Card Title="t"><Template #footer>{{ Missing }}</Template></Card>`,
	}, diag.C305)
}

func TestComponentBodyIsCheckedAgainstItsOwnProps(t *testing.T) {
	d := componentError(t, map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": "{{ Nope }}",
		"app/page.rill":                  `<Badge Label="x" />`,
	}, diag.C305)
	if !strings.Contains(d.File, "components/Badge") {
		t.Errorf("file = %q, want the component template", d.File)
	}
}

func TestBoundAttributeSpanPointsIntoTheAttribute(t *testing.T) {
	source := propsBlock + `<Badge :Label="Missing" />`
	var bag diag.Bag
	if _, err := Compile(app(map[string]string{
		"components/Badge/props.go":      badgeProps,
		"components/Badge/template.rill": badgeTemplate,
		"app/page.rill":                  source,
	}), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, d := range bag.Items() {
		if d.Code != diag.C305 {
			continue
		}
		if got := source[d.Span.Start:d.Span.End]; got != "Missing" {
			t.Errorf("span covers %q, want the expression inside the attribute", got)
		}
		return
	}
	t.Fatal("no C305 reported")
}
