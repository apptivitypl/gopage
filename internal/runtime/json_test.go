package runtime

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/ir"
)

type card struct {
	name string
	tags []string
}

func (c card) Names() []string { return []string{"Name", "Tags"} }

func (c card) Get(path []string) (Value, bool) {
	if len(path) != 1 {
		return Nil(), false
	}
	switch path[0] {
	case "Name":
		return String(c.name), true
	case "Tags":
		return Seq(stringSeq(c.tags)), true
	default:
		return Nil(), false
	}
}

type stringSeq []string

func (s stringSeq) Len() int { return len(s) }

func (s stringSeq) At(i int) Value { return String(s[i]) }

func encode(t *testing.T, value Value) string {
	t.Helper()
	return string(AppendJSON(nil, value))
}

func TestJSONScalars(t *testing.T) {
	cases := map[string]Value{
		"null":  Nil(),
		`"ada"`: String("ada"),
		"42":    Int(42),
		"1.5":   Float(1.5),
		"true":  Bool(true),
		"false": Bool(false),
	}
	for want, value := range cases {
		if got := encode(t, value); got != want {
			t.Errorf("%+v = %s, want %s", value, got, want)
		}
	}
}

func TestJSONEscapesForAScriptTag(t *testing.T) {
	got := encode(t, String(`</script><b>"quoted"&\`+"\n\t"))
	for _, forbidden := range []string{"</script>", "<b>"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("json = %s, want %q escaped", got, forbidden)
		}
	}
	var back string
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("the output is not json: %v (%s)", err, got)
	}
	if back != `</script><b>"quoted"&\`+"\n\t" {
		t.Errorf("round trip = %q", back)
	}
}

func TestJSONKeepsUnicode(t *testing.T) {
	got := encode(t, String("żółw"))
	if got != `"żółw"` {
		t.Errorf("json = %s", got)
	}
}

func TestJSONSequences(t *testing.T) {
	if got := encode(t, Seq(Values{Int(1), String("a"), Nil()})); got != `[1,"a",null]` {
		t.Errorf("json = %s", got)
	}
	if got := encode(t, Seq(nil)); got != "null" {
		t.Errorf("json = %s", got)
	}
	if got := encode(t, Seq(Values{})); got != "[]" {
		t.Errorf("json = %s", got)
	}
}

func TestJSONObjects(t *testing.T) {
	got := encode(t, Object(card{name: "ada", tags: []string{"x", "y"}}))
	if got != `{"Name":"ada","Tags":["x","y"]}` {
		t.Errorf("json = %s", got)
	}
}

func TestAnObjectWithoutNamesIsNull(t *testing.T) {
	if got := encode(t, Object(Map{"A": Int(1)})); got != "null" {
		t.Errorf("json = %s, want null for a value that cannot list its fields", got)
	}
}

func TestJSONRefusesToRecurseForever(t *testing.T) {
	nested := Values{}
	for range maxJSONDepth + 4 {
		nested = Values{Seq(nested)}
	}
	got := encode(t, Seq(nested))
	if !strings.Contains(got, "null") {
		t.Errorf("json = %s, want the deepest level cut off", got)
	}
}

func TestJSONRefusesNotANumber(t *testing.T) {
	if got := encode(t, Float(math.NaN())); got != "null" {
		t.Errorf("json = %s", got)
	}
}

func TestJSONHandlesUnrepresentableNumbers(t *testing.T) {
	if got := encode(t, Float(math.Inf(1))); got != "null" {
		t.Errorf("json = %s", got)
	}
}

func TestBufferWritesJSON(t *testing.T) {
	out := Acquire(16)
	defer Release(out)
	out.WriteJSON(String("ada"))
	if out.String() != `"ada"` {
		t.Errorf("buffer = %s", out.String())
	}
}

func TestJSONEscapesControlCharactersAndReturns(t *testing.T) {
	got := encode(t, String("a\rb\x01c"))
	if !strings.Contains(got, `\r`) || !strings.Contains(got, `\u0001`) {
		t.Errorf("json = %s", got)
	}
}

func TestAnUnknownValueKindIsNull(t *testing.T) {
	if got := encode(t, Value{Kind: 99}); got != "null" {
		t.Errorf("json = %s", got)
	}
}

func TestTheJSONOpWritesThroughThePlan(t *testing.T) {
	chain := []*ir.Plan{{
		Ops:      []ir.Op{{Kind: ir.OpJSON, A: 0}},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{"Tags"}},
		Capacity: 32,
	}}
	out := Acquire(Capacity(chain))
	defer Release(out)
	props := Map{"Tags": Seq(Values{String("a")})}
	if err := Render(chain, props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.String() != `["a"]` {
		t.Errorf("html = %s", out.String())
	}
}

func TestTheJSONOpReportsAFailingExpression(t *testing.T) {
	chain := []*ir.Plan{{
		Ops:      []ir.Op{{Kind: ir.OpJSON, A: 0}},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{"Missing"}},
		Capacity: 8,
	}}
	out := Acquire(8)
	defer Release(out)
	if err := Render(chain, Empty{}, out); err == nil {
		t.Error("a missing path must be reported")
	}
}
