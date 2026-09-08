package compile

import (
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func boxFiles(body, page string) map[string]string {
	return map[string]string{
		"components/Box/template.gopage": body,
		"components/Box/props.go":        "package Box\n\ntype Props struct {\n\tWide bool\n\tLabel string\n}\n",
		"app/page.gopage":                page,
	}
}

func TestAConditionAtTheRootKeepsTheSiblingsThatFollow(t *testing.T) {
	html, bag := renderApp(t, boxFiles(
		`{% if Wide %}<div class="wide">wide</div>{% else %}<div class="narrow">narrow</div>{% endif %}`,
		`<main><Box :Wide="true" Label="x" /><aside>side</aside></main>`,
	), runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if html != `<main><div class="wide">wide</div><aside>side</aside></main>` {
		t.Errorf("html = %q", html)
	}
}

func TestAConditionBeforeAClosingTagKeepsTheTag(t *testing.T) {
	html, bag := renderApp(t, map[string]string{
		"app/page.gopage": `<main><p>one</p>{% if false %}<p>two</p>{% endif %}</main><footer>end</footer>`,
	}, runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if html != `<main><p>one</p></main><footer>end</footer>` {
		t.Errorf("html = %q", html)
	}
}

func TestATakenBranchKeepsWhatFollowsIt(t *testing.T) {
	html, bag := renderApp(t, map[string]string{
		"app/page.gopage": `{% if true %}<p>yes</p>{% endif %}<p>after</p>`,
	}, runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if html != `<p>yes</p><p>after</p>` {
		t.Errorf("html = %q", html)
	}
}

func TestInterpolationInAComponentPropIsRejected(t *testing.T) {
	_, bag := renderApp(t, boxFiles(`<p>{{ Label }}</p>`, `<Box Label="{{ Title }}" />`), runtime.Empty{})
	item, ok := found(bag, diag.C326)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Help, `:Label="…"`) {
		t.Errorf("help = %q", item.Help)
	}
}

func TestALiteralPropIsStillAccepted(t *testing.T) {
	html, bag := renderApp(t, boxFiles(`<p>{{ Label }}</p>`, `<Box Label="plain" />`), runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if html != "<p>plain</p>" {
		t.Errorf("html = %q", html)
	}
}

func TestABoundBooleanAttributeIsRejected(t *testing.T) {
	_, bag := renderApp(t, map[string]string{
		"app/page.gopage": `<details :open="true"><summary>x</summary></details>`,
	}, runtime.Empty{})
	item, ok := found(bag, diag.C327)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Help, `open?="condition"`) {
		t.Errorf("help = %q", item.Help)
	}
}

func TestAConditionalBooleanAttributeIsTheWayThrough(t *testing.T) {
	html, bag := renderApp(t, map[string]string{
		"app/page.gopage": `<details open?="false"><summary>a</summary></details>` +
			`<input type="checkbox" checked?="true">`,
	}, runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if strings.Contains(html, "<details open>") || !strings.Contains(html, "checked") {
		t.Errorf("html = %q", html)
	}
}

func TestANonBooleanAttributeStillBinds(t *testing.T) {
	html, bag := renderApp(t, boxFiles(`<input :value="Label">`, `<Box Label="x" />`), runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(html, `value="x"`) {
		t.Errorf("html = %q", html)
	}
}

func TestALayoutWithALoaderIsRejected(t *testing.T) {
	_, bag := renderApp(t, map[string]string{
		"app/layout.gopage": "---\ntype Props struct{}\n\nfunc Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }\n---\n<main>{% outlet %}</main>",
		"app/page.gopage":   "<p>home</p>",
	}, runtime.Empty{})
	item, ok := found(bag, diag.C328)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Message, "Load") {
		t.Errorf("message = %q", item.Message)
	}
}

func TestALayoutWithoutAFrontmatterIsFine(t *testing.T) {
	_, bag := renderApp(t, map[string]string{
		"app/layout.gopage": "<main>{% outlet %}</main>",
		"app/page.gopage":   "<p>home</p>",
	}, runtime.Empty{})
	if bag.HasErrors() {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}

func TestAGetFormCarriesNoToken(t *testing.T) {
	html, bag := renderApp(t, map[string]string{
		"app/page.gopage": `<Form method="get" action="/search"><input name="q"></Form>`,
	}, runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if strings.Contains(html, "__csrf") {
		t.Errorf("html = %q, want no token on a read", html)
	}
	if !strings.Contains(html, `method="get"`) || !strings.Contains(html, `action="/search"`) {
		t.Errorf("html = %q", html)
	}
}

func TestAGetFormMayLiveInACachedFragment(t *testing.T) {
	_, bag := renderApp(t, map[string]string{
		"app/page.gopage": `{% fragment "filters" cache="1m" %}<Form method="get"><input name="q"></Form>{% endfragment %}`,
	}, runtime.Empty{})
	if _, ok := found(bag, diag.C503); ok {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}

func TestAPostFormInACachedFragmentIsStillRefused(t *testing.T) {
	_, bag := renderApp(t, map[string]string{
		"app/page.gopage": `{% fragment "signup" cache="1m" %}<Form><input name="q"></Form>{% endfragment %}`,
	}, runtime.Empty{})
	if _, ok := found(bag, diag.C503); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestMatchWorksOnStrings(t *testing.T) {
	html, bag := renderApp(t, boxFiles(
		`{% match Label %}{% when "wide" %}<p>wide</p>{% when "narrow" %}<p>narrow</p>{% else %}<p>other</p>{% endmatch %}`,
		`<Box Label="narrow" />`,
	), runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if html != "<p>narrow</p>" {
		t.Errorf("html = %q", html)
	}
}

func TestMatchFallsThroughToTheDefault(t *testing.T) {
	html, bag := renderApp(t, boxFiles(
		`{% match Label %}{% when "wide" %}<p>wide</p>{% else %}<p>other</p>{% endmatch %}<span>tail</span>`,
		`<Box Label="tall" />`,
	), runtime.Empty{})
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if html != "<p>other</p><span>tail</span>" {
		t.Errorf("html = %q", html)
	}
}

func TestAStringMatchWithoutADefaultIsRejected(t *testing.T) {
	_, bag := renderApp(t, boxFiles(
		`{% match Label %}{% when "wide" %}<p>wide</p>{% endmatch %}`,
		`<Box Label="wide" />`,
	), runtime.Empty{})
	if _, ok := found(bag, diag.C309); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestMixedCasesAreRejected(t *testing.T) {
	_, bag := renderApp(t, boxFiles(
		`{% match Label %}{% when "wide" %}<p>a</p>{% when Narrow %}<p>b</p>{% else %}<p>c</p>{% endmatch %}`,
		`<Box Label="wide" />`,
	), runtime.Empty{})
	if _, ok := found(bag, diag.C309); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestADuplicatedStringCaseIsRejected(t *testing.T) {
	_, bag := renderApp(t, boxFiles(
		`{% match Label %}{% when "wide" %}<p>a</p>{% when "wide" %}<p>b</p>{% else %}<p>c</p>{% endmatch %}`,
		`<Box Label="wide" />`,
	), runtime.Empty{})
	if _, ok := found(bag, diag.C309); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}

func TestQuotedCasesNeedAStringSubject(t *testing.T) {
	_, bag := renderApp(t, map[string]string{
		"components/Box/template.gopage": `{% match Count %}{% when "1" %}<p>one</p>{% else %}<p>more</p>{% endmatch %}`,
		"components/Box/props.go":        "package Box\n\ntype Props struct {\n\tCount int\n}\n",
		"app/page.gopage":                `<Box Count="2" />`,
	}, runtime.Empty{})
	if _, ok := found(bag, diag.C309); !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
}
