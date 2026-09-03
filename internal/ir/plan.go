package ir

import "strings"

type OpKind uint8

const (
	OpStatic OpKind = iota
	OpText
	OpOutlet
	OpJumpIfFalse
	OpJump
	OpLet
	OpIterStart
	OpIterNext
	OpFragment
	OpJSON
	OpPreload
	OpURL
)

var opNames = map[OpKind]string{
	OpStatic:      "static",
	OpText:        "text",
	OpOutlet:      "outlet",
	OpFragment:    "fragment",
	OpJSON:        "json",
	OpURL:         "url",
	OpPreload:     "preload",
	OpJumpIfFalse: "jump-if-false",
	OpJump:        "jump",
	OpLet:         "let",
	OpIterStart:   "iter-start",
	OpIterNext:    "iter-next",
}

func (k OpKind) String() string {
	if name, ok := opNames[k]; ok {
		return name
	}
	return "unknown op"
}

type Op struct {
	Kind OpKind
	A    uint32
	B    uint32
	C    uint32
}

type ExprKind uint8

const (
	ExprConst ExprKind = iota
	ExprPath
	ExprLocal
	ExprBinary
	ExprUnary
	ExprIndex
	ExprFilter
	ExprMessage
)

var exprNames = map[ExprKind]string{
	ExprConst:   "constant",
	ExprPath:    "props path",
	ExprLocal:   "local",
	ExprBinary:  "binary",
	ExprUnary:   "unary",
	ExprIndex:   "index",
	ExprFilter:  "filter",
	ExprMessage: "message",
}

func (k ExprKind) String() string {
	if name, ok := exprNames[k]; ok {
		return name
	}
	return "unknown expression"
}

type ExprNode struct {
	Kind ExprKind
	Op   uint8
	A    uint32
	B    uint32
}

type ConstKind uint8

const (
	ConstString ConstKind = iota
	ConstInt
	ConstFloat
	ConstBool
)

type Const struct {
	Kind  ConstKind
	Str   string
	Int   int64
	Float float64
}

const NoPath = ^uint32(0)

type IslandUse struct {
	Name     string
	Strategy string
}

type Plan struct {
	Messages  []string
	Ops       []Op
	Fragments []Fragment
	Islands   []IslandUse
	Exprs     []ExprNode
	Consts    []Const
	Blob      []byte
	Paths     [][]string
	Locals    uint32
	Capacity  uint32
}

func (p *Plan) Message(index uint32) string {
	if index >= uint32(len(p.Messages)) {
		return ""
	}
	return p.Messages[index]
}

func (p *Plan) Fragment(index uint32) (Fragment, bool) {
	if index >= uint32(len(p.Fragments)) {
		return Fragment{}, false
	}
	return p.Fragments[index], true
}

func (p *Plan) Static(op Op) []byte {
	if op.A > uint32(len(p.Blob)) || op.A+op.B > uint32(len(p.Blob)) {
		return nil
	}
	return p.Blob[op.A : op.A+op.B]
}

func (p *Plan) Path(index uint32) []string {
	if index >= uint32(len(p.Paths)) {
		return nil
	}
	return p.Paths[index]
}

func (p *Plan) Expr(index uint32) (ExprNode, bool) {
	if index >= uint32(len(p.Exprs)) {
		return ExprNode{}, false
	}
	return p.Exprs[index], true
}

func (p *Plan) Const(index uint32) (Const, bool) {
	if index >= uint32(len(p.Consts)) {
		return Const{}, false
	}
	return p.Consts[index], true
}

type RouteClass uint8

const (
	ClassStatic RouteClass = iota
	ClassDynamic
)

var classNames = map[RouteClass]string{
	ClassStatic:  "static",
	ClassDynamic: "dynamic",
}

func (c RouteClass) String() string {
	if name, ok := classNames[c]; ok {
		return name
	}
	return "unknown class"
}

type Route struct {
	Pattern     string
	Name        string
	Plan        uint32
	LayoutChain []uint32
	Class       RouteClass
}

type Fragment struct {
	Name     string
	TTL      int64
	Stale    int64
	Deferred bool
	Strategy string
	Paths    []uint32
	BodyEnd  uint32
	Hold     uint32
	HoldEnd  uint32
}

func (f Fragment) Held() bool {
	return f.HoldEnd > f.Hold
}

func (f Fragment) Cacheable() bool {
	return f.TTL > 0
}

type FallbackKind uint8

const (
	FallbackNotFound FallbackKind = iota
	FallbackError
)

func (k FallbackKind) String() string {
	if k == FallbackError {
		return "error"
	}
	return "not-found"
}

type Fallback struct {
	Prefix      string
	Name        string
	Kind        FallbackKind
	Plan        uint32
	LayoutChain []uint32
}

const PluralForms = 6

type Catalog struct {
	Locale string
	Texts  [][PluralForms]string
}

type Manifest struct {
	Version   uint32
	Messages  []string
	Catalogs  []Catalog
	Routes    []Route
	Plans     []Plan
	Fallbacks []Fallback
}

const Version uint32 = 7

func (m *Manifest) Catalog(locale string) (*Catalog, bool) {
	for i := range m.Catalogs {
		if m.Catalogs[i].Locale == locale {
			return &m.Catalogs[i], true
		}
	}
	return nil, false
}

func (c *Catalog) Text(message uint32, form int) (string, bool) {
	if message >= uint32(len(c.Texts)) || form < 0 || form >= PluralForms {
		return "", false
	}
	forms := c.Texts[message]
	if forms[form] != "" {
		return forms[form], true
	}
	return forms[0], forms[0] != ""
}

func (m *Manifest) Fallback(kind FallbackKind, path string) (Fallback, bool) {
	best := -1
	for i, fallback := range m.Fallbacks {
		if fallback.Kind != kind || !underPrefix(path, fallback.Prefix) {
			continue
		}
		if best < 0 || len(fallback.Prefix) > len(m.Fallbacks[best].Prefix) {
			best = i
		}
	}
	if best < 0 {
		return Fallback{}, false
	}
	return m.Fallbacks[best], true
}

func underPrefix(path, prefix string) bool {
	if prefix == "/" || prefix == "" {
		return true
	}
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func (m *Manifest) Chain(route Route) []*Plan {
	chain := make([]*Plan, 0, len(route.LayoutChain)+1)
	for _, index := range route.LayoutChain {
		if index < uint32(len(m.Plans)) {
			chain = append(chain, &m.Plans[index])
		}
	}
	if route.Plan < uint32(len(m.Plans)) {
		chain = append(chain, &m.Plans[route.Plan])
	}
	return chain
}

func (m *Manifest) Lookup(pattern string) (Route, bool) {
	for _, route := range m.Routes {
		if route.Pattern == pattern {
			return route, true
		}
	}
	return Route{}, false
}

type BinaryOp uint8

const (
	BinaryOr BinaryOp = iota
	BinaryAnd
	BinaryEq
	BinaryNe
	BinaryLt
	BinaryLe
	BinaryGt
	BinaryGe
	BinaryConcat
	BinaryAdd
	BinarySub
	BinaryMul
	BinaryDiv
	BinaryMod
)

var binaryNames = map[BinaryOp]string{
	BinaryOr: "||", BinaryAnd: "&&",
	BinaryEq: "==", BinaryNe: "!=", BinaryLt: "<", BinaryLe: "<=", BinaryGt: ">", BinaryGe: ">=",
	BinaryConcat: "~", BinaryAdd: "+", BinarySub: "-", BinaryMul: "*", BinaryDiv: "/", BinaryMod: "%",
}

func (o BinaryOp) String() string {
	if name, ok := binaryNames[o]; ok {
		return name
	}
	return "unknown operator"
}

type UnaryOp uint8

const (
	UnaryNot UnaryOp = iota
	UnaryNeg
)

func (o UnaryOp) String() string {
	if o == UnaryNeg {
		return "-"
	}
	return "!"
}
