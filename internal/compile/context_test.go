package compile

import (
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func contextCode(t *testing.T, body string, want diag.Code) diag.Diagnostic {
	t.Helper()
	bag := typed(t, body)
	for _, item := range bag.Items() {
		if item.Code == want {
			return item
		}
	}
	t.Fatalf("%q produced %+v, want %s", body, bag.Items(), want)
	return diag.Diagnostic{}
}

func TestAValueInAScriptBodyIsRejected(t *testing.T) {
	item := contextCode(t, `<script>const title = "{{ Title }}";</script>`, diag.C321)
	if !strings.Contains(item.Help, "island") {
		t.Errorf("help = %q, want the island route offered", item.Help)
	}
}

func TestAValueInAStyleBodyIsRejected(t *testing.T) {
	contextCode(t, `<style>.hero { color: {{ Title }} }</style>`, diag.C322)
}

func TestAValueInAStyleAttributeIsRejected(t *testing.T) {
	contextCode(t, `<div style="color: {{ Title }}">x</div>`, diag.C322)
	contextCode(t, `<div :style="Title">x</div>`, diag.C322)
}

func TestAValueInAnEventHandlerIsRejected(t *testing.T) {
	item := contextCode(t, `<button :onclick="Title">x</button>`, diag.C323)
	if !strings.Contains(item.Message, "onclick") {
		t.Errorf("message = %q, want the attribute named", item.Message)
	}
	contextCode(t, `<button onclick="pick('{{ Title }}')">x</button>`, diag.C323)
	contextCode(t, `<div ONMOUSEOVER="{{ Title }}">x</div>`, diag.C323)
}

func TestAValueInSrcdocIsRejected(t *testing.T) {
	contextCode(t, `<iframe :srcdoc="Title"></iframe>`, diag.C324)
}

func TestTheScriptContextEndsWithTheClosingTag(t *testing.T) {
	accepts(t, `<script>const a = 1;</script><p>{{ Title }}</p>`)
}

func TestAStaticScriptBodyIsAccepted(t *testing.T) {
	accepts(t, `<script>console.log("plain")</script>`)
	accepts(t, `<style>.hero { color: red }</style>`)
	accepts(t, `<div style="color: red">x</div>`)
	accepts(t, `<button onclick="pick()">x</button>`)
}

func TestAnAttributeNamedLikeAHandlerButNotOneIsAccepted(t *testing.T) {
	accepts(t, `<div :on="Title">x</div>`)
}

func TestAScriptContextSurvivesABranch(t *testing.T) {
	contextCode(t, `<script>{% if Count > 0 %}const n = {{ Count }};{% endif %}</script>`, diag.C321)
}

func TestClassMapNamesAreEscaped(t *testing.T) {
	props := runtime.Map{"Count": runtime.Int(1)}
	got := mustRender(t, `<div :class="{'a<img src=x>': Count > 0}">x</div>`, props)
	if strings.Contains(got, "<img") {
		t.Errorf("render = %q, want the markup escaped", got)
	}
	if !strings.Contains(got, "a&lt;img src=x&gt;") {
		t.Errorf("render = %q", got)
	}
}

func TestABoundURLAttributeDropsAJavascriptScheme(t *testing.T) {
	props := runtime.Map{"Badge": runtime.String("javascript:alert(document.cookie)")}
	got := mustRender(t, `<a :href="Badge">go</a>`, props)
	if strings.Contains(got, "javascript:") {
		t.Errorf("render = %q, want the scheme dropped", got)
	}
	if got != `<a href="">go</a>` {
		t.Errorf("render = %q", got)
	}
}

func TestALeadingInterpolationInAURLIsFiltered(t *testing.T) {
	props := runtime.Map{"Badge": runtime.String("javascript:alert(1)")}
	got := mustRender(t, `<img src="{{ Badge }}">`, props)
	if strings.Contains(got, "javascript:") {
		t.Errorf("render = %q, want the scheme dropped", got)
	}
}

func TestURLAttributesKeepWhatIsSafe(t *testing.T) {
	props := runtime.Map{"Badge": runtime.String("https://example.test/a?x=1&y=2")}
	got := mustRender(t, `<a :href="Badge">go</a>`, props)
	if got != `<a href="https://example.test/a?x=1&amp;y=2">go</a>` {
		t.Errorf("render = %q", got)
	}
}

func TestAStaticPrefixKeepsTheAttributeATextContext(t *testing.T) {
	props := runtime.Map{"Title": runtime.String("a b")}
	got := mustRender(t, `<a href="!/listings/{{ Title }}">go</a>`, props)
	if got != `<a href="/listings/a b">go</a>` {
		t.Errorf("render = %q", got)
	}
}

func TestDataDashAttributesAreNotTreatedAsURLs(t *testing.T) {
	props := runtime.Map{"Badge": runtime.String("javascript:alert(1)")}
	got := mustRender(t, `<div :data-note="Badge">x</div>`, props)
	if !strings.Contains(got, "javascript:alert(1)") {
		t.Errorf("render = %q, want data- left alone", got)
	}
}
