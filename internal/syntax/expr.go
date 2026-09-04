package syntax

import "github.com/apptivitypl/gopage/internal/diag"

type BinaryOp uint8

const (
	OpOr BinaryOp = iota
	OpAnd
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe
	OpConcat
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
)

var binaryNames = map[BinaryOp]string{
	OpOr: "||", OpAnd: "&&",
	OpEq: "==", OpNe: "!=", OpLt: "<", OpLe: "<=", OpGt: ">", OpGe: ">=",
	OpConcat: "~", OpAdd: "+", OpSub: "-", OpMul: "*", OpDiv: "/", OpMod: "%",
}

func (o BinaryOp) String() string {
	if name, ok := binaryNames[o]; ok {
		return name
	}
	return "unknown operator"
}

type UnaryOp uint8

const (
	OpNot UnaryOp = iota
	OpNeg
)

func (o UnaryOp) String() string {
	if o == OpNeg {
		return "-"
	}
	return "!"
}

type Expr interface {
	ExprSpan() diag.Span
}

type Path struct {
	Span     diag.Span
	Segments []string
}

func (p *Path) ExprSpan() diag.Span { return p.Span }

type StringLit struct {
	Span  diag.Span
	Value string
}

func (s *StringLit) ExprSpan() diag.Span { return s.Span }

type IntLit struct {
	Span  diag.Span
	Value int64
}

func (i *IntLit) ExprSpan() diag.Span { return i.Span }

type FloatLit struct {
	Span  diag.Span
	Value float64
}

func (f *FloatLit) ExprSpan() diag.Span { return f.Span }

type BoolLit struct {
	Span  diag.Span
	Value bool
}

func (b *BoolLit) ExprSpan() diag.Span { return b.Span }

type Unary struct {
	Span    diag.Span
	Op      UnaryOp
	Operand Expr
}

func (u *Unary) ExprSpan() diag.Span { return u.Span }

type Binary struct {
	Span  diag.Span
	Op    BinaryOp
	Left  Expr
	Right Expr
}

func (b *Binary) ExprSpan() diag.Span { return b.Span }

type Index struct {
	Span  diag.Span
	Base  Expr
	Index Expr
}

func (i *Index) ExprSpan() diag.Span { return i.Span }

var binaryOps = map[Kind]BinaryOp{
	KindOr:      OpOr,
	KindAnd:     OpAnd,
	KindEq:      OpEq,
	KindNe:      OpNe,
	KindLt:      OpLt,
	KindLe:      OpLe,
	KindGt:      OpGt,
	KindGe:      OpGe,
	KindTilde:   OpConcat,
	KindPlus:    OpAdd,
	KindMinus:   OpSub,
	KindStar:    OpMul,
	KindSlash:   OpDiv,
	KindPercent: OpMod,
}

var precedence = map[BinaryOp]int{
	OpOr:  1,
	OpAnd: 2,
	OpEq:  3, OpNe: 3, OpLt: 3, OpLe: 3, OpGt: 3, OpGe: 3,
	OpConcat: 4,
	OpAdd:    5, OpSub: 5,
	OpMul: 6, OpDiv: 6, OpMod: 6,
}

func Precedence(op BinaryOp) int {
	return precedence[op]
}
