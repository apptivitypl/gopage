package compile

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

func islandProject(page string) fstest.MapFS {
	return fstest.MapFS{
		"components/Search/props.go": &fstest.MapFile{Data: []byte(`package search

type Props struct {
	Placeholder string
	Tags        []string ` + "`rill:\"rest\"`" + `
	Compact     bool
}
`)},
		"components/Search/template.rill": &fstest.MapFile{Data: []byte(`<div class="search">{{ Placeholder }}</div>`)},
		"components/Search/client.ts":     &fstest.MapFile{Data: []byte("export function mount() {}")},
		"components/Badge/props.go":       &fstest.MapFile{Data: []byte("package badge\n\ntype Props struct {\n\tLabel string\n}\n")},
		"components/Badge/template.rill":  &fstest.MapFile{Data: []byte(`<b>{{ Label }}</b>`)},
		"app/page.rill":                   &fstest.MapFile{Data: []byte(page)},
	}
}

func compileIsland(t *testing.T, page string) (Result, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(islandProject(page), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return result, &bag
}

func renderIsland(t *testing.T, result Result, props runtime.Accessible) string {
	t.Helper()
	chain := result.Manifest.Chain(result.Manifest.Routes[0])
	out := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(out)
	if err := runtime.Render(chain, props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestAnIslandCarriesPropsAndMarkup(t *testing.T) {
	result, bag := compileIsland(t, `---
type Props struct {
	Tags []string
}
---
<Search client="visible" Placeholder="find" :Tags="Tags" />`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	html := renderIsland(t, result, runtime.Map{
		"Tags": runtime.Seq(runtime.Values{runtime.String("a"), runtime.String("<b>")}),
	})
	for _, want := range []string{
		`<rill-island style="display:contents" name="Search" strategy="visible">`,
		`<script type="application/json">`,
		`"Placeholder":"find"`,
		`"Tags":["a","\u003cb\u003e"]`,
		`<div class="search">find</div>`,
		`</rill-island>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html = %q, want %q", html, want)
		}
	}
}

func TestAnIslandStillRendersWithoutJavascript(t *testing.T) {
	result, _ := compileIsland(t, `<Search client="load" Placeholder="find" />`)
	html := renderIsland(t, result, runtime.Empty{})
	if !strings.Contains(html, `<div class="search">find</div>`) {
		t.Errorf("html = %q", html)
	}
}

func TestEveryStrategyCompiles(t *testing.T) {
	for _, strategy := range Strategies() {
		page := `<Search client="` + strategy + `" Placeholder="x"`
		if strategy == "media" {
			page += ` media="(min-width: 40rem)"`
		}
		page += " />"
		result, bag := compileIsland(t, page)
		if bag.HasErrors() {
			t.Fatalf("%s: %v", strategy, bag.Sorted())
		}
		if !strings.Contains(renderIsland(t, result, runtime.Empty{}), `strategy="`+strategy+`"`) {
			t.Errorf("%s did not reach the markup", strategy)
		}
	}
}

func TestTheMediaStrategyCarriesItsQuery(t *testing.T) {
	result, _ := compileIsland(t, `<Search client="media" media="(min-width: 40rem)" Placeholder="x" />`)
	if !strings.Contains(renderIsland(t, result, runtime.Empty{}), `media="(min-width: 40rem)"`) {
		t.Errorf("html = %q", renderIsland(t, result, runtime.Empty{}))
	}
}

func TestIslandDiagnostics(t *testing.T) {
	cases := map[string]string{
		"unknown strategy":                `<Search client="soon" Placeholder="x" />`,
		"media without a query":           `<Search client="media" Placeholder="x" />`,
		"component without a client file": `<Badge client="load" Label="x" />`,
	}
	for name, page := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := compileIsland(t, page)
			if !hasCode(bag, diag.C315) {
				t.Errorf("diagnostics = %v, want C315", bag.Sorted())
			}
		})
	}
}

func TestAComponentWithoutClientIsStillInlined(t *testing.T) {
	result, bag := compileIsland(t, `<Badge Label="ready" />`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	html := renderIsland(t, result, runtime.Empty{})
	if strings.Contains(html, "rill-island") || !strings.Contains(html, "<b>ready</b>") {
		t.Errorf("html = %q", html)
	}
}

func TestIslandDiscovery(t *testing.T) {
	islands := DiscoverIslands(islandProject(`<Badge Label="x" />`))
	if len(islands) != 1 {
		t.Fatalf("islands = %+v", islands)
	}
	found := islands["Search"]
	if found.Dir != "components/Search" || found.Client != "components/Search/client.ts" {
		t.Errorf("island = %+v", found)
	}
	if !slices.Contains(Strategies(), "idle") {
		t.Errorf("strategies = %v", Strategies())
	}
}

func TestAnIslandOmitsItsOwnAttributesFromTheProps(t *testing.T) {
	result, _ := compileIsland(t, `<Search client="media" media="(min-width: 40rem)" Placeholder="x" />`)
	html := renderIsland(t, result, runtime.Empty{})
	script := html[strings.Index(html, "{") : strings.Index(html, "}")+1]
	for _, forbidden := range []string{"client", "media"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("props = %q, want %q left out", script, forbidden)
		}
	}
	if !strings.Contains(script, `"Compact":false`) {
		t.Errorf("props = %q, want the defaulted bool", script)
	}
}

func singleFileIsland(page, script string) fstest.MapFS {
	return fstest.MapFS{
		"components/Ticker.rill": &fstest.MapFile{Data: []byte(`---
type Props struct {
	Start int
}
---
<output>{{ Start }}</output>

<script client>
` + script + `
</script>
`)},
		"app/page.rill": &fstest.MapFile{Data: []byte(page)},
	}
}

func TestASingleFileComponentIsAnIsland(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(singleFileIsland(`<Ticker client="load" :Start="3" />`, "export function mount() { return () => {} }"), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	island, ok := result.Islands["Ticker"]
	if !ok {
		t.Fatalf("islands = %v, want Ticker", result.Islands)
	}
	if !island.Inline() || !strings.Contains(island.Code, "export function mount") {
		t.Errorf("island = %+v, want the browser code carried inline", island)
	}
	html := renderIsland(t, result, runtime.Empty{})
	if !strings.Contains(html, `name="Ticker"`) || !strings.Contains(html, "<output>3</output>") {
		t.Errorf("html = %q", html)
	}
	if strings.Contains(html, "export function mount") {
		t.Error("the browser code must not be rendered into the document")
	}
}

func TestAClientScriptOutsideAComponentIsRejected(t *testing.T) {
	var bag diag.Bag
	files := fstest.MapFS{
		"app/page.rill": &fstest.MapFile{Data: []byte("<p>x</p>\n<script client>export function mount() {}</script>")},
	}
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C317) {
		t.Fatalf("diagnostics = %v, want C317", bag.Sorted())
	}
}

func TestASecondClientScriptIsRejected(t *testing.T) {
	var bag diag.Bag
	files := singleFileIsland(`<Ticker client="load" :Start="1" />`, "export function mount() {}")
	files["components/Ticker.rill"] = &fstest.MapFile{Data: []byte(`<output>x</output>
<script client>export function mount() {}</script>
<script client>export function mount() {}</script>
`)}
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C317) {
		t.Fatalf("diagnostics = %v, want C317 for the second block", bag.Sorted())
	}
}

func TestAClientScriptWithoutMountIsRejected(t *testing.T) {
	var bag diag.Bag
	files := singleFileIsland(`<Ticker client="load" :Start="1" />`, "const value = 1;")
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C317) {
		t.Fatalf("diagnostics = %v, want C317 for a script that exports no mount", bag.Sorted())
	}
}

func TestAReactIslandIsDiscoveredInEitherShape(t *testing.T) {
	files := singleFileIsland(`<Ticker client="load" :Start="1" />`, "export function mount() {}")
	files["components/Ticker.rill"] = &fstest.MapFile{Data: []byte(`---
type Props struct {
	Start int
}
---
<output>{{ Start }}</output>

<script client lang="tsx">
import type { Props } from "rill:props/Ticker";
export default function Ticker(props: Props) { return <output>{props.Start}</output>; }
</script>
`)}
	files["components/Chart/props.go"] = &fstest.MapFile{Data: []byte("package chart\n\ntype Props struct {\n\tPoints []int\n}\n")}
	files["components/Chart/template.rill"] = &fstest.MapFile{Data: []byte(`<div class="chart"></div>`)}
	files["components/Chart/client.tsx"] = &fstest.MapFile{Data: []byte("export default function Chart() { return null; }\n")}
	files["components/Plain/props.go"] = &fstest.MapFile{Data: []byte("package plain\n\ntype Props struct {\n\tLabel string\n}\n")}
	files["components/Plain/template.rill"] = &fstest.MapFile{Data: []byte(`<b></b>`)}
	files["components/Plain/client.js"] = &fstest.MapFile{Data: []byte("export function mount() {}\n")}

	islands := DiscoverIslands(files)
	cases := map[string]Island{
		"Ticker": {Lang: "tsx", React: true},
		"Chart":  {Lang: "tsx", React: true},
		"Plain":  {Lang: "js", React: false},
	}
	for name, want := range cases {
		got, ok := islands[name]
		if !ok {
			t.Fatalf("islands = %v, want %s", islands, name)
		}
		if got.Lang != want.Lang || got.React != want.React {
			t.Errorf("%s: lang = %q, react = %v, want %q, %v", name, got.Lang, got.React, want.Lang, want.React)
		}
	}
	if got := islands["Ticker"]; !got.Inline() || !strings.Contains(got.Code, "export default") {
		t.Errorf("Ticker = %+v, want the tsx body carried inline", got)
	}

	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v, want a default export accepted as an island", bag.Sorted())
	}
}

func TestAClientScriptWithoutALangIsTypescript(t *testing.T) {
	files := singleFileIsland(`<Ticker client="load" :Start="1" />`, "export function mount() {}")
	if got := DiscoverIslands(files)["Ticker"]; got.Lang != "ts" || got.React {
		t.Errorf("island = %+v, want plain typescript", got)
	}
}

func TestAClientScriptExportingBothShapesIsRejected(t *testing.T) {
	var bag diag.Bag
	files := singleFileIsland(`<Ticker client="load" :Start="1" />`,
		"export function mount() {}\nexport default function T() { return null }")
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C317) {
		t.Fatalf("diagnostics = %v, want C317 for an ambiguous script", bag.Sorted())
	}
}

func TestAPlanRecordsWhichIslandsItUsesAndHow(t *testing.T) {
	result, bag := compileIsland(t, `<Search client="idle" Placeholder="find" /><Badge Label="x" /><Search client="visible" Placeholder="again" />`)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	plan := result.Manifest.Plans[result.Manifest.Routes[0].Plan]
	if len(plan.Islands) != 2 {
		t.Fatalf("islands = %+v, want both uses and no plain component", plan.Islands)
	}
	if plan.Islands[0].Strategy != "idle" || plan.Islands[1].Strategy != "visible" || plan.Islands[0].Name != "Search" {
		t.Errorf("islands = %+v", plan.Islands)
	}
}

func TestTheAssetsBlockLeavesRoomForRoutePreloads(t *testing.T) {
	files := islandProject(`<p>x</p>`)
	files["app/layout.rill"] = &fstest.MapFile{Data: []byte("<head>{% assets %}</head>{% outlet %}")}
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	layout := result.Manifest.Plans[result.Manifest.Routes[0].LayoutChain[0]]
	var found bool
	for _, op := range layout.Ops {
		found = found || op.Kind == ir.OpPreload
	}
	if !found {
		t.Errorf("ops = %v, want a preload op after the asset tags", layout.Ops)
	}
}
