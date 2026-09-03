package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/runtime"
)

type row struct {
	fields runtime.Map
}

func (r row) Get(path []string) (runtime.Value, bool) {
	return r.fields.Get(path)
}

func rows(values ...runtime.Map) runtime.Value {
	items := make(runtime.Values, 0, len(values))
	for _, value := range values {
		items = append(items, runtime.Object(row{fields: value}))
	}
	return runtime.Seq(items)
}

func numbers(values ...int64) runtime.Value {
	items := make(runtime.Values, 0, len(values))
	for _, value := range values {
		items = append(items, runtime.Int(value))
	}
	return runtime.Seq(items)
}

func renderPage(t *testing.T, body string, props runtime.Accessible) (string, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": file(body)}, &bag)
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
	out := runtime.NewBuffer(256)
	if err := runtime.Render(result.Manifest.Chain(route), props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String(), &bag
}

func mustRender(t *testing.T, body string, props runtime.Accessible) string {
	t.Helper()
	out, bag := renderPage(t, body, props)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	return out
}

func renderFails(t *testing.T, body string, props runtime.Accessible) error {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{"app/page.rill": file(body)}, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", bag.Items())
	}
	route, _ := result.Manifest.Lookup("/")
	return runtime.Render(result.Manifest.Chain(route), props, runtime.NewBuffer(64))
}

func TestArithmetic(t *testing.T) {
	cases := map[string]string{
		"{{ 2 + 3 }}":       "5",
		"{{ 10 - 4 }}":      "6",
		"{{ 6 * 7 }}":       "42",
		"{{ 9 / 2 }}":       "4",
		"{{ 9 % 2 }}":       "1",
		"{{ -3 }}":          "-3",
		"{{ 2 + 3 * 4 }}":   "14",
		"{{ (2 + 3) * 4 }}": "20",
		"{{ 1.5 + 1.5 }}":   "3",
		"{{ 7 / 2.0 }}":     "3.5",
	}
	for body, want := range cases {
		if got := mustRender(t, body, nil); got != want {
			t.Errorf("%s = %q, want %q", body, got, want)
		}
	}
}

func TestComparisonAndLogic(t *testing.T) {
	cases := map[string]string{
		"{{ 1 < 2 }}":          "true",
		"{{ 2 <= 2 }}":         "true",
		"{{ 3 > 4 }}":          "false",
		"{{ 3 >= 3 }}":         "true",
		"{{ 1 == 1 }}":         "true",
		"{{ 1 != 1 }}":         "false",
		"{{ 'a' < 'b' }}":      "true",
		"{{ 'a' == 'a' }}":     "true",
		"{{ true && false }}":  "false",
		"{{ true || false }}":  "true",
		"{{ !true }}":          "false",
		"{{ 1 < 2 && 2 < 3 }}": "true",
		"{{ 1 == 1.0 }}":       "true",
		"{{ 'a' == 1 }}":       "false",
	}
	for body, want := range cases {
		if got := mustRender(t, body, nil); got != want {
			t.Errorf("%s = %q, want %q", body, got, want)
		}
	}
}

func TestConcatenationJoinsAnyValue(t *testing.T) {
	if got := mustRender(t, "{{ 'n=' ~ 42 }}", nil); got != "n=42" {
		t.Errorf("concat = %q", got)
	}
}

func TestInterpolationEscapesTheResult(t *testing.T) {
	got := mustRender(t, "{{ Title }}", runtime.Map{"Title": runtime.String("<b>&</b>")})
	if got != "&lt;b&gt;&amp;&lt;/b&gt;" {
		t.Errorf("render = %q", got)
	}
}

func TestPropsPathReachesNestedStructs(t *testing.T) {
	props := runtime.Map{"Listing": runtime.Object(row{fields: runtime.Map{"Title": runtime.String("Chair")}})}
	if got := mustRender(t, "{{ Listing.Title }}", props); got != "Chair" {
		t.Errorf("render = %q", got)
	}
}

func TestIndexReadsASequence(t *testing.T) {
	props := runtime.Map{"Scores": numbers(10, 20, 30)}
	if got := mustRender(t, "{{ Scores[1] }}", props); got != "20" {
		t.Errorf("render = %q", got)
	}
}

func TestIfChoosesOneBranch(t *testing.T) {
	const body = "{% if N > 10 %}big{% elif N > 5 %}medium{% else %}small{% endif %}"
	cases := map[int64]string{20: "big", 7: "medium", 1: "small"}
	for value, want := range cases {
		got := mustRender(t, body, runtime.Map{"N": runtime.Int(value)})
		if got != want {
			t.Errorf("N=%d rendered %q, want %q", value, got, want)
		}
	}
}

func TestIfWithoutElseRendersNothing(t *testing.T) {
	if got := mustRender(t, "[{% if false %}x{% endif %}]", nil); got != "[]" {
		t.Errorf("render = %q", got)
	}
}

func TestNestedIf(t *testing.T) {
	const body = "{% if A %}{% if B %}both{% else %}first{% endif %}{% else %}none{% endif %}"
	props := runtime.Map{"A": runtime.Bool(true), "B": runtime.Bool(false)}
	if got := mustRender(t, body, props); got != "first" {
		t.Errorf("render = %q", got)
	}
}

func TestForLoopsOverASequence(t *testing.T) {
	props := runtime.Map{"Items": numbers(1, 2, 3)}
	if got := mustRender(t, "{% for n in Items %}[{{ n }}]{% endfor %}", props); got != "[1][2][3]" {
		t.Errorf("render = %q", got)
	}
}

func TestForReadsFieldsOfEachElement(t *testing.T) {
	props := runtime.Map{"Cards": rows(
		runtime.Map{"Title": runtime.String("one")},
		runtime.Map{"Title": runtime.String("two")},
	)}
	got := mustRender(t, "{% for c in Cards %}<p>{{ c.Title }}</p>{% endfor %}", props)
	if got != "<p>one</p><p>two</p>" {
		t.Errorf("render = %q", got)
	}
}

func TestForElseRunsOnAnEmptySequence(t *testing.T) {
	props := runtime.Map{"Items": runtime.Seq(runtime.Values{})}
	got := mustRender(t, "{% for n in Items %}[{{ n }}]{% else %}nothing{% endfor %}", props)
	if got != "nothing" {
		t.Errorf("render = %q", got)
	}
}

func TestForElseIsSkippedWhenItemsExist(t *testing.T) {
	props := runtime.Map{"Items": numbers(7)}
	got := mustRender(t, "{% for n in Items %}[{{ n }}]{% else %}nothing{% endfor %}", props)
	if got != "[7]" {
		t.Errorf("render = %q", got)
	}
}

func TestNestedLoops(t *testing.T) {
	props := runtime.Map{"Outer": numbers(1, 2), "Inner": numbers(3, 4)}
	got := mustRender(t, "{% for a in Outer %}{% for b in Inner %}{{ a }}{{ b }} {% endfor %}{% endfor %}", props)
	if got != "13 14 23 24 " {
		t.Errorf("render = %q", got)
	}
}

func TestLetBindsAValue(t *testing.T) {
	props := runtime.Map{"Price": runtime.Int(100)}
	got := mustRender(t, "{% let doubled = Price * 2 %}{{ doubled }}", props)
	if got != "200" {
		t.Errorf("render = %q", got)
	}
}

func TestLetInsideALoopSeesTheLoopVariable(t *testing.T) {
	props := runtime.Map{"Items": numbers(2, 3)}
	got := mustRender(t, "{% for n in Items %}{% let sq = n * n %}{{ sq }} {% endfor %}", props)
	if got != "4 9 " {
		t.Errorf("render = %q", got)
	}
}

func TestLoopVariableShadowsAPropsField(t *testing.T) {
	props := runtime.Map{"Items": numbers(9), "n": runtime.Int(1)}
	if got := mustRender(t, "{% for n in Items %}{{ n }}{% endfor %}", props); got != "9" {
		t.Errorf("render = %q, want the loop variable to win", got)
	}
}

func TestLoopVariableIsGoneAfterTheLoop(t *testing.T) {
	props := runtime.Map{"Items": numbers(9), "n": runtime.String("from props")}
	got := mustRender(t, "{% for n in Items %}{{ n }}{% endfor %}{{ n }}", props)
	if got != "9from props" {
		t.Errorf("render = %q", got)
	}
}

func TestDivisionByZeroIsARenderError(t *testing.T) {
	err := renderFails(t, "{{ 1 / 0 }}", nil)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("err = %v", err)
	}
}

func TestLoopingOverANonSequenceIsARenderError(t *testing.T) {
	err := renderFails(t, "{% for n in Title %}x{% endfor %}", runtime.Map{"Title": runtime.String("no")})
	if err == nil || !strings.Contains(err.Error(), "cannot loop") {
		t.Errorf("err = %v", err)
	}
}

func TestIndexOutOfRangeIsARenderError(t *testing.T) {
	err := renderFails(t, "{{ Items[9] }}", runtime.Map{"Items": numbers(1)})
	if err == nil || !strings.Contains(err.Error(), "outside the sequence") {
		t.Errorf("err = %v", err)
	}
}

func TestComparingIncompatibleValuesIsARenderError(t *testing.T) {
	props := runtime.Map{"Title": runtime.String("x"), "N": runtime.Int(1)}
	err := renderFails(t, "{{ Title < N }}", props)
	if err == nil || !strings.Contains(err.Error(), "cannot compare") {
		t.Errorf("err = %v", err)
	}
}

func TestFieldAccessOnAScalarIsARenderError(t *testing.T) {
	props := runtime.Map{"Items": numbers(1)}
	err := renderFails(t, "{% for n in Items %}{{ n.Title }}{% endfor %}", props)
	if err == nil || !strings.Contains(err.Error(), "no fields") {
		t.Errorf("err = %v", err)
	}
}

func TestUnbalancedBlockReportsC006(t *testing.T) {
	for _, body := range []string{
		"{% if A %}x",
		"{% for n in Items %}x",
		"{% endif %}",
		"{% if A %}x{% endfor %}",
	} {
		var bag diag.Bag
		if _, err := Compile(fstest.MapFS{"app/page.rill": file(body)}, &bag); err != nil {
			t.Fatalf("Compile: %v", err)
		}
		if !hasCode(&bag, diag.C006) {
			t.Errorf("%q produced %+v, want C006", body, bag.Items())
		}
	}
}

func TestMalformedDirectivesAreReported(t *testing.T) {
	cases := map[string]diag.Code{
		"{% for %}x{% endfor %}":           diag.C201,
		"{% for n of Items %}{% endfor %}": diag.C201,
		"{% let %}":                        diag.C201,
		"{% let x 1 %}":                    diag.C201,
		"{{ (1 + 2 }}":                     diag.C202,
		"{{ Items[0 }}":                    diag.C202,
	}
	for body, want := range cases {
		var bag diag.Bag
		if _, err := Compile(fstest.MapFS{"app/page.rill": file(body)}, &bag); err != nil {
			t.Fatalf("Compile: %v", err)
		}
		if !hasCode(&bag, want) {
			t.Errorf("%q produced %+v, want %s", body, bag.Items(), want)
		}
	}
}

func TestConstantsAreDeduplicated(t *testing.T) {
	var bag diag.Bag
	result, _ := Compile(fstest.MapFS{"app/page.rill": file("{{ 'x' }}{{ 'x' }}{{ 'y' }}")}, &bag)
	if got := len(result.Manifest.Plans[0].Consts); got != 2 {
		t.Errorf("constants = %d, want x and y once each", got)
	}
}
