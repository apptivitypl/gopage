package syntax

import (
	"slices"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
)

func parse(t *testing.T, src string) (*Document, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	return Parse("page.gopage", src, &bag), &bag
}

func parseClean(t *testing.T, src string) *Document {
	t.Helper()
	doc, bag := parse(t, src)
	if bag.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %+v", bag.Items())
	}
	return doc
}

func codesOf(bag *diag.Bag) []diag.Code {
	codes := make([]diag.Code, 0, bag.Len())
	for _, d := range bag.Items() {
		codes = append(codes, d.Code)
	}
	return codes
}

func hasCode(bag *diag.Bag, want diag.Code) bool {
	return slices.Contains(codesOf(bag), want)
}

func TestParseTextOnly(t *testing.T) {
	doc := parseClean(t, "plain words")
	if len(doc.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(doc.Nodes))
	}
	text, ok := doc.Nodes[0].(*Text)
	if !ok || text.Value != "plain words" {
		t.Errorf("node = %#v", doc.Nodes[0])
	}
	if text.NodeSpan().Start != 0 {
		t.Errorf("span = %+v", text.NodeSpan())
	}
}

func TestElementsBecomeNodes(t *testing.T) {
	doc := parseClean(t, "<h1>hi</h1>")
	if len(doc.Nodes) != 3 {
		t.Fatalf("nodes = %d, want the open tag, the text and the close tag", len(doc.Nodes))
	}
	open, ok := doc.Nodes[0].(*Element)
	if !ok || open.Name != "h1" {
		t.Errorf("first node = %#v", doc.Nodes[0])
	}
	closing, ok := doc.Nodes[2].(*Text)
	if !ok || closing.Value != "</h1>" {
		t.Errorf("last node = %#v", doc.Nodes[2])
	}
}

func TestParseInterpolationPath(t *testing.T) {
	doc := parseClean(t, "{{ Listing.Title }}")
	interp, ok := doc.Nodes[0].(*Interpolation)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	path, ok := interp.Expr.(*Path)
	if !ok {
		t.Fatalf("expression = %#v", interp.Expr)
	}
	if strings.Join(path.Segments, ".") != "Listing.Title" {
		t.Errorf("path = %v", path.Segments)
	}
}

func TestParseFrontmatter(t *testing.T) {
	doc := parseClean(t, "---\ntype Props struct{}\n---\nbody text")
	if doc.Frontmatter == nil {
		t.Fatal("frontmatter was not captured")
	}
	if doc.Frontmatter.Code != "type Props struct{}\n" {
		t.Errorf("code = %q", doc.Frontmatter.Code)
	}
	if len(doc.Nodes) != 1 {
		t.Errorf("nodes = %d, want the body only", len(doc.Nodes))
	}
}

func TestParseOutletDirective(t *testing.T) {
	doc := parseClean(t, "<main>{% outlet %}</main>")
	if !doc.HasOutlet() {
		t.Error("outlet was not parsed")
	}
	if len(doc.Nodes) != 3 {
		t.Errorf("nodes = %d, want text, outlet, text", len(doc.Nodes))
	}
}

func TestDocumentWithoutOutlet(t *testing.T) {
	if parseClean(t, "<p>x</p>").HasOutlet() {
		t.Error("HasOutlet must be false here")
	}
}

func TestWalkVisitsNestedNodes(t *testing.T) {
	doc := parseClean(t, "{% if A %}one{{ B }}two{% endif %}")
	var texts, interpolations int
	Walk(doc.Nodes, func(node Node) {
		switch node.(type) {
		case *Text:
			texts++
		case *Interpolation:
			interpolations++
		}
	})
	if texts != 2 || interpolations != 1 {
		t.Errorf("walk saw %d texts and %d interpolations", texts, interpolations)
	}
}

func TestUnterminatedFrontmatterReportsC001(t *testing.T) {
	_, bag := parse(t, "---\ntype Props struct{}\n")
	if !hasCode(bag, diag.C001) {
		t.Errorf("codes = %v, want C001", codesOf(bag))
	}
}

func TestUnterminatedInterpolationReportsC002(t *testing.T) {
	for _, src := range []string{"<p>{{ Title", "<p>{{ ", "<p>{{"} {
		_, bag := parse(t, src)
		if !hasCode(bag, diag.C002) {
			t.Errorf("parse(%q) codes = %v, want C002", src, codesOf(bag))
		}
	}
}

func TestNonValueInsideInterpolationReportsC201(t *testing.T) {
	_, bag := parse(t, "{{ # }}")
	if !hasCode(bag, diag.C201) {
		t.Errorf("codes = %v, want C201", codesOf(bag))
	}
}

func TestUnknownDirectiveReportsC004(t *testing.T) {
	_, bag := parse(t, "{% wobble %}")
	if !hasCode(bag, diag.C004) {
		t.Errorf("codes = %v, want C004", codesOf(bag))
	}
	if bag.Items()[0].Help == "" {
		t.Error("an unknown directive must come with a suggestion")
	}
}

func TestNamelessDirectiveReportsC004(t *testing.T) {
	_, bag := parse(t, "{% %}")
	if !hasCode(bag, diag.C004) {
		t.Errorf("codes = %v, want C004", codesOf(bag))
	}
}

func TestUnterminatedDirectiveReportsC005(t *testing.T) {
	_, bag := parse(t, "{% outlet")
	if !hasCode(bag, diag.C005) {
		t.Errorf("codes = %v, want C005", codesOf(bag))
	}
}

func TestSuggestionForANearMiss(t *testing.T) {
	_, bag := parse(t, "{% outl %}")
	if !strings.Contains(bag.Items()[0].Help, "outlet") {
		t.Errorf("help = %q, want it to suggest outlet", bag.Items()[0].Help)
	}
}

func TestParserRecoversAndKeepsGoing(t *testing.T) {
	doc, bag := parse(t, "<a>{{ # }}</a><b>{{ Ok }}</b>")
	if bag.Len() == 0 {
		t.Fatal("expected a diagnostic for the broken interpolation")
	}
	var found bool
	for _, node := range doc.Nodes {
		interp, ok := node.(*Interpolation)
		if !ok {
			continue
		}
		if path, ok := interp.Expr.(*Path); ok && path.Segments[0] == "Ok" {
			found = true
		}
	}
	if !found {
		t.Errorf("the parser must keep parsing after an error, nodes = %d", len(doc.Nodes))
	}
}

func TestDiagnosticsCarryTheFileName(t *testing.T) {
	_, bag := parse(t, "{{ # }}")
	if bag.Items()[0].File != "page.gopage" {
		t.Errorf("file = %q", bag.Items()[0].File)
	}
}

func TestParseIsTotal(t *testing.T) {
	inputs := []string{
		"", "---", "---\n", "{{", "{%", "}}", "%}", "{{}}", "{%%}", "{{.}}", "{{a.}}",
		"---\n---", "---\n---\n", "{{ a.b.c.d }}", "\x00\x01", "{% outlet %}{{",
	}
	for _, src := range inputs {
		var bag diag.Bag
		if doc := Parse("x.gopage", src, &bag); doc == nil {
			t.Errorf("Parse(%q) returned nil", src)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"<h1>{{ Title }}</h1>",
		"---\ntype Props struct{}\n---\n{% outlet %}",
		"{{ a.b }}{% outlet %}",
		"---",
		"{{",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		var bag diag.Bag
		doc := Parse("fuzz.gopage", src, &bag)
		if doc == nil {
			t.Fatal("Parse returned nil")
		}
		for _, d := range bag.Items() {
			if !d.Code.Known() {
				t.Fatalf("unknown diagnostic code %q", d.Code)
			}
			if d.Span.End < d.Span.Start {
				t.Fatalf("inverted span %+v", d.Span)
			}
		}
	})
}

func TestDirectivesAreListed(t *testing.T) {
	names := Directives()
	for _, want := range []string{"outlet", "if", "for", "fragment", "meta", "assets"} {
		if !slices.Contains(names, want) {
			t.Errorf("directives = %v, want %q", names, want)
		}
	}
	names[0] = "changed"
	if Directives()[0] == "changed" {
		t.Error("the caller must not be able to edit the list")
	}
}

func TestAClientScriptIsOneRawNode(t *testing.T) {
	source := "<div>x</div>\n<script client>\nconst a = 1 < 2 && {{ not an interpolation }};\n</script>\n"
	var bag diag.Bag
	document := Parse("components/Counter.gopage", source, &bag)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	script, ok := ClientScriptOf(document)
	if !ok {
		t.Fatal("the client script was not parsed")
	}
	if !strings.Contains(script.Code, "1 < 2 &&") || !strings.Contains(script.Code, "{{ not an interpolation }}") {
		t.Errorf("code = %q, want the body kept verbatim", script.Code)
	}
}

func TestAPlainScriptStaysMarkup(t *testing.T) {
	var bag diag.Bag
	document := Parse("app/page.gopage", "<script>console.log(1)</script>", &bag)
	if _, ok := ClientScriptOf(document); ok {
		t.Error("a script without the client attribute is ordinary markup")
	}
}

func TestAnUnclosedClientScriptStaysText(t *testing.T) {
	var bag diag.Bag
	document := Parse("components/Counter.gopage", "<script client>\nconst a = 1;\n", &bag)
	if _, ok := ClientScriptOf(document); ok {
		t.Error("an unterminated script must not be taken for a client block")
	}
	if len(document.Nodes) == 0 {
		t.Error("the parser dropped the input")
	}
}

func TestEveryClientScriptIsListed(t *testing.T) {
	var bag diag.Bag
	document := Parse("components/Counter.gopage",
		"<script client>export function mount() {}</script><script client>export function mount() {}</script>", &bag)
	if got := len(ClientScripts(document)); got != 2 {
		t.Errorf("scripts = %d, want both so the compiler can reject the second", got)
	}
}

func TestEveryNodeReportsItsSpan(t *testing.T) {
	source := "<div>{{ Name }}</div>\n{% fragment \"Reviews\" defer %}x{% endfragment %}\n<script client>export function mount() {}</script>"
	var bag diag.Bag
	document := Parse("components/Reviews.gopage", source, &bag)
	for _, node := range document.Nodes {
		if node.NodeSpan().End == 0 && node.NodeSpan().Start == 0 {
			t.Errorf("%T reports an empty span", node)
		}
	}
}

func TestTheDeferFlagIsParsedAlongsideCache(t *testing.T) {
	var bag diag.Bag
	document := Parse("app/page.gopage", `{% fragment "Reviews" defer cache="5m" %}x{% endfragment %}`, &bag)
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	fragment, ok := document.Nodes[0].(*Fragment)
	if !ok {
		t.Fatalf("nodes = %T", document.Nodes[0])
	}
	if !fragment.Defer || fragment.Cache != "5m" {
		t.Errorf("fragment = %+v, want defer and a cache window", fragment)
	}
}

func TestAnUnknownFragmentSettingIsReported(t *testing.T) {
	var bag diag.Bag
	Parse("app/page.gopage", `{% fragment "Reviews" later="1m" %}x{% endfragment %}`, &bag)
	if !bag.HasErrors() {
		t.Error("an unknown setting must be reported")
	}
}

func TestAFragmentTakesAPlaceholderSection(t *testing.T) {
	doc := parseClean(t, `{% fragment "Reviews" defer %}<b>body</b>{% placeholder %}<i>hold</i>{% endfragment %}`)
	fragment, ok := doc.Nodes[0].(*Fragment)
	if !ok {
		t.Fatalf("node = %T, want a fragment", doc.Nodes[0])
	}
	if len(fragment.Body) == 0 || len(fragment.Placeholder) == 0 {
		t.Fatalf("body = %d nodes, placeholder = %d nodes, want both filled",
			len(fragment.Body), len(fragment.Placeholder))
	}
	if !fragment.Defer {
		t.Error("the fragment kept its defer setting")
	}
}

func TestAPlaceholderOutsideAFragmentIsReported(t *testing.T) {
	_, bag := parse(t, `<b>x</b>{% placeholder %}<i>hold</i>`)
	if bag.Len() == 0 {
		t.Fatal("a placeholder with no fragment around it must be reported")
	}
}

func TestAFragmentWithAnUnclosedPlaceholderIsReported(t *testing.T) {
	_, bag := parse(t, `{% fragment "Reviews" defer %}<b>body</b>{% placeholder %}<i>hold</i>`)
	if bag.Len() == 0 {
		t.Fatal("an unclosed placeholder must be reported")
	}
}

func TestAClientScriptCarriesItsLanguage(t *testing.T) {
	cases := map[string]string{
		`<script client lang="tsx">export default () => null</script>`: "tsx",
		`<script lang='jsx' client>export default () => null</script>`: "jsx",
		`<script client>export function mount() {}</script>`:           "",
	}
	for source, want := range cases {
		var bag diag.Bag
		script, ok := ClientScriptOf(Parse("components/Chart.gopage", source, &bag))
		if !ok {
			t.Fatalf("%s: the client script was not parsed", source)
		}
		if script.Lang != want {
			t.Errorf("%s: lang = %q, want %q", source, script.Lang, want)
		}
	}
}

func TestAClosingAngleInsideAnAttributeDoesNotEndTheTag(t *testing.T) {
	var bag diag.Bag
	source := `<script client data-note="a > b">export function mount() {}</script>`
	script, ok := ClientScriptOf(Parse("components/Chart.gopage", source, &bag))
	if !ok {
		t.Fatal("the client script was not parsed")
	}
	if !strings.HasPrefix(script.Code, "export function mount") {
		t.Errorf("code = %q, want the body to start after the real tag end", script.Code)
	}
}

func TestClientTagEdgesAreHandled(t *testing.T) {
	cases := map[string]bool{
		`<script client data-a='x > y'>export function mount() {}</script>`: true,
		`<script client data-a="unterminated>export function mount() {}`:    false,
		`<script client=load>export function mount() {}</script>`:           false,
		`<script client defer>export function mount() {}</script>`:          true,
	}
	for source, want := range cases {
		var bag diag.Bag
		_, got := ClientScriptOf(Parse("components/Chart.gopage", source, &bag))
		if got != want {
			t.Errorf("%s: client script = %v, want %v", source, got, want)
		}
	}
}

func TestDeferCarriesAnOptionalStrategy(t *testing.T) {
	cases := map[string]struct {
		defer_   bool
		strategy string
	}{
		`{% fragment "R" defer %}x{% endfragment %}`:                      {true, ""},
		`{% fragment "R" defer="visible" %}x{% endfragment %}`:            {true, "visible"},
		`{% fragment "R" defer="idle" cache="5m" %}x{% endfragment %}`:    {true, "idle"},
		`{% fragment "R" cache="5m" defer="visible" %}x{% endfragment %}`: {true, "visible"},
		`{% fragment "R" cache="5m" %}x{% endfragment %}`:                 {false, ""},
	}
	for source, want := range cases {
		var bag diag.Bag
		document := Parse("app/page.gopage", source, &bag)
		if bag.HasErrors() {
			t.Fatalf("%s: %v", source, bag.Sorted())
		}
		var fragment *Fragment
		Walk(document.Nodes, func(node Node) {
			if found, ok := node.(*Fragment); ok {
				fragment = found
			}
		})
		if fragment == nil {
			t.Fatalf("%s: no fragment", source)
		}
		if fragment.Defer != want.defer_ || fragment.Strategy != want.strategy {
			t.Errorf("%s: defer = %v, strategy = %q", source, fragment.Defer, fragment.Strategy)
		}
	}
}
