package syntax

import (
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/diag"
)

func TestIfBranches(t *testing.T) {
	doc := parseClean(t, "{% if A %}one{% elif B %}two{% else %}three{% endif %}")
	node, ok := doc.Nodes[0].(*If)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	if len(node.Branches) != 3 {
		t.Fatalf("branches = %d, want three", len(node.Branches))
	}
	if node.Branches[2].Cond != nil {
		t.Error("the final else branch carries no condition")
	}
	if render(node.Branches[0].Cond) != "A" {
		t.Errorf("first condition = %s", render(node.Branches[0].Cond))
	}
}

func TestIfWithoutElse(t *testing.T) {
	doc := parseClean(t, "{% if A %}one{% endif %}")
	node := doc.Nodes[0].(*If)
	if len(node.Branches) != 1 {
		t.Errorf("branches = %d, want one", len(node.Branches))
	}
}

func TestSeveralElifBranches(t *testing.T) {
	doc := parseClean(t, "{% if A %}1{% elif B %}2{% elif C %}3{% endif %}")
	node := doc.Nodes[0].(*If)
	if len(node.Branches) != 3 {
		t.Errorf("branches = %d, want three", len(node.Branches))
	}
}

func TestForParts(t *testing.T) {
	doc := parseClean(t, "{% for item in Items %}x{% else %}none{% endfor %}")
	node, ok := doc.Nodes[0].(*For)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	if node.Var != "item" {
		t.Errorf("loop variable = %q", node.Var)
	}
	if render(node.Seq) != "Items" {
		t.Errorf("sequence = %s", render(node.Seq))
	}
	if len(node.Body) != 1 || len(node.Empty) != 1 {
		t.Errorf("body = %d nodes, empty = %d nodes", len(node.Body), len(node.Empty))
	}
}

func TestForWithoutElse(t *testing.T) {
	doc := parseClean(t, "{% for item in Items %}x{% endfor %}")
	node := doc.Nodes[0].(*For)
	if len(node.Empty) != 0 {
		t.Errorf("empty = %d nodes, want none", len(node.Empty))
	}
}

func TestLetParts(t *testing.T) {
	doc := parseClean(t, "{% let total = A + B %}")
	node, ok := doc.Nodes[0].(*Let)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	if node.Name != "total" {
		t.Errorf("name = %q", node.Name)
	}
	if render(node.Value) != "(A + B)" {
		t.Errorf("value = %s", render(node.Value))
	}
}

func TestNestedBlocks(t *testing.T) {
	doc := parseClean(t, "{% for a in A %}{% if a %}{{ a }}{% endif %}{% endfor %}")
	loop := doc.Nodes[0].(*For)
	if _, ok := loop.Body[0].(*If); !ok {
		t.Errorf("loop body = %#v", loop.Body[0])
	}
}

func TestOutletInsideABlockIsFound(t *testing.T) {
	for _, source := range []string{
		"{% if A %}{% outlet %}{% endif %}",
		"{% for a in A %}{% outlet %}{% endfor %}",
		"{% for a in A %}{% else %}{% outlet %}{% endfor %}",
	} {
		if !parseClean(t, source).HasOutlet() {
			t.Errorf("%q hides its outlet", source)
		}
	}
}

func TestOutletIsAbsentWhenNotWritten(t *testing.T) {
	if parseClean(t, "{% if A %}x{% endif %}").HasOutlet() {
		t.Error("HasOutlet must be false here")
	}
}

func TestWalkReachesEveryBranch(t *testing.T) {
	doc := parseClean(t, "{% if A %}a{% else %}b{% endif %}{% for x in X %}c{% else %}d{% endfor %}")
	var texts int
	Walk(doc.Nodes, func(node Node) {
		if _, ok := node.(*Text); ok {
			texts++
		}
	})
	if texts != 4 {
		t.Errorf("walk saw %d texts, want 4", texts)
	}
}

func TestUnbalancedBlocksReportC006(t *testing.T) {
	cases := []string{
		"{% if A %}x",
		"{% if A %}x{% endfor %}",
		"{% for a in A %}x",
		"{% for a in A %}x{% endif %}",
		"{% endif %}",
		"{% endfor %}",
		"{% else %}",
		"{% elif A %}",
	}
	for _, source := range cases {
		_, bag := parse(t, source)
		if !hasCode(bag, diag.C006) {
			t.Errorf("%q produced %v, want C006", source, codesOf(bag))
		}
	}
}

func TestStrayCloserExplainsWhatItCloses(t *testing.T) {
	_, bag := parse(t, "{% endfor %}")
	if help := bag.Items()[0].Help; help == "" {
		t.Error("a stray closer must explain what it would close")
	}
}

func TestMalformedForIsReported(t *testing.T) {
	for _, source := range []string{
		"{% for %}x{% endfor %}",
		"{% for 1 in A %}x{% endfor %}",
		"{% for a of A %}x{% endfor %}",
		"{% for a in %}x{% endfor %}",
	} {
		_, bag := parse(t, source)
		if !hasCode(bag, diag.C201) {
			t.Errorf("%q produced %v, want C201", source, codesOf(bag))
		}
	}
}

func TestMalformedLetIsReported(t *testing.T) {
	for _, source := range []string{"{% let %}", "{% let 1 = 2 %}", "{% let x 1 %}", "{% let x = %}"} {
		_, bag := parse(t, source)
		if !hasCode(bag, diag.C201) {
			t.Errorf("%q produced %v, want C201", source, codesOf(bag))
		}
	}
}

func TestUnclosedDirectivesReportC005(t *testing.T) {
	for _, source := range []string{"{% outlet", "{% if A %}x{% endif", "{% let x = 1", "{% for a in A"} {
		_, bag := parse(t, source)
		if !hasCode(bag, diag.C005) {
			t.Errorf("%q produced %v, want C005", source, codesOf(bag))
		}
	}
}

func TestParserRecoversInsideBlocks(t *testing.T) {
	doc, bag := parse(t, "{% if A %}{{ @ }}{% endif %}after")
	if bag.Len() == 0 {
		t.Fatal("expected a diagnostic")
	}
	var sawAfter bool
	Walk(doc.Nodes, func(node Node) {
		if text, ok := node.(*Text); ok && text.Value == "after" {
			sawAfter = true
		}
	})
	if !sawAfter {
		t.Error("the parser must keep going after an error inside a block")
	}
}

func TestMalformedComponentTagsReportC306(t *testing.T) {
	cases := map[string]string{
		"unclosed element":    "<Card>body",
		"unclosed tag":        "<Card",
		"bound without value": `<Card :Label />`,
		"value not quoted":    "<Card Label=x />",
		"stray closer":        "</Card>",
		"mismatched closer":   "<Card><p>x</p></Other>",
		"hash without name":   `<Card><Template #="x"></Template></Card>`,
		"template no slot":    "<Card><Template>x</Template></Card>",
		"junk in tag":         "<Card @ />",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := parse(t, source)
			if !hasCode(bag, diag.C306) {
				t.Errorf("%q produced %v, want C306", source, codesOf(bag))
			}
		})
	}
}

func TestComponentTagShapes(t *testing.T) {
	doc := parseClean(t, `<Card Title="t" :Count="N" Loud><p>body</p><Template #footer>end</Template></Card>`)
	node, ok := doc.Nodes[0].(*Component)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	if node.Name != "Card" || len(node.Attributes) != 3 {
		t.Fatalf("component = %+v", node)
	}
	if node.Attributes[0].Text != "t" || node.Attributes[0].Bound {
		t.Errorf("literal attribute = %+v", node.Attributes[0])
	}
	if !node.Attributes[1].Bound || render(node.Attributes[1].Value) != "N" {
		t.Errorf("bound attribute = %+v", node.Attributes[1])
	}
	if node.Attributes[2].Bound || node.Attributes[2].Text != "" {
		t.Errorf("bare attribute = %+v", node.Attributes[2])
	}
	if len(node.Children) != 3 {
		t.Errorf("children = %d, want the open tag, text and close tag", len(node.Children))
	}
	footer, ok := node.Slot("footer")
	if !ok || len(footer) != 1 {
		t.Errorf("slots = %+v", node.Slots)
	}
	if _, ok := node.Slot("missing"); ok {
		t.Error("Slot must not invent fills")
	}
}

func TestSelfClosingComponent(t *testing.T) {
	doc := parseClean(t, "<Rule />")
	node := doc.Nodes[0].(*Component)
	if len(node.Children) != 0 || len(node.Attributes) != 0 {
		t.Errorf("component = %+v", node)
	}
}

func TestLowercaseTagsBecomeElements(t *testing.T) {
	doc := parseClean(t, "<div class=\"x\">text</div>")
	element, ok := doc.Nodes[0].(*Element)
	if !ok || element.Name != "div" {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	if len(element.Attributes) != 1 || element.Attributes[0].Text != "x" {
		t.Errorf("attributes = %+v", element.Attributes)
	}
}

func TestBoundAttributeWithABrokenExpressionIsReported(t *testing.T) {
	_, bag := parse(t, `<Card :Label="1 +" />`)
	if !hasCode(bag, diag.C201) {
		t.Errorf("codes = %v, want C201", codesOf(bag))
	}
}

func TestBoundAttributeWithTrailingJunkIsReported(t *testing.T) {
	_, bag := parse(t, `<Card :Label="A B" />`)
	if !hasCode(bag, diag.C201) {
		t.Errorf("codes = %v, want C201", codesOf(bag))
	}
}

func TestSlotDirectiveNeedsAQuotedName(t *testing.T) {
	_, bag := parse(t, "{% slot footer %}")
	if !hasCode(bag, diag.C306) {
		t.Errorf("codes = %v, want C306", codesOf(bag))
	}
}

func TestChildrenAndSlotDirectivesParse(t *testing.T) {
	doc := parseClean(t, `{% children %}{% slot "footer" %}`)
	if _, ok := doc.Nodes[0].(*Children); !ok {
		t.Errorf("node = %#v", doc.Nodes[0])
	}
	outlet, ok := doc.Nodes[1].(*SlotOutlet)
	if !ok || outlet.Name != "footer" {
		t.Errorf("node = %#v", doc.Nodes[1])
	}
}

func TestMatchShape(t *testing.T) {
	doc := parseClean(t, "{% match S %}{% when A %}a{% when B %}b{% endmatch %}")
	node, ok := doc.Nodes[0].(*Match)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	if render(node.Subject) != "S" || len(node.Arms) != 2 {
		t.Errorf("match = %+v", node)
	}
	if node.Arms[0].Name != "A" || len(node.Arms[0].Body) != 1 {
		t.Errorf("first arm = %+v", node.Arms[0])
	}
}

func TestSpansOfComponentNodes(t *testing.T) {
	doc := parseClean(t, `<Card Title="t">{% children %}{% slot "f" %}</Card>{% match S %}{% when A %}x{% endmatch %}`)
	for _, node := range doc.Nodes {
		if node.NodeSpan().Len() == 0 {
			t.Errorf("%T reports an empty span", node)
		}
	}
	component := doc.Nodes[0].(*Component)
	for _, child := range component.Children {
		if child.NodeSpan().Len() == 0 {
			t.Errorf("%T inside a component reports an empty span", child)
		}
	}
}

func TestOutletIsFoundInsideComponentsAndMatches(t *testing.T) {
	for _, source := range []string{
		"<Card>{% outlet %}</Card>",
		"<Card><Template #f>{% outlet %}</Template></Card>",
		"{% match S %}{% when A %}{% outlet %}{% endmatch %}",
	} {
		if !parseClean(t, source).HasOutlet() {
			t.Errorf("%q hides its outlet", source)
		}
	}
}

func TestWalkReachesComponentsAndMatches(t *testing.T) {
	doc := parseClean(t, "<Card>a<Template #f>b</Template></Card>{% match S %}{% when A %}c{% endmatch %}")
	var texts int
	Walk(doc.Nodes, func(node Node) {
		if _, ok := node.(*Text); ok {
			texts++
		}
	})
	if texts != 3 {
		t.Errorf("walk saw %d texts, want 3", texts)
	}
}

func TestBoundAttributeSpansAreShifted(t *testing.T) {
	source := `<Card :A="Left + Right" />`
	doc := parseClean(t, source)
	attribute := doc.Nodes[0].(*Component).Attributes[0]
	binary := attribute.Value.(*Binary)

	for _, part := range []Expr{binary, binary.Left, binary.Right} {
		span := part.ExprSpan()
		if span.Start == 0 || int(span.End) > len(source) {
			t.Errorf("span %+v does not point into the attribute", span)
		}
	}
	if got := source[binary.Left.ExprSpan().Start:binary.Left.ExprSpan().End]; got != "Left" {
		t.Errorf("left operand covers %q", got)
	}
}

func TestBoundAttributeSpansForEveryExpressionShape(t *testing.T) {
	source := `<Card :A="-N" :B="'s'" :C="1" :D="1.5" :E="true" :F="X[0]" />`
	doc := parseClean(t, source)
	for _, attribute := range doc.Nodes[0].(*Component).Attributes {
		if attribute.Value.ExprSpan().Start == 0 {
			t.Errorf("%s keeps a span from the sub-parser", attribute.Name)
		}
	}
}

func TestUnclosedMatchAndStrayArmsAreReported(t *testing.T) {
	for _, source := range []string{
		"{% match S %}{% when A %}x",
		"{% match S %}{% when A %}x{% endif %}",
		"{% match %}{% endmatch %}",
		"{% match S %}{% when A %}x{% endmatch",
	} {
		_, bag := parse(t, source)
		if bag.Len() == 0 {
			t.Errorf("%q parsed without a diagnostic", source)
		}
	}
}

func TestSlotDirectiveIsClosed(t *testing.T) {
	_, bag := parse(t, `{% slot "f"`)
	if !hasCode(bag, diag.C005) {
		t.Errorf("codes = %v, want C005", codesOf(bag))
	}
}

func TestFenceRunToEndOfFile(t *testing.T) {
	doc, bag := parse(t, "---\nvar X = 1")
	if bag.Len() == 0 {
		t.Error("an unterminated fence must be reported")
	}
	if doc == nil {
		t.Error("Parse must still return a document")
	}
}

func TestTagNeedsALetter(t *testing.T) {
	doc := parseClean(t, "<1abc> and </> and <")
	if len(doc.Nodes) != 1 {
		t.Errorf("nodes = %d, want the whole thing as text", len(doc.Nodes))
	}
}

func TestClosingTagWithoutGtIsText(t *testing.T) {
	doc := parseClean(t, "</Card")
	if _, ok := doc.Nodes[0].(*Text); !ok {
		t.Errorf("node = %#v, want text", doc.Nodes[0])
	}
}

func TestElementCloseBecomesText(t *testing.T) {
	doc := parseClean(t, "</div>")
	text, ok := doc.Nodes[0].(*Text)
	if !ok || text.Value != "</div>" {
		t.Errorf("node = %#v", doc.Nodes[0])
	}
}

func TestStandaloneDirectivesParse(t *testing.T) {
	cases := map[string]func(Node) bool{
		"{% outlet %}":   func(n Node) bool { _, ok := n.(*Outlet); return ok },
		"{% children %}": func(n Node) bool { _, ok := n.(*Children); return ok },
		"{% meta %}":     func(n Node) bool { _, ok := n.(*MetaBlock); return ok },
		"{% assets %}":   func(n Node) bool { _, ok := n.(*AssetsBlock); return ok },
	}
	for source, matches := range cases {
		doc := parseClean(t, source)
		if len(doc.Nodes) != 1 || !matches(doc.Nodes[0]) {
			t.Errorf("%s parsed to %#v", source, doc.Nodes)
		}
		if doc.Nodes[0].NodeSpan().Start == doc.Nodes[0].NodeSpan().End {
			t.Errorf("%s carries an empty span", source)
		}
	}
}

func TestUnterminatedStandaloneDirectivesAreReported(t *testing.T) {
	for _, source := range []string{"{% outlet", "{% children", "{% meta", "{% assets"} {
		_, bag := parse(t, source)
		if !hasCode(bag, diag.C005) {
			t.Errorf("%q produced %v, want C005", source, codesOf(bag))
		}
	}
}

func TestFragmentParsesItsNameAndWindow(t *testing.T) {
	doc := parseClean(t, `{% fragment "reviews" cache="5m" stale="1h" %}<p>x</p>{% endfragment %}`)
	node, ok := doc.Nodes[0].(*Fragment)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	if node.Name != "reviews" || node.Cache != "5m" || node.Stale != "1h" {
		t.Errorf("fragment = %+v", node)
	}
	if len(node.Body) == 0 || node.NameSpan.Start <= node.Span.Start {
		t.Errorf("body = %d, name span = %+v", len(node.Body), node.NameSpan)
	}
}

func TestAFragmentNeedsNoSettings(t *testing.T) {
	doc := parseClean(t, `{% fragment "plain" %}x{% endfragment %}`)
	node := doc.Nodes[0].(*Fragment)
	if node.Cache != "" || node.Stale != "" {
		t.Errorf("fragment = %+v", node)
	}
}

func TestMalformedFragmentsAreReported(t *testing.T) {
	sources := []string{
		`{% fragment %}x{% endfragment %}`,
		`{% fragment reviews %}x{% endfragment %}`,
		`{% fragment "a" cache %}x{% endfragment %}`,
		`{% fragment "a" cache=5m %}x{% endfragment %}`,
		`{% fragment "a" wobble="1m" %}x{% endfragment %}`,
		`{% fragment "a" %}x`,
		`{% fragment "a" cache="1m"`,
	}
	for _, source := range sources {
		_, bag := parse(t, source)
		if !bag.HasErrors() {
			t.Errorf("%q was accepted", source)
		}
	}
}

func TestAStrayEndfragmentIsReported(t *testing.T) {
	_, bag := parse(t, "{% endfragment %}")
	if !hasCode(bag, diag.C006) {
		t.Errorf("codes = %v, want C006", codesOf(bag))
	}
	if !strings.Contains(bag.Items()[0].Help, "fragment") {
		t.Errorf("help = %q", bag.Items()[0].Help)
	}
}

func TestWalkVisitsFragmentBodies(t *testing.T) {
	doc := parseClean(t, `{% fragment "a" %}<p>{{ Title }}</p>{% endfragment %}`)
	seen := 0
	Walk(doc.Nodes, func(Node) { seen++ })
	if seen < 3 {
		t.Errorf("visited %d nodes, want the body too", seen)
	}
}

func TestWalkExprsReachesEveryPosition(t *testing.T) {
	source := `{% let a = One %}{% if Two %}{{ Three }}{% endif %}` +
		`{% for x in Four %}{% endfor %}{% match Five %}{% when A %}{% endmatch %}` +
		`<p :title="Six" class="{{ Seven }}" :class="{ 'x': Eight }">y</p>` +
		`<Badge :label="Nine">{{ Ten }}</Badge>` +
		`{{ Eleven | default(Twelve) }}{{ -Thirteen }}{{ Fourteen[Fifteen] }}`
	doc := parseClean(t, source)
	found := map[string]bool{}
	WalkExprs(doc.Nodes, func(expr Expr) {
		if path, ok := expr.(*Path); ok {
			found[path.Segments[0]] = true
		}
	})
	for _, want := range []string{"One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight",
		"Nine", "Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen"} {
		if !found[want] {
			t.Errorf("%s was never visited", want)
		}
	}
}
