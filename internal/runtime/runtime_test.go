package runtime

import (
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/ir"
)

func planOf(blob string, ops []ir.Op, paths [][]string) *ir.Plan {
	plan := &ir.Plan{Ops: ops, Blob: []byte(blob), Paths: paths, Capacity: uint32(len(blob) * 2)}
	for i := range paths {
		plan.Exprs = append(plan.Exprs, ir.ExprNode{Kind: ir.ExprPath, A: uint32(i)})
	}
	return plan
}

func render(t *testing.T, chain []*ir.Plan, props Accessible) string {
	t.Helper()
	out := NewBuffer(64)
	if err := Render(chain, props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestValueText(t *testing.T) {
	cases := []struct {
		value Value
		want  string
	}{
		{String("hi"), "hi"},
		{Int(-42), "-42"},
		{Bool(true), "true"},
		{Bool(false), "false"},
		{Nil(), ""},
		{Value{Kind: 200}, ""},
	}
	for _, c := range cases {
		if got := c.value.Text(); got != c.want {
			t.Errorf("Value%+v.Text() = %q, want %q", c.value, got, c.want)
		}
	}
}

func TestMapLookup(t *testing.T) {
	m := Map{"Title": String("hi")}
	if value, ok := m.Get([]string{"Title"}); !ok || value.Str != "hi" {
		t.Errorf("Get = %+v, %v", value, ok)
	}
	if _, ok := m.Get([]string{"Missing"}); ok {
		t.Error("Get must report missing fields")
	}
	if _, ok := m.Get([]string{"A", "B"}); ok {
		t.Error("the map accessor handles single segments only")
	}
}

func TestEmptyAccessorHasNoFields(t *testing.T) {
	if _, ok := (Empty{}).Get([]string{"Anything"}); ok {
		t.Error("Empty must never resolve a field")
	}
}

func TestAppendEscapedCoversEveryDangerousByte(t *testing.T) {
	got := string(AppendEscaped(nil, `<a href="x">&'</a>`))
	want := "&lt;a href=&#34;x&#34;&gt;&amp;&#39;&lt;/a&gt;"
	if got != want {
		t.Errorf("AppendEscaped =\n%q\nwant\n%q", got, want)
	}
}

func TestAppendEscapedLeavesPlainTextAlone(t *testing.T) {
	const plain = "a plain sentence, 123"
	if got := string(AppendEscaped(nil, plain)); got != plain {
		t.Errorf("AppendEscaped = %q", got)
	}
}

func TestAppendEscapedNeverEmitsRawAngleBrackets(t *testing.T) {
	for _, input := range []string{"<", ">", "<<>>", "a<b>c", "", "\x00<"} {
		got := string(AppendEscaped(nil, input))
		if strings.ContainsAny(got, "<>") {
			t.Errorf("AppendEscaped(%q) = %q leaked a bracket", input, got)
		}
	}
}

func TestBufferAccumulates(t *testing.T) {
	b := NewBuffer(4)
	b.Write([]byte("ab"))
	b.WriteEscaped("<c>")
	if b.String() != "ab&lt;c&gt;" {
		t.Errorf("buffer = %q", b.String())
	}
	if b.Len() != len(b.Bytes()) {
		t.Error("Len and Bytes disagree")
	}
	b.Reset()
	if b.Len() != 0 {
		t.Errorf("after Reset the buffer holds %d bytes", b.Len())
	}
}

func TestPoolReusesBuffersAndGrowsThem(t *testing.T) {
	small := Acquire(8)
	small.Write([]byte("x"))
	Release(small)

	large := Acquire(4096)
	if cap(large.Bytes()) < 4096 {
		t.Errorf("capacity = %d, want at least 4096", cap(large.Bytes()))
	}
	if large.Len() != 0 {
		t.Errorf("a pooled buffer must come back empty, got %d bytes", large.Len())
	}
	Release(large)
}

func TestRenderStaticOnly(t *testing.T) {
	plan := planOf("<h1>hi</h1>", []ir.Op{{Kind: ir.OpStatic, A: 0, B: 11}}, nil)
	if got := render(t, []*ir.Plan{plan}, nil); got != "<h1>hi</h1>" {
		t.Errorf("render = %q", got)
	}
}

func TestRenderEscapesInterpolatedText(t *testing.T) {
	plan := planOf("", []ir.Op{{Kind: ir.OpText, A: 0}}, [][]string{{"Title"}})
	got := render(t, []*ir.Plan{plan}, Map{"Title": String("<script>")})
	if got != "&lt;script&gt;" {
		t.Errorf("render = %q, want the value escaped", got)
	}
}

func TestRenderWalksTheLayoutChain(t *testing.T) {
	layout := planOf("<main></main>", []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 6},
		{Kind: ir.OpOutlet},
		{Kind: ir.OpStatic, A: 6, B: 7},
	}, nil)
	page := planOf("<p>x</p>", []ir.Op{{Kind: ir.OpStatic, A: 0, B: 8}}, nil)

	if got := render(t, []*ir.Plan{layout, page}, nil); got != "<main><p>x</p></main>" {
		t.Errorf("render = %q", got)
	}
}

func TestOutletWithoutAPageRendersNothingExtra(t *testing.T) {
	layout := planOf("<main></main>", []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 6},
		{Kind: ir.OpOutlet},
	}, nil)
	if got := render(t, []*ir.Plan{layout}, nil); got != "<main>" {
		t.Errorf("render = %q", got)
	}
}

func TestRenderOfAnEmptyChainIsEmpty(t *testing.T) {
	if got := render(t, nil, nil); got != "" {
		t.Errorf("render = %q", got)
	}
}

func TestRenderRejectsAnUnknownOp(t *testing.T) {
	plan := planOf("", []ir.Op{{Kind: ir.OpKind(200)}}, nil)
	if err := Render([]*ir.Plan{plan}, nil, NewBuffer(8)); err == nil {
		t.Error("expected an error for an unknown op")
	}
}

func TestRenderRejectsADanglingExpression(t *testing.T) {
	plan := planOf("", []ir.Op{{Kind: ir.OpText, A: 7}}, nil)
	err := Render([]*ir.Plan{plan}, Map{}, NewBuffer(8))
	if err == nil || !strings.Contains(err.Error(), "not in the plan") {
		t.Errorf("err = %v", err)
	}
}

func TestRenderRejectsMissingProps(t *testing.T) {
	plan := planOf("", []ir.Op{{Kind: ir.OpText, A: 0}}, [][]string{{"Title"}})

	err := Render([]*ir.Plan{plan}, nil, NewBuffer(8))
	if err == nil || !strings.Contains(err.Error(), "no props") {
		t.Errorf("err = %v, want it to name the missing props", err)
	}

	err = Render([]*ir.Plan{plan}, Map{}, NewBuffer(8))
	if err == nil || !strings.Contains(err.Error(), "Title") {
		t.Errorf("err = %v, want it to name the missing field", err)
	}
}

func TestErrorsFromANestedPlanReachTheCaller(t *testing.T) {
	layout := planOf("", []ir.Op{{Kind: ir.OpOutlet}}, nil)
	page := planOf("", []ir.Op{{Kind: ir.OpKind(200)}}, nil)
	if err := Render([]*ir.Plan{layout, page}, nil, NewBuffer(8)); err == nil {
		t.Error("an error inside the page must not be swallowed by the layout")
	}
}

func TestCapacitySumsTheChain(t *testing.T) {
	a := &ir.Plan{Capacity: 10}
	b := &ir.Plan{Capacity: 32}
	if got := Capacity([]*ir.Plan{a, b}); got != 42 {
		t.Errorf("Capacity = %d, want 42", got)
	}
}

func benchChain() []*ir.Plan {
	layout := planOf("<!doctype html><html><body></body></html>", []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 27},
		{Kind: ir.OpOutlet},
		{Kind: ir.OpStatic, A: 27, B: 14},
	}, nil)
	page := planOf("<h1></h1><p>a paragraph of body text</p>", []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 4},
		{Kind: ir.OpText, A: 0},
		{Kind: ir.OpStatic, A: 4, B: 36},
	}, [][]string{{"Title"}})
	return []*ir.Plan{layout, page}
}

func BenchmarkRender(b *testing.B) {
	chain := benchChain()
	props := Map{"Title": String("A page title")}
	capacity := Capacity(chain)
	b.ReportAllocs()
	for b.Loop() {
		out := Acquire(capacity)
		if err := Render(chain, props, out); err != nil {
			b.Fatal(err)
		}
		Release(out)
	}
}

func BenchmarkAppendEscaped(b *testing.B) {
	const input = "a title with <angle> brackets & an ampersand"
	buf := make([]byte, 0, 256)
	b.ReportAllocs()
	for b.Loop() {
		buf = AppendEscaped(buf[:0], input)
	}
}

func TestLeafReadsAsAValue(t *testing.T) {
	leaf := Leaf("hello")
	value, ok := leaf.Get(nil)
	if !ok || value.Str != "hello" {
		t.Errorf("value = %+v, ok = %v", value, ok)
	}
	if _, ok := leaf.Get([]string{"Field"}); ok {
		t.Error("a leaf has no fields")
	}
}

func TestWithRootPlacesAValueBesideTheProps(t *testing.T) {
	props := WithRoot(Empty{}, "flash", Leaf("sent"))
	value, ok := props.Get([]string{"flash"})
	if !ok || value.Str != "sent" {
		t.Errorf("value = %+v, ok = %v", value, ok)
	}
	if _, ok := props.Get([]string{"other"}); ok {
		t.Error("other paths fall through to the props")
	}
	if _, ok := WithRoot(nil, "flash", Leaf("x")).Get([]string{"other"}); ok {
		t.Error("a nil props layer answers nothing")
	}
}
