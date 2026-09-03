package runtime

import (
	"fmt"

	"github.com/apptivitypl/rill/internal/i18n"
	"math"
	"strings"

	"github.com/apptivitypl/rill/internal/ir"
)

type scope struct {
	plan    *ir.Plan
	props   Accessible
	locals  []Value
	catalog *ir.Catalog
	plural  i18n.Rule
}

func (s *scope) eval(index uint32) (Value, error) {
	node, ok := s.plan.Expr(index)
	if !ok {
		return Nil(), fmt.Errorf("expression %d is not in the plan", index)
	}
	return s.evalNode(node)
}

func (s *scope) evalNode(node ir.ExprNode) (Value, error) {
	switch node.Kind {
	case ir.ExprConst:
		return s.constant(node.A)
	case ir.ExprPath:
		return s.path(node.A)
	case ir.ExprLocal:
		return s.local(node.A, node.B)
	case ir.ExprUnary:
		return s.unary(node)
	case ir.ExprBinary:
		return s.binary(node)
	case ir.ExprIndex:
		return s.index(node)
	case ir.ExprFilter:
		return s.filter(node)
	case ir.ExprMessage:
		return s.message(node)
	default:
		return Nil(), fmt.Errorf("plan uses %s, which this runtime does not know", node.Kind)
	}
}

func (s *scope) filter(node ir.ExprNode) (Value, error) {
	value, err := s.eval(node.A)
	if err != nil {
		return Nil(), err
	}
	argument := Nil()
	if node.B != NoArgument {
		if argument, err = s.eval(node.B); err != nil {
			return Nil(), err
		}
	}
	return ApplyFilter(uint32(node.Op), value, argument)
}

func (s *scope) text(index uint32) (string, error) {
	node, ok := s.plan.Expr(index)
	if !ok {
		return "", fmt.Errorf("expression %d is not in the plan", index)
	}
	if node.Kind == ir.ExprPath {
		value, err := s.path(node.A)
		if err != nil {
			return "", err
		}
		return value.Text(), nil
	}
	value, err := s.evalNode(node)
	if err != nil {
		return "", err
	}
	return value.Text(), nil
}

func (s *scope) constant(index uint32) (Value, error) {
	value, ok := s.plan.Const(index)
	if !ok {
		return Nil(), fmt.Errorf("constant %d is not in the plan", index)
	}
	switch value.Kind {
	case ir.ConstString:
		return String(value.Str), nil
	case ir.ConstInt:
		return Int(value.Int), nil
	case ir.ConstFloat:
		return Float(value.Float), nil
	default:
		return Bool(value.Int != 0), nil
	}
}

func (s *scope) path(index uint32) (Value, error) {
	path := s.plan.Path(index)
	if path == nil {
		return Nil(), fmt.Errorf("path %d is not in the plan", index)
	}
	if s.props == nil {
		return Nil(), fmt.Errorf("template reads %s but the route provides no props", strings.Join(path, "."))
	}
	value, ok := s.props.Get(path)
	if !ok {
		return Nil(), fmt.Errorf("props have no field %s", strings.Join(path, "."))
	}
	return value, nil
}

func (s *scope) local(slot, pathIndex uint32) (Value, error) {
	if slot >= uint32(len(s.locals)) {
		return Nil(), fmt.Errorf("local %d is out of range", slot)
	}
	value := s.locals[slot]
	if pathIndex == ir.NoPath {
		return value, nil
	}
	path := s.plan.Path(pathIndex)
	if path == nil {
		return Nil(), fmt.Errorf("path %d is not in the plan", pathIndex)
	}
	object := value.Object()
	if value.Kind != KindObject || object == nil {
		return Nil(), fmt.Errorf("cannot read %s: the value has no fields", strings.Join(path, "."))
	}
	field, ok := object.Get(path)
	if !ok {
		return Nil(), fmt.Errorf("no field %s on the loop value", strings.Join(path, "."))
	}
	return field, nil
}

func (s *scope) unary(node ir.ExprNode) (Value, error) {
	operand, err := s.eval(node.A)
	if err != nil {
		return Nil(), err
	}
	switch ir.UnaryOp(node.Op) {
	case ir.UnaryNot:
		return Bool(!operand.Truthy()), nil
	default:
		if operand.IsInt() {
			return Int(-operand.Int()), nil
		}
		number, ok := operand.Number()
		if !ok {
			return Nil(), fmt.Errorf("cannot negate a %s", kindName(operand.Kind))
		}
		return Float(-number), nil
	}
}

func (s *scope) binary(node ir.ExprNode) (Value, error) {
	op := ir.BinaryOp(node.Op)
	left, err := s.eval(node.A)
	if err != nil {
		return Nil(), err
	}
	switch op {
	case ir.BinaryOr:
		if left.Truthy() {
			return Bool(true), nil
		}
		right, err := s.eval(node.B)
		if err != nil {
			return Nil(), err
		}
		return Bool(right.Truthy()), nil
	case ir.BinaryAnd:
		if !left.Truthy() {
			return Bool(false), nil
		}
		right, err := s.eval(node.B)
		if err != nil {
			return Nil(), err
		}
		return Bool(right.Truthy()), nil
	}

	right, err := s.eval(node.B)
	if err != nil {
		return Nil(), err
	}
	switch op {
	case ir.BinaryEq:
		return Bool(Equal(left, right)), nil
	case ir.BinaryNe:
		return Bool(!Equal(left, right)), nil
	case ir.BinaryConcat:
		return String(left.Text() + right.Text()), nil
	case ir.BinaryLt, ir.BinaryLe, ir.BinaryGt, ir.BinaryGe:
		return compare(op, left, right)
	default:
		return arithmetic(op, left, right)
	}
}

func compare(op ir.BinaryOp, left, right Value) (Value, error) {
	if left.Kind == KindString && right.Kind == KindString {
		return Bool(orderResult(op, strings.Compare(left.Str, right.Str))), nil
	}
	a, aok := left.Number()
	b, bok := right.Number()
	if !aok || !bok {
		return Nil(), fmt.Errorf("cannot compare a %s with a %s", kindName(left.Kind), kindName(right.Kind))
	}
	switch {
	case a < b:
		return Bool(orderResult(op, -1)), nil
	case a > b:
		return Bool(orderResult(op, 1)), nil
	default:
		return Bool(orderResult(op, 0)), nil
	}
}

func orderResult(op ir.BinaryOp, sign int) bool {
	switch op {
	case ir.BinaryLt:
		return sign < 0
	case ir.BinaryLe:
		return sign <= 0
	case ir.BinaryGt:
		return sign > 0
	default:
		return sign >= 0
	}
}

func arithmetic(op ir.BinaryOp, left, right Value) (Value, error) {
	if left.IsInt() && right.IsInt() {
		return integerArithmetic(op, left.Int(), right.Int())
	}
	a, aok := left.Number()
	b, bok := right.Number()
	if !aok || !bok {
		return Nil(), fmt.Errorf("cannot apply %s to a %s and a %s",
			op, kindName(left.Kind), kindName(right.Kind))
	}
	switch op {
	case ir.BinaryAdd:
		return Float(a + b), nil
	case ir.BinarySub:
		return Float(a - b), nil
	case ir.BinaryMul:
		return Float(a * b), nil
	case ir.BinaryDiv:
		if b == 0 {
			return Nil(), fmt.Errorf("division by zero")
		}
		return Float(a / b), nil
	case ir.BinaryMod:
		if b == 0 {
			return Nil(), fmt.Errorf("division by zero")
		}
		return Float(math.Mod(a, b)), nil
	default:
		return Nil(), fmt.Errorf("plan uses an unknown operator %d", op)
	}
}

func integerArithmetic(op ir.BinaryOp, a, b int64) (Value, error) {
	switch op {
	case ir.BinaryAdd:
		return Int(a + b), nil
	case ir.BinarySub:
		return Int(a - b), nil
	case ir.BinaryMul:
		return Int(a * b), nil
	case ir.BinaryDiv:
		if b == 0 {
			return Nil(), fmt.Errorf("division by zero")
		}
		return Int(a / b), nil
	case ir.BinaryMod:
		if b == 0 {
			return Nil(), fmt.Errorf("division by zero")
		}
		return Int(a % b), nil
	default:
		return Nil(), fmt.Errorf("plan uses an unknown operator %d", op)
	}
}

func (s *scope) index(node ir.ExprNode) (Value, error) {
	base, err := s.eval(node.A)
	if err != nil {
		return Nil(), err
	}
	position, err := s.eval(node.B)
	if err != nil {
		return Nil(), err
	}
	seq := base.Sequence()
	if base.Kind != KindSeq || seq == nil {
		return Nil(), fmt.Errorf("cannot index a %s", kindName(base.Kind))
	}
	if !position.IsInt() {
		return Nil(), fmt.Errorf("an index must be a whole number, got a %s", kindName(position.Kind))
	}
	at := position.Int()
	if at < 0 || at >= int64(seq.Len()) {
		return Nil(), fmt.Errorf("index %d is outside the sequence of %d", at, seq.Len())
	}
	return seq.At(int(at)), nil
}

var kindNames = map[ValueKind]string{
	KindNil:    "missing value",
	KindString: "string",
	KindInt:    "whole number",
	KindFloat:  "number",
	KindBool:   "boolean",
	KindSeq:    "sequence",
	KindObject: "struct",
}

func kindName(kind ValueKind) string {
	if name, ok := kindNames[kind]; ok {
		return name
	}
	return "value"
}
