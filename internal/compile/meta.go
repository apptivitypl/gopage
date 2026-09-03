package compile

import (
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

type metaTag struct {
	before string
	field  string
	after  string
}

var metaTags = []metaTag{
	{"<title>", "Title", "</title>"},
	{`<meta name="description" content="`, "Description", `">`},
	{`<link rel="canonical" href="`, "Canonical", `">`},
	{`<meta property="og:title" content="`, "Title", `">`},
	{`<meta property="og:description" content="`, "Description", `">`},
	{`<meta property="og:image" content="`, "Image", `">`},
	{`<meta name="robots" content="`, "Robots", `">`},
}

func (b *builder) meta() {
	b.metaTags()
	b.alternates()
}

func (b *builder) alternates() {
	seq := b.emitExpr(ir.ExprNode{Kind: ir.ExprPath, A: b.pathOf([]string{runtime.MetaRoot, runtime.AlternatesField})})
	slot := b.declare("__alternate")
	start := b.emit(ir.Op{Kind: ir.OpIterStart, A: seq, B: slot})
	body := b.here()

	b.static(`<link rel="alternate" hreflang="`)
	b.emit(ir.Op{Kind: ir.OpText, A: b.local(slot, []string{"Lang"})})
	b.static(`" href="`)
	b.emit(ir.Op{Kind: ir.OpText, A: b.local(slot, []string{"Href"})})
	b.static(`">`)

	b.emit(ir.Op{Kind: ir.OpIterNext, A: slot, B: body})
	b.locals = b.locals[:len(b.locals)-1]
	b.ops[start].C = b.here()
	b.mergeable = -1
}

func (b *builder) local(slot uint32, rest []string) uint32 {
	return b.emitExpr(ir.ExprNode{Kind: ir.ExprLocal, A: slot, B: b.pathOf(rest)})
}

func (b *builder) metaTags() {
	for _, tag := range metaTags {
		value := b.emitExpr(ir.ExprNode{
			Kind: ir.ExprPath,
			A:    b.pathOf([]string{runtime.MetaRoot, tag.field}),
		})
		test := b.emit(ir.Op{Kind: ir.OpJumpIfFalse, A: value})
		b.static(tag.before)
		b.emit(ir.Op{Kind: ir.OpText, A: value})
		b.static(tag.after)
		b.patch(test, b.here())
	}
}
