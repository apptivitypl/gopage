package syntax

import (
	"fmt"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
)

func exprOf(t *testing.T, source string) Expr {
	t.Helper()
	doc := parseClean(t, "{{ "+source+" }}")
	interp, ok := doc.Nodes[0].(*Interpolation)
	if !ok {
		t.Fatalf("node = %#v", doc.Nodes[0])
	}
	return interp.Expr
}

func render(e Expr) string {
	switch n := e.(type) {
	case *Path:
		return strings.Join(n.Segments, ".")
	case *StringLit:
		return fmt.Sprintf("%q", n.Value)
	case *IntLit:
		return fmt.Sprintf("%d", n.Value)
	case *FloatLit:
		return fmt.Sprintf("%g", n.Value)
	case *BoolLit:
		return fmt.Sprintf("%t", n.Value)
	case *Unary:
		return "(" + n.Op.String() + render(n.Operand) + ")"
	case *Binary:
		return "(" + render(n.Left) + " " + n.Op.String() + " " + render(n.Right) + ")"
	case *Index:
		return render(n.Base) + "[" + render(n.Index) + "]"
	case *MessageCall:
		if n.Count == nil {
			return fmt.Sprintf("t(%q)", n.Key)
		}
		return fmt.Sprintf("t(%q, %s)", n.Key, render(n.Count))
	case *FilterCall:
		if n.Argument == nil {
			return "(" + render(n.Input) + " | " + n.Name + ")"
		}
		return "(" + render(n.Input) + " | " + n.Name + "(" + render(n.Argument) + "))"
	default:
		return "?"
	}
}

func TestExpressionShapes(t *testing.T) {
	cases := map[string]string{
		"A":      "A",
		"A.B.C":  "A.B.C",
		"'text'": `"text"`,
		`"text"`: `"text"`,
		"42":     "42",
		"1.5":    "1.5",
		"true":   "true",
		"false":  "false",
		"!A":     "(!A)",
		"-A":     "(-A)",
		"A + B":  "(A + B)",
		"A[0]":   "A[0]",
		"A[B]":   "A[B]",
		"A.B[0]": "A.B[0]",
	}
	for source, want := range cases {
		if got := render(exprOf(t, source)); got != want {
			t.Errorf("%s parsed as %s, want %s", source, got, want)
		}
	}
}

func TestOperatorPrecedence(t *testing.T) {
	cases := map[string]string{
		"1 + 2 * 3":        "(1 + (2 * 3))",
		"1 * 2 + 3":        "((1 * 2) + 3)",
		"(1 + 2) * 3":      "((1 + 2) * 3)",
		"1 + 2 ~ 3":        "((1 + 2) ~ 3)",
		"1 < 2 && 3 < 4":   "((1 < 2) && (3 < 4))",
		"A || B && C":      "(A || (B && C))",
		"A == B || C == D": "((A == B) || (C == D))",
		"1 - 2 - 3":        "((1 - 2) - 3)",
		"!A && B":          "((!A) && B)",
		"-A + B":           "((-A) + B)",
	}
	for source, want := range cases {
		if got := render(exprOf(t, source)); got != want {
			t.Errorf("%s parsed as %s, want %s", source, got, want)
		}
	}
}

func TestEveryOperatorParses(t *testing.T) {
	for _, op := range []string{"||", "&&", "==", "!=", "<", "<=", ">", ">=", "~", "+", "-", "*", "/", "%"} {
		source := "A " + op + " B"
		binary, ok := exprOf(t, source).(*Binary)
		if !ok {
			t.Fatalf("%s did not parse as a binary expression", source)
		}
		if binary.Op.String() != op {
			t.Errorf("%s parsed with operator %s", source, binary.Op)
		}
	}
}

func TestExpressionSpansCoverTheSource(t *testing.T) {
	expression := exprOf(t, "A + B")
	span := expression.ExprSpan()
	if span.Start != 3 || span.End != 8 {
		t.Errorf("span = %+v, want it to cover the whole expression", span)
	}
}

func TestMalformedExpressionsReportC201(t *testing.T) {
	for _, source := range []string{"{{ + }}", "{{ A + }}", "{{ A. }}", "{{ 'x' + }}", "{{ !}}"} {
		_, bag := parse(t, source)
		if !hasCode(bag, diag.C201) {
			t.Errorf("%q produced %v, want C201", source, codesOf(bag))
		}
	}
}

func TestUnclosedGroupsReportC202(t *testing.T) {
	for _, source := range []string{"{{ (1 + 2 }}", "{{ A[0 }}", "{{ (A }}"} {
		_, bag := parse(t, source)
		if !hasCode(bag, diag.C202) {
			t.Errorf("%q produced %v, want C202", source, codesOf(bag))
		}
	}
}

func TestNumberOutOfRangeIsReported(t *testing.T) {
	_, bag := parse(t, "{{ 99999999999999999999 }}")
	if !hasCode(bag, diag.C201) {
		t.Errorf("codes = %v, want C201", codesOf(bag))
	}
}

func TestUnterminatedStringIsReported(t *testing.T) {
	_, bag := parse(t, "{{ 'open }}")
	if bag.Len() == 0 {
		t.Error("an unterminated string must be reported")
	}
}

func TestPrecedenceTableCoversEveryOperator(t *testing.T) {
	for kind, op := range binaryOps {
		if Precedence(op) == 0 {
			t.Errorf("%s (token %s) has no precedence", op, kind)
		}
	}
}

func TestOperatorNames(t *testing.T) {
	if BinaryOp(99).String() != "unknown operator" {
		t.Error("an unknown binary operator must say so")
	}
	if OpNeg.String() != "-" || OpNot.String() != "!" {
		t.Error("unary operator names are wrong")
	}
}

func TestKeywordsAreNotPaths(t *testing.T) {
	if _, ok := exprOf(t, "true").(*BoolLit); !ok {
		t.Error("true must parse as a literal, not a path")
	}
	if _, ok := exprOf(t, "trueish").(*Path); !ok {
		t.Error("an identifier starting with true is still a path")
	}
}

func TestEverySpanAccessor(t *testing.T) {
	sources := []string{"A", "'x'", "1", "1.5", "true", "!A", "A + B", "A[0]"}
	for _, source := range sources {
		if got := exprOf(t, source).ExprSpan(); got.Len() == 0 {
			t.Errorf("%s reports an empty span", source)
		}
	}
}

func TestEveryNodeSpan(t *testing.T) {
	doc := parseClean(t, "text{{ A }}{% let x = 1 %}{% if A %}y{% endif %}{% for a in A %}z{% endfor %}")
	if len(doc.Nodes) != 5 {
		t.Fatalf("nodes = %d", len(doc.Nodes))
	}
	for _, node := range doc.Nodes {
		if node.NodeSpan().Len() == 0 {
			t.Errorf("%T reports an empty span", node)
		}
	}
	layout := parseClean(t, "{% outlet %}")
	if layout.Nodes[0].NodeSpan().Len() == 0 {
		t.Error("outlet reports an empty span")
	}
}

func TestFilterShapes(t *testing.T) {
	cases := map[string]string{
		"A | upper":                "(A | upper)",
		"A | truncate(3)":          "(A | truncate(3))",
		"A | default('x') | upper": `((A | default("x")) | upper)`,
		"A + B | upper":            "((A + B) | upper)",
		"A | money('PLN' | upper)": `(A | money(("PLN" | upper)))`,
	}
	for source, want := range cases {
		doc := parseClean(t, "{{ "+source+" }}")
		node, ok := doc.Nodes[0].(*Interpolation)
		if !ok {
			t.Fatalf("%q parsed to %#v", source, doc.Nodes[0])
		}
		if got := render(node.Expr); got != want {
			t.Errorf("%q = %s, want %s", source, got, want)
		}
	}
}

func TestFilterSpansCoverTheWholeCall(t *testing.T) {
	doc := parseClean(t, "{{ A | truncate(3) }}")
	call := doc.Nodes[0].(*Interpolation).Expr.(*FilterCall)
	if call.Span.Start >= call.Span.End || call.NameSpan.Start <= call.Span.Start {
		t.Errorf("span = %+v, name span = %+v", call.Span, call.NameSpan)
	}
}

func TestMalformedFiltersAreReported(t *testing.T) {
	for _, source := range []string{"{{ A | }}", "{{ A | 3 }}", "{{ A | truncate(3 }}", "{{ A | truncate( }}"} {
		_, bag := parse(t, source)
		if !bag.HasErrors() {
			t.Errorf("%q was accepted", source)
		}
	}
}

func TestMessageAndBuiltinShapes(t *testing.T) {
	cases := map[string]string{
		"t('a.b')":                       `t("a.b")`,
		"t(\"reviews\", count = len(R))": `t("reviews", (R | len))`,
		"len(Items) > 0":                 "((Items | len) > 0)",
		"t('a') | upper":                 `(t("a") | upper)`,
		"tone":                           "tone",
		"t.name":                         "t.name",
	}
	for source, want := range cases {
		doc := parseClean(t, "{{ "+source+" }}")
		got := render(doc.Nodes[0].(*Interpolation).Expr)
		if got != want {
			t.Errorf("%q = %s, want %s", source, got, want)
		}
	}
}

func TestMessageSpansCoverTheCall(t *testing.T) {
	doc := parseClean(t, `{{ t("a.b") }}`)
	call := doc.Nodes[0].(*Interpolation).Expr.(*MessageCall)
	if call.Span.Start >= call.Span.End || call.KeySpan.Start <= call.Span.Start {
		t.Errorf("span = %+v, key span = %+v", call.Span, call.KeySpan)
	}
}

func TestMalformedMessageCallsAreReported(t *testing.T) {
	sources := []string{
		"{{ t() }}",
		"{{ t(key) }}",
		`{{ t("a", 3) }}`,
		`{{ t("a", other = 3) }}`,
		`{{ t("a", count 3) }}`,
		`{{ t("a", count = ) }}`,
		`{{ t("a" }}`,
		`{{ t("a", count = 1 }}`,
	}
	for _, source := range sources {
		_, bag := parse(t, source)
		if !bag.HasErrors() {
			t.Errorf("%q was accepted", source)
		}
	}
}

func TestMalformedBuiltinCallsAreReported(t *testing.T) {
	for _, source := range []string{"{{ len( }}", "{{ len(Items }}", "{{ len(@) }}"} {
		_, bag := parse(t, source)
		if !bag.HasErrors() {
			t.Errorf("%q was accepted", source)
		}
	}
}
