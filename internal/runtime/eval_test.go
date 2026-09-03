package runtime

import (
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/ir"
)

type arena struct {
	plan ir.Plan
}

func (a *arena) constant(value ir.Const) uint32 {
	a.plan.Consts = append(a.plan.Consts, value)
	return a.node(ir.ExprNode{Kind: ir.ExprConst, A: uint32(len(a.plan.Consts) - 1)})
}

func (a *arena) str(s string) uint32 {
	return a.constant(ir.Const{Kind: ir.ConstString, Str: s})
}

func (a *arena) integer(n int64) uint32 {
	return a.constant(ir.Const{Kind: ir.ConstInt, Int: n})
}

func (a *arena) float(f float64) uint32 {
	return a.constant(ir.Const{Kind: ir.ConstFloat, Float: f})
}

func (a *arena) boolean(b bool) uint32 {
	var n int64
	if b {
		n = 1
	}
	return a.constant(ir.Const{Kind: ir.ConstBool, Int: n})
}

func (a *arena) path(segments ...string) uint32 {
	a.plan.Paths = append(a.plan.Paths, segments)
	return a.node(ir.ExprNode{Kind: ir.ExprPath, A: uint32(len(a.plan.Paths) - 1)})
}

func (a *arena) localPath(slot uint32, segments ...string) uint32 {
	rest := ir.NoPath
	if len(segments) > 0 {
		a.plan.Paths = append(a.plan.Paths, segments)
		rest = uint32(len(a.plan.Paths) - 1)
	}
	return a.node(ir.ExprNode{Kind: ir.ExprLocal, A: slot, B: rest})
}

func (a *arena) binary(op ir.BinaryOp, left, right uint32) uint32 {
	return a.node(ir.ExprNode{Kind: ir.ExprBinary, Op: uint8(op), A: left, B: right})
}

func (a *arena) unary(op ir.UnaryOp, operand uint32) uint32 {
	return a.node(ir.ExprNode{Kind: ir.ExprUnary, Op: uint8(op), A: operand})
}

func (a *arena) index(base, position uint32) uint32 {
	return a.node(ir.ExprNode{Kind: ir.ExprIndex, A: base, B: position})
}

func (a *arena) node(node ir.ExprNode) uint32 {
	a.plan.Exprs = append(a.plan.Exprs, node)
	return uint32(len(a.plan.Exprs) - 1)
}

func (a *arena) scope(props Accessible, locals ...Value) *scope {
	return &scope{plan: &a.plan, props: props, locals: locals}
}

func evalOK(t *testing.T, s *scope, index uint32) Value {
	t.Helper()
	value, err := s.eval(index)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return value
}

func evalErr(t *testing.T, s *scope, index uint32, want string) {
	t.Helper()
	_, err := s.eval(index)
	if err == nil {
		t.Fatalf("expected an error mentioning %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to mention %q", err, want)
	}
}

func TestConstantsOfEveryKind(t *testing.T) {
	var a arena
	s := a.scope(nil)
	cases := map[uint32]string{
		a.str("hi"):      "hi",
		a.integer(-7):    "-7",
		a.float(2.5):     "2.5",
		a.boolean(true):  "true",
		a.boolean(false): "false",
	}
	for index, want := range cases {
		if got := evalOK(t, s, index).Text(); got != want {
			t.Errorf("constant rendered %q, want %q", got, want)
		}
	}
}

func TestPathReadsProps(t *testing.T) {
	var a arena
	index := a.path("Title")
	s := a.scope(Map{"Title": String("hello")})
	if got := evalOK(t, s, index).Str; got != "hello" {
		t.Errorf("path = %q", got)
	}
}

func TestPathErrors(t *testing.T) {
	var a arena
	index := a.path("Missing")
	evalErr(t, a.scope(Map{}), index, "no field Missing")
	evalErr(t, a.scope(nil), index, "no props")

	var b arena
	dangling := b.node(ir.ExprNode{Kind: ir.ExprPath, A: 9})
	evalErr(t, b.scope(Map{}), dangling, "not in the plan")
}

func TestLocalReadsTheSlot(t *testing.T) {
	var a arena
	index := a.localPath(0)
	if got := evalOK(t, a.scope(nil, Int(42)), index).Int(); got != 42 {
		t.Errorf("local = %d", got)
	}
}

func TestLocalFieldAccess(t *testing.T) {
	var a arena
	index := a.localPath(0, "Title")
	item := Object(Map{"Title": String("card")})
	if got := evalOK(t, a.scope(nil, item), index).Str; got != "card" {
		t.Errorf("local field = %q", got)
	}
}

func TestLocalErrors(t *testing.T) {
	var a arena
	evalErr(t, a.scope(nil), a.localPath(3), "out of range")

	var b arena
	onScalar := b.localPath(0, "Title")
	evalErr(t, b.scope(nil, Int(1)), onScalar, "no fields")

	var c arena
	missing := c.localPath(0, "Nope")
	evalErr(t, c.scope(nil, Object(Map{})), missing, "no field Nope")

	var d arena
	dangling := d.node(ir.ExprNode{Kind: ir.ExprLocal, A: 0, B: 9})
	evalErr(t, d.scope(nil, Object(Map{})), dangling, "not in the plan")
}

func TestUnaryOperators(t *testing.T) {
	var a arena
	s := a.scope(nil)
	if evalOK(t, s, a.unary(ir.UnaryNot, a.boolean(true))).Truthy() {
		t.Error("!true must be false")
	}
	if got := evalOK(t, s, a.unary(ir.UnaryNeg, a.integer(5))).Int(); got != -5 {
		t.Errorf("-5 = %d", got)
	}
	if got := evalOK(t, s, a.unary(ir.UnaryNeg, a.float(1.5))).Float(); got != -1.5 {
		t.Errorf("-1.5 = %v", got)
	}
	evalErr(t, s, a.unary(ir.UnaryNeg, a.str("x")), "cannot negate")
}

func TestIntegerArithmetic(t *testing.T) {
	var a arena
	s := a.scope(nil)
	cases := map[ir.BinaryOp]int64{
		ir.BinaryAdd: 7,
		ir.BinarySub: 1,
		ir.BinaryMul: 12,
		ir.BinaryDiv: 1,
		ir.BinaryMod: 1,
	}
	for op, want := range cases {
		index := a.binary(op, a.integer(4), a.integer(3))
		if got := evalOK(t, s, index).Int(); got != want {
			t.Errorf("4 %s 3 = %d, want %d", op, got, want)
		}
	}
}

func TestFloatArithmetic(t *testing.T) {
	var a arena
	s := a.scope(nil)
	cases := map[ir.BinaryOp]float64{
		ir.BinaryAdd: 5.5,
		ir.BinarySub: 2.5,
		ir.BinaryMul: 6,
		ir.BinaryDiv: 2.6666666666666665,
		ir.BinaryMod: 1,
	}
	for op, want := range cases {
		index := a.binary(op, a.float(4), a.float(1.5))
		if got := evalOK(t, s, index).Float(); got != want {
			t.Errorf("4 %s 1.5 = %v, want %v", op, got, want)
		}
	}
}

func TestDivisionAndModuloByZero(t *testing.T) {
	var a arena
	s := a.scope(nil)
	for _, op := range []ir.BinaryOp{ir.BinaryDiv, ir.BinaryMod} {
		evalErr(t, s, a.binary(op, a.integer(1), a.integer(0)), "division by zero")
		evalErr(t, s, a.binary(op, a.float(1), a.float(0)), "division by zero")
	}
}

func TestArithmeticOnNonNumbers(t *testing.T) {
	var a arena
	evalErr(t, a.scope(nil), a.binary(ir.BinaryAdd, a.str("a"), a.integer(1)), "cannot apply")
}

func TestShortCircuitLogic(t *testing.T) {
	var a arena
	s := a.scope(nil)
	missing := a.path("Missing")

	if evalOK(t, s, a.binary(ir.BinaryAnd, a.boolean(false), missing)).Truthy() {
		t.Error("&& must stop at a false left side")
	}
	if !evalOK(t, s, a.binary(ir.BinaryOr, a.boolean(true), missing)).Truthy() {
		t.Error("|| must stop at a true left side")
	}
	if !evalOK(t, s, a.binary(ir.BinaryAnd, a.boolean(true), a.boolean(true))).Truthy() {
		t.Error("true && true must be true")
	}
	if evalOK(t, s, a.binary(ir.BinaryOr, a.boolean(false), a.boolean(false))).Truthy() {
		t.Error("false || false must be false")
	}
}

func TestLogicPropagatesErrorsFromTheRightSide(t *testing.T) {
	var a arena
	s := a.scope(Map{})
	missing := a.path("Missing")
	evalErr(t, s, a.binary(ir.BinaryAnd, a.boolean(true), missing), "no field")
	evalErr(t, s, a.binary(ir.BinaryOr, a.boolean(false), missing), "no field")
}

func TestEqualityAcrossKinds(t *testing.T) {
	var a arena
	s := a.scope(nil)
	cases := []struct {
		index uint32
		want  bool
	}{
		{a.binary(ir.BinaryEq, a.integer(1), a.float(1)), true},
		{a.binary(ir.BinaryEq, a.str("a"), a.str("a")), true},
		{a.binary(ir.BinaryEq, a.str("a"), a.integer(1)), false},
		{a.binary(ir.BinaryEq, a.boolean(true), a.boolean(true)), true},
		{a.binary(ir.BinaryNe, a.integer(1), a.integer(2)), true},
	}
	for _, c := range cases {
		if got := evalOK(t, s, c.index).Truthy(); got != c.want {
			t.Errorf("equality expression %d = %v, want %v", c.index, got, c.want)
		}
	}
}

func TestNilEqualsNil(t *testing.T) {
	if !Equal(Nil(), Nil()) {
		t.Error("two missing values compare equal")
	}
	if Equal(Seq(Values{}), Seq(Values{})) {
		t.Error("sequences do not compare equal")
	}
}

func TestOrderingNumbersAndStrings(t *testing.T) {
	var a arena
	s := a.scope(nil)
	cases := []struct {
		index uint32
		want  bool
	}{
		{a.binary(ir.BinaryLt, a.integer(1), a.integer(2)), true},
		{a.binary(ir.BinaryLe, a.integer(2), a.integer(2)), true},
		{a.binary(ir.BinaryGt, a.integer(3), a.integer(2)), true},
		{a.binary(ir.BinaryGe, a.integer(2), a.integer(3)), false},
		{a.binary(ir.BinaryLt, a.str("a"), a.str("b")), true},
		{a.binary(ir.BinaryGe, a.str("b"), a.str("b")), true},
	}
	for _, c := range cases {
		if got := evalOK(t, s, c.index).Truthy(); got != c.want {
			t.Errorf("ordering expression %d = %v, want %v", c.index, got, c.want)
		}
	}
}

func TestOrderingRejectsMixedKinds(t *testing.T) {
	var a arena
	evalErr(t, a.scope(nil), a.binary(ir.BinaryLt, a.str("a"), a.integer(1)), "cannot compare")
}

func TestConcatUsesTextForm(t *testing.T) {
	var a arena
	index := a.binary(ir.BinaryConcat, a.str("n="), a.integer(3))
	if got := evalOK(t, a.scope(nil), index).Str; got != "n=3" {
		t.Errorf("concat = %q", got)
	}
}

func TestIndexReadsSequences(t *testing.T) {
	var a arena
	items := Seq(Values{Int(10), Int(20)})
	s := a.scope(Map{"Items": items})
	if got := evalOK(t, s, a.index(a.path("Items"), a.integer(1))).Int(); got != 20 {
		t.Errorf("index = %d", got)
	}
}

func TestIndexErrors(t *testing.T) {
	var a arena
	s := a.scope(Map{"Items": Seq(Values{Int(1)}), "Title": String("x")})
	evalErr(t, s, a.index(a.path("Title"), a.integer(0)), "cannot index")
	evalErr(t, s, a.index(a.path("Items"), a.str("x")), "whole number")
	evalErr(t, s, a.index(a.path("Items"), a.integer(5)), "outside the sequence")
	evalErr(t, s, a.index(a.path("Items"), a.integer(-1)), "outside the sequence")
}

func TestUnknownExpressionKind(t *testing.T) {
	var a arena
	index := a.node(ir.ExprNode{Kind: ir.ExprKind(99)})
	evalErr(t, a.scope(nil), index, "does not know")
}

func TestDanglingConstant(t *testing.T) {
	var a arena
	index := a.node(ir.ExprNode{Kind: ir.ExprConst, A: 9})
	evalErr(t, a.scope(nil), index, "not in the plan")
}

func TestValuesSequenceBounds(t *testing.T) {
	items := Values{Int(1)}
	if items.Len() != 1 {
		t.Errorf("Len = %d", items.Len())
	}
	if items.At(0).Int() != 1 {
		t.Error("At(0) must return the element")
	}
	for _, index := range []int{-1, 5} {
		if items.At(index).Kind != KindNil {
			t.Errorf("At(%d) must be the missing value", index)
		}
	}
}

func TestMapWalksNestedObjects(t *testing.T) {
	nested := Map{"Inner": Object(Map{"Leaf": String("v")})}
	if value, ok := nested.Get([]string{"Inner", "Leaf"}); !ok || value.Str != "v" {
		t.Errorf("nested lookup = %+v, %v", value, ok)
	}
	if _, ok := nested.Get(nil); ok {
		t.Error("an empty path resolves to nothing")
	}
	scalar := Map{"Leaf": Int(1)}
	if _, ok := scalar.Get([]string{"Leaf", "Deeper"}); ok {
		t.Error("a scalar has no fields")
	}
}

func TestTruthiness(t *testing.T) {
	cases := map[string]struct {
		value Value
		want  bool
	}{
		"empty string":   {String(""), false},
		"string":         {String("x"), true},
		"zero":           {Int(0), false},
		"number":         {Int(1), true},
		"zero float":     {Float(0), false},
		"float":          {Float(0.5), true},
		"false":          {Bool(false), false},
		"true":           {Bool(true), true},
		"empty sequence": {Seq(Values{}), false},
		"sequence":       {Seq(Values{Int(1)}), true},
		"nil sequence":   {Value{Kind: KindSeq}, false},
		"object":         {Object(Map{}), true},
		"nil object":     {Value{Kind: KindObject}, false},
		"missing":        {Nil(), false},
	}
	for name, c := range cases {
		if got := c.value.Truthy(); got != c.want {
			t.Errorf("%s: Truthy = %v, want %v", name, got, c.want)
		}
	}
}

func TestNumberConversion(t *testing.T) {
	if _, ok := String("1").Number(); ok {
		t.Error("a string is not a number")
	}
	if value, ok := Float(1.5).Number(); !ok || value != 1.5 {
		t.Errorf("Number = %v, %v", value, ok)
	}
	if !Int(1).IsInt() || Float(1).IsInt() {
		t.Error("IsInt is wrong")
	}
}

func TestKindNames(t *testing.T) {
	if kindName(KindSeq) != "sequence" || kindName(ValueKind(99)) != "value" {
		t.Error("kind names are wrong")
	}
}

func TestUnknownOperatorsAreRejected(t *testing.T) {
	var a arena
	s := a.scope(nil)
	evalErr(t, s, a.node(ir.ExprNode{Kind: ir.ExprBinary, Op: 99, A: a.integer(1), B: a.integer(2)}), "unknown operator")
	evalErr(t, s, a.node(ir.ExprNode{Kind: ir.ExprBinary, Op: 99, A: a.float(1), B: a.float(2)}), "unknown operator")
}

func TestErrorsPropagateThroughEveryOperand(t *testing.T) {
	var a arena
	s := a.scope(Map{})
	missing := a.path("Missing")
	evalErr(t, s, a.unary(ir.UnaryNot, missing), "no field")
	evalErr(t, s, a.binary(ir.BinaryAdd, missing, a.integer(1)), "no field")
	evalErr(t, s, a.binary(ir.BinaryAdd, a.integer(1), missing), "no field")
	evalErr(t, s, a.index(missing, a.integer(0)), "no field")
	evalErr(t, s, a.index(a.path("Items"), missing), "no field")
}

func TestIndexOnANilSequenceIsRejected(t *testing.T) {
	var a arena
	s := a.scope(Map{"Items": Value{Kind: KindSeq}})
	evalErr(t, s, a.index(a.path("Items"), a.integer(0)), "cannot index")
}

func TestTextTakesTheFastPathForPlainFields(t *testing.T) {
	var a arena
	s := a.scope(Map{"Title": String("plain")})
	got, err := s.text(a.path("Title"))
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	if got != "plain" {
		t.Errorf("text = %q", got)
	}
}

func TestTextFallsBackToTheEvaluator(t *testing.T) {
	var a arena
	s := a.scope(nil)
	got, err := s.text(a.binary(ir.BinaryAdd, a.integer(2), a.integer(3)))
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	if got != "5" {
		t.Errorf("text = %q", got)
	}
}

func TestTextReportsErrorsFromBothPaths(t *testing.T) {
	var a arena
	s := a.scope(Map{})
	if _, err := s.text(a.path("Missing")); err == nil {
		t.Error("the fast path must report a missing field")
	}
	if _, err := s.text(a.binary(ir.BinaryAdd, a.path("Missing"), a.integer(1))); err == nil {
		t.Error("the evaluator path must report a missing field")
	}
	if _, err := s.text(99); err == nil {
		t.Error("a dangling expression must be reported")
	}
}

func TestEvalRejectsADanglingIndex(t *testing.T) {
	var a arena
	evalErr(t, a.scope(nil), 99, "not in the plan")
}

func TestIndexPositionErrorSurfaces(t *testing.T) {
	var a arena
	s := a.scope(Map{"Items": Seq(Values{Int(1)})})
	evalErr(t, s, a.index(a.path("Items"), a.path("Missing")), "no field Missing")
}
