package compile

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/ir"
	"github.com/sonquer/rill/internal/runtime"
	"github.com/sonquer/rill/internal/schema"
	"github.com/sonquer/rill/internal/syntax"
)

const capacitySlack = 130

type frame struct {
	floor    int
	children []syntax.Node
	slots    []syntax.Slot
}

type builder struct {
	file       string
	bag        *diag.Bag
	isLayout   bool
	assets     string
	components map[string]Component
	floor      int
	frames     []frame
	active     []string
	ops        []ir.Op
	exprs      []ir.ExprNode
	consts     []ir.Const
	blob       []byte
	paths      [][]string
	pathIndex  map[string]uint32
	constIndex map[string]uint32
	locals     []string
	maxLocals  uint32
	fragments  []ir.Fragment
	messages   *messageTable
	islands    map[string]bool
	clientSeen bool
	nested     int
	deferred   map[string]bool
	fetches    bool
	inventory  *inventory
	reads      *[]uint32
	mergeable  int
	uses       []ir.IslandUse
}

type LowerOptions struct {
	IsLayout   bool
	Components map[string]Component
	Assets     string
	Messages   *messageTable
	Islands    map[string]bool
	Classes    *inventory
	Deferred   map[string]bool
	Fetches    bool
}

func newBuilder(file string, opts LowerOptions, bag *diag.Bag) *builder {
	return &builder{
		file:       file,
		bag:        bag,
		isLayout:   opts.IsLayout,
		assets:     opts.Assets,
		messages:   opts.Messages,
		islands:    opts.Islands,
		inventory:  opts.Classes,
		components: opts.Components,
		deferred:   opts.Deferred,
		fetches:    opts.Fetches,
		pathIndex:  map[string]uint32{},
		constIndex: map[string]uint32{},
		mergeable:  -1,
	}
}

func Lower(doc *syntax.Document, file string, opts LowerOptions, bag *diag.Bag) ir.Plan {
	b := newBuilder(file, opts, bag)
	b.nodes(doc.Nodes)
	return b.plan()
}

func (b *builder) plan() ir.Plan {
	return ir.Plan{
		Messages:  b.messageKeys(),
		Fragments: b.fragments,
		Islands:   b.uses,
		Ops:       b.ops,
		Exprs:     b.exprs,
		Consts:    b.consts,
		Blob:      b.blob,
		Paths:     b.paths,
		Locals:    b.maxLocals,
		Capacity:  uint32(len(b.blob)) * capacitySlack / 100,
	}
}

func (b *builder) nodes(nodes []syntax.Node) {
	for _, node := range nodes {
		b.node(node)
	}
}

func (b *builder) node(node syntax.Node) {
	switch n := node.(type) {
	case *syntax.Text:
		b.static(n.Value)
	case *syntax.Interpolation:
		b.emit(ir.Op{Kind: ir.OpText, A: b.expr(n.Expr)})
	case *syntax.ClientScript:
		b.clientScript(n)
	case *syntax.Outlet:
		b.outlet(n)
	case *syntax.Let:
		b.let(n)
	case *syntax.If:
		b.ifNode(n)
	case *syntax.For:
		b.forNode(n)
	case *syntax.Element:
		b.element(n)
	case *syntax.MetaBlock:
		b.meta()
	case *syntax.AssetsBlock:
		b.static(b.assets)
		b.emit(ir.Op{Kind: ir.OpPreload})
	case *syntax.Fragment:
		b.fragment(n)
	case *syntax.Match:
		b.matchNode(n)
	case *syntax.Component:
		b.component(n)
	case *syntax.Children:
		b.fill(nil)
	case *syntax.SlotOutlet:
		b.fill(&n.Name)
	}
}

func (b *builder) fill(slot *string) {
	if len(b.frames) == 0 {
		return
	}
	top := b.frames[len(b.frames)-1]
	nodes := top.children
	if slot != nil {
		nodes, _ = slotNodes(top.slots, *slot)
	}
	saved := b.floor
	b.floor = top.floor
	b.frames = b.frames[:len(b.frames)-1]
	b.nodes(nodes)
	b.frames = append(b.frames, top)
	b.floor = saved
}

func slotNodes(slots []syntax.Slot, name string) ([]syntax.Node, bool) {
	for _, slot := range slots {
		if slot.Name == name {
			return slot.Nodes, true
		}
	}
	return nil, false
}

func (b *builder) outlet(node *syntax.Outlet) {
	if !b.isLayout {
		b.report(diag.C103, node.Span, "outlet is only meaningful inside a layout",
			"pages are rendered into the outlet of their layout; remove this directive")
		return
	}
	b.emit(ir.Op{Kind: ir.OpOutlet})
}

func (b *builder) let(node *syntax.Let) {
	value := b.expr(node.Value)
	slot := b.declare(node.Name)
	b.emit(ir.Op{Kind: ir.OpLet, A: slot, B: value})
}

func (b *builder) ifNode(node *syntax.If) {
	b.nested++
	defer func() { b.nested-- }()
	var exits []int
	for _, branch := range node.Branches {
		if branch.Cond == nil {
			b.nodes(branch.Body)
			continue
		}
		test := b.emit(ir.Op{Kind: ir.OpJumpIfFalse, A: b.expr(branch.Cond)})
		b.nodes(branch.Body)
		exits = append(exits, b.emit(ir.Op{Kind: ir.OpJump}))
		b.ops[test].B = uint32(len(b.ops))
	}
	end := uint32(len(b.ops))
	for _, exit := range exits {
		b.ops[exit].B = end
	}
}

func (b *builder) matchNode(node *syntax.Match) {
	b.nested++
	defer func() { b.nested-- }()
	subject := b.expr(node.Subject)
	var exits []int
	for _, arm := range node.Arms {
		name := b.constant(ir.Const{Kind: ir.ConstString, Str: arm.Name}, "s"+arm.Name)
		test := b.emitExpr(ir.ExprNode{Kind: ir.ExprBinary, Op: uint8(ir.BinaryEq), A: subject, B: name})
		jump := b.emit(ir.Op{Kind: ir.OpJumpIfFalse, A: test})
		b.nodes(arm.Body)
		exits = append(exits, b.emit(ir.Op{Kind: ir.OpJump}))
		b.patch(jump, b.here())
	}
	end := b.here()
	for _, exit := range exits {
		b.patch(exit, end)
	}
}

func (b *builder) forNode(node *syntax.For) {
	b.nested++
	defer func() { b.nested-- }()
	seq := b.expr(node.Seq)
	depth := len(b.locals)
	slot := b.declare(node.Var)

	start := b.emit(ir.Op{Kind: ir.OpIterStart, A: seq, B: slot})
	bodyStart := b.here()
	b.nodes(node.Body)
	b.emit(ir.Op{Kind: ir.OpIterNext, A: slot, B: bodyStart})
	b.locals = b.locals[:depth]

	skip := b.emit(ir.Op{Kind: ir.OpJump})
	b.ops[start].C = b.here()
	b.mergeable = -1
	b.nodes(node.Empty)
	b.patch(skip, b.here())
}

func (b *builder) emit(op ir.Op) int {
	b.ops = append(b.ops, op)
	index := len(b.ops) - 1
	if op.Kind == ir.OpStatic {
		b.mergeable = index
	} else {
		b.mergeable = -1
	}
	return index
}

func (b *builder) patch(op int, target uint32) {
	b.ops[op].B = target
	if int(target) >= len(b.ops) {
		b.mergeable = -1
	}
}

func (b *builder) here() uint32 {
	return uint32(len(b.ops))
}

func (b *builder) static(text string) {
	if text == "" {
		return
	}
	start := uint32(len(b.blob))
	b.blob = append(b.blob, text...)
	if last := b.mergeable; last == len(b.ops)-1 && last >= 0 &&
		b.ops[last].A+b.ops[last].B == start {
		b.ops[last].B += uint32(len(text))
		return
	}
	b.emit(ir.Op{Kind: ir.OpStatic, A: start, B: uint32(len(text))})
}

func (b *builder) declare(name string) uint32 {
	slot := uint32(len(b.locals))
	b.locals = append(b.locals, name)
	b.maxLocals = max(b.maxLocals, slot+1)
	return slot
}

func (b *builder) lookup(name string) (uint32, bool) {
	for i := len(b.locals) - 1; i >= b.floor; i-- {
		if b.locals[i] == name {
			return uint32(i), true
		}
	}
	return 0, false
}

func (b *builder) expr(node syntax.Expr) uint32 {
	switch n := node.(type) {
	case *syntax.Path:
		return b.path(n)
	case *syntax.StringLit:
		return b.constant(ir.Const{Kind: ir.ConstString, Str: n.Value}, "s"+n.Value)
	case *syntax.IntLit:
		return b.constant(ir.Const{Kind: ir.ConstInt, Int: n.Value}, "i"+strconv.FormatInt(n.Value, 10))
	case *syntax.FloatLit:
		return b.constant(ir.Const{Kind: ir.ConstFloat, Float: n.Value},
			"f"+strconv.FormatFloat(n.Value, 'g', -1, 64))
	case *syntax.BoolLit:
		return b.boolean(n.Value)
	case *syntax.Unary:
		return b.emitExpr(ir.ExprNode{Kind: ir.ExprUnary, Op: unaryOp(n.Op), A: b.expr(n.Operand)})
	case *syntax.Binary:
		left := b.expr(n.Left)
		right := b.expr(n.Right)
		return b.emitExpr(ir.ExprNode{Kind: ir.ExprBinary, Op: binaryOp(n.Op), A: left, B: right})
	case *syntax.FilterCall:
		return b.filterCall(n)
	case *syntax.MessageCall:
		return b.messageCall(n)
	case *syntax.Index:
		base := b.expr(n.Base)
		index := b.expr(n.Index)
		return b.emitExpr(ir.ExprNode{Kind: ir.ExprIndex, A: base, B: index})
	default:
		return b.constant(ir.Const{Kind: ir.ConstString}, "s")
	}
}

func (b *builder) filterCall(node *syntax.FilterCall) uint32 {
	input := b.expr(node.Input)
	id, filter, known := runtime.LookupFilter(node.Name)
	if !known {
		b.report(diag.C313, node.NameSpan, fmt.Sprintf("no filter named %s", node.Name),
			b.filterHelp(node.Name))
		return input
	}
	argument := runtime.NoArgument
	switch {
	case filter.Arity == 0 && node.Argument != nil:
		b.report(diag.C313, node.NameSpan, fmt.Sprintf("%s takes no argument", node.Name),
			fmt.Sprintf("write {{ value | %s }}", node.Name))
		return input
	case filter.Arity == 1 && node.Argument == nil:
		b.report(diag.C313, node.NameSpan, fmt.Sprintf("%s needs one argument", node.Name),
			fmt.Sprintf(`write {{ value | %s(…) }}`, node.Name))
		return input
	case node.Argument != nil:
		argument = b.expr(node.Argument)
	}
	return b.emitExpr(ir.ExprNode{Kind: ir.ExprFilter, Op: uint8(id), A: input, B: argument})
}

func (b *builder) filterHelp(name string) string {
	names := runtime.FilterNames()
	if suggestion := schema.Suggest(name, names); suggestion != "" {
		return fmt.Sprintf("did you mean %s?", suggestion)
	}
	return "filters: " + strings.Join(names, ", ")
}

func (b *builder) path(node *syntax.Path) uint32 {
	if slot, ok := b.lookup(node.Segments[0]); ok {
		rest := ir.NoPath
		if len(node.Segments) > 1 {
			rest = b.pathOf(node.Segments[1:])
		}
		return b.emitExpr(ir.ExprNode{Kind: ir.ExprLocal, A: slot, B: rest})
	}
	return b.emitExpr(ir.ExprNode{Kind: ir.ExprPath, A: b.pathOf(node.Segments)})
}

func (b *builder) pathOf(segments []string) uint32 {
	key := strings.Join(segments, ".")
	if index, ok := b.pathIndex[key]; ok {
		b.record(index)
		return index
	}
	index := uint32(len(b.paths))
	b.paths = append(b.paths, segments)
	b.pathIndex[key] = index
	b.record(index)
	return index
}

func (b *builder) record(index uint32) {
	if b.reads == nil || slices.Contains(*b.reads, index) {
		return
	}
	*b.reads = append(*b.reads, index)
}

func (b *builder) constant(value ir.Const, key string) uint32 {
	if index, ok := b.constIndex[key]; ok {
		return b.emitExpr(ir.ExprNode{Kind: ir.ExprConst, A: index})
	}
	index := uint32(len(b.consts))
	b.consts = append(b.consts, value)
	b.constIndex[key] = index
	return b.emitExpr(ir.ExprNode{Kind: ir.ExprConst, A: index})
}

func (b *builder) integer(text string) uint32 {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return b.constant(ir.Const{Kind: ir.ConstInt}, "i0")
	}
	return b.constant(ir.Const{Kind: ir.ConstInt, Int: value}, "i"+strconv.FormatInt(value, 10))
}

func (b *builder) real(text string) uint32 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return b.constant(ir.Const{Kind: ir.ConstFloat}, "f0")
	}
	return b.constant(ir.Const{Kind: ir.ConstFloat, Float: value},
		"f"+strconv.FormatFloat(value, 'g', -1, 64))
}

func (b *builder) boolean(value bool) uint32 {
	number := int64(0)
	if value {
		number = 1
	}
	return b.constant(ir.Const{Kind: ir.ConstBool, Int: number}, "b"+strconv.FormatBool(value))
}

func (b *builder) emitExpr(node ir.ExprNode) uint32 {
	b.exprs = append(b.exprs, node)
	return uint32(len(b.exprs) - 1)
}

func (b *builder) report(code diag.Code, span diag.Span, message, help string) {
	b.bag.Add(diag.New(code, b.file, span, message).WithHelp(help))
}

var binaryOps = map[syntax.BinaryOp]ir.BinaryOp{
	syntax.OpOr:     ir.BinaryOr,
	syntax.OpAnd:    ir.BinaryAnd,
	syntax.OpEq:     ir.BinaryEq,
	syntax.OpNe:     ir.BinaryNe,
	syntax.OpLt:     ir.BinaryLt,
	syntax.OpLe:     ir.BinaryLe,
	syntax.OpGt:     ir.BinaryGt,
	syntax.OpGe:     ir.BinaryGe,
	syntax.OpConcat: ir.BinaryConcat,
	syntax.OpAdd:    ir.BinaryAdd,
	syntax.OpSub:    ir.BinarySub,
	syntax.OpMul:    ir.BinaryMul,
	syntax.OpDiv:    ir.BinaryDiv,
	syntax.OpMod:    ir.BinaryMod,
}

func binaryOp(op syntax.BinaryOp) uint8 {
	return uint8(binaryOps[op])
}

func unaryOp(op syntax.UnaryOp) uint8 {
	if op == syntax.OpNeg {
		return uint8(ir.UnaryNeg)
	}
	return uint8(ir.UnaryNot)
}

const (
	LoaderName = "func Load("
	MetaName   = "func Meta("
	SubmitName = "func Submit("
)

type Template struct {
	File        string
	Source      string
	Document    *syntax.Document
	IsLayout    bool
	Frontmatter string
	FirstLine   int
}

func (t Template) HasLoader() bool {
	return strings.Contains(t.Frontmatter, LoaderName)
}

func (t Template) HasMeta() bool {
	return strings.Contains(t.Frontmatter, MetaName)
}

func (t Template) LoaderTakesParams() bool {
	return LoaderTakesParams(t.Frontmatter, t.File)
}

func (t Template) HasSubmit() bool {
	return strings.Contains(t.Frontmatter, SubmitName)
}

func (t Template) Sources() []schema.Source {
	if strings.TrimSpace(t.Frontmatter) == "" {
		return nil
	}
	return []schema.Source{{File: t.File, Code: t.Frontmatter, Line: t.FirstLine}}
}

func ReadTemplate(fsys fs.FS, file string, bag *diag.Bag) (Template, bool) {
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		bag.Add(diag.New(diag.C101, file, diag.Span{}, fmt.Sprintf("cannot read %s: %v", file, err)))
		return Template{}, false
	}
	source := string(data)
	document := syntax.Parse(file, source, bag)
	template := Template{
		File:     file,
		Source:   source,
		Document: document,
		IsLayout: strings.HasSuffix(file, LayoutFile),
	}
	if document.Frontmatter != nil {
		template.Frontmatter = document.Frontmatter.Code
		template.FirstLine = diag.PositionOf(source, document.Frontmatter.Span.Start).Line
	}
	return template, true
}
