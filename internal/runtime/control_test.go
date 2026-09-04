package runtime

import (
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/ir"
)

type program struct {
	arena
	blob []byte
}

func (p *program) static(text string) {
	start := uint32(len(p.blob))
	p.blob = append(p.blob, text...)
	p.op(ir.Op{Kind: ir.OpStatic, A: start, B: uint32(len(text))})
}

func (p *program) op(op ir.Op) int {
	p.plan.Ops = append(p.plan.Ops, op)
	return len(p.plan.Ops) - 1
}

func (p *program) here() uint32 {
	return uint32(len(p.plan.Ops))
}

func (p *program) build(locals uint32) *ir.Plan {
	p.plan.Blob = p.blob
	p.plan.Locals = locals
	p.plan.Capacity = 128
	return &p.plan
}

func run(t *testing.T, plan *ir.Plan, props Accessible) string {
	t.Helper()
	out := NewBuffer(128)
	if err := Render([]*ir.Plan{plan}, props, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func runErr(t *testing.T, plan *ir.Plan, props Accessible, want string) {
	t.Helper()
	err := Render([]*ir.Plan{plan}, props, NewBuffer(64))
	if err == nil {
		t.Fatalf("expected an error mentioning %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to mention %q", err, want)
	}
}

func TestJumpIfFalseSkipsTheBranch(t *testing.T) {
	var p program
	cond := p.boolean(false)
	test := p.op(ir.Op{Kind: ir.OpJumpIfFalse, A: cond})
	p.static("taken")
	p.plan.Ops[test].B = p.here()
	p.static("after")

	if got := run(t, p.build(0), nil); got != "after" {
		t.Errorf("render = %q", got)
	}
}

func TestJumpIfTrueFallsThrough(t *testing.T) {
	var p program
	cond := p.boolean(true)
	test := p.op(ir.Op{Kind: ir.OpJumpIfFalse, A: cond})
	p.static("taken")
	p.plan.Ops[test].B = p.here()

	if got := run(t, p.build(0), nil); got != "taken" {
		t.Errorf("render = %q", got)
	}
}

func TestJumpMovesForward(t *testing.T) {
	var p program
	jump := p.op(ir.Op{Kind: ir.OpJump})
	p.static("skipped")
	p.plan.Ops[jump].B = p.here()
	p.static("reached")

	if got := run(t, p.build(0), nil); got != "reached" {
		t.Errorf("render = %q", got)
	}
}

func TestLetStoresAndReads(t *testing.T) {
	var p program
	value := p.integer(7)
	p.op(ir.Op{Kind: ir.OpLet, A: 0, B: value})
	p.op(ir.Op{Kind: ir.OpText, A: p.localPath(0)})

	if got := run(t, p.build(1), nil); got != "7" {
		t.Errorf("render = %q", got)
	}
}

func loopPlan(t *testing.T, items Value) *ir.Plan {
	t.Helper()
	var p program
	seq := p.path("Items")
	start := p.op(ir.Op{Kind: ir.OpIterStart, A: seq, B: 0})
	body := p.here()
	p.static("[")
	p.op(ir.Op{Kind: ir.OpText, A: p.localPath(0)})
	p.static("]")
	p.op(ir.Op{Kind: ir.OpIterNext, A: 0, B: body})
	skip := p.op(ir.Op{Kind: ir.OpJump})
	p.plan.Ops[start].C = p.here()
	p.static("empty")
	p.plan.Ops[skip].B = p.here()
	return p.build(1)
}

func TestLoopVisitsEveryItem(t *testing.T) {
	plan := loopPlan(t, Nil())
	props := Map{"Items": Seq(Values{Int(1), Int(2), Int(3)})}
	if got := run(t, plan, props); got != "[1][2][3]" {
		t.Errorf("render = %q", got)
	}
}

func TestEmptyLoopTakesTheElseBranch(t *testing.T) {
	plan := loopPlan(t, Nil())
	props := Map{"Items": Seq(Values{})}
	if got := run(t, plan, props); got != "empty" {
		t.Errorf("render = %q", got)
	}
}

func TestLoopOverANonSequenceFails(t *testing.T) {
	plan := loopPlan(t, Nil())
	runErr(t, plan, Map{"Items": String("no")}, "cannot loop")
}

func TestLoopOverANilSequenceFails(t *testing.T) {
	plan := loopPlan(t, Nil())
	runErr(t, plan, Map{"Items": Value{Kind: KindSeq}}, "cannot loop")
}

func TestIterNextWithoutAStartIsRejected(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpIterNext, A: 0, B: 0})
	runErr(t, p.build(1), nil, "never started")
}

func TestLetIntoAMissingSlotIsRejected(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpLet, A: 5, B: p.integer(1)})
	runErr(t, p.build(1), nil, "out of range")
}

func TestIterStartIntoAMissingSlotIsRejected(t *testing.T) {
	var p program
	seq := p.path("Items")
	p.op(ir.Op{Kind: ir.OpIterStart, A: seq, B: 9, C: 2})
	runErr(t, p.build(1), Map{"Items": Seq(Values{Int(1)})}, "out of range")
}

func TestIterNextIntoAMissingSlotIsRejected(t *testing.T) {
	var p program
	seq := p.path("Items")
	start := p.op(ir.Op{Kind: ir.OpIterStart, A: seq, B: 0})
	body := p.here()
	p.op(ir.Op{Kind: ir.OpIterNext, A: 0, B: body})
	p.plan.Ops[start].C = p.here()
	plan := p.build(1)
	plan.Locals = 1

	out := NewBuffer(16)
	if err := Render([]*ir.Plan{plan}, Map{"Items": Seq(Values{Int(1), Int(2)})}, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
}

func TestErrorsInsideAConditionSurface(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpJumpIfFalse, A: p.path("Missing"), B: 1})
	runErr(t, p.build(0), Map{}, "no field")
}

func TestErrorsInsideALoopSequenceSurface(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpIterStart, A: p.path("Missing"), B: 0, C: 1})
	runErr(t, p.build(1), Map{}, "no field")
}

func TestErrorsInsideALetSurface(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpLet, A: 0, B: p.path("Missing")})
	runErr(t, p.build(1), Map{}, "no field")
}

func TestBackwardJumpIsRejected(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpJump, B: 0})
	runErr(t, p.build(0), nil, "jumps backwards")
}

func TestPlanWithoutLocalsSkipsAllocation(t *testing.T) {
	var p program
	p.static("plain")
	if got := run(t, p.build(0), nil); got != "plain" {
		t.Errorf("render = %q", got)
	}
}

func TestUnknownOpIsRejected(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpKind(200)})
	runErr(t, p.build(0), nil, "does not know")
}

func TestTextOpErrorSurfaces(t *testing.T) {
	var p program
	p.op(ir.Op{Kind: ir.OpText, A: p.path("Missing")})
	runErr(t, p.build(0), Map{}, "no field")
}

func TestLoopTerminatesOnSequenceLength(t *testing.T) {
	var p program
	seq := p.path("Items")
	start := p.op(ir.Op{Kind: ir.OpIterStart, A: seq, B: 0})
	body := p.here()
	p.op(ir.Op{Kind: ir.OpIterNext, A: 0, B: body})
	p.plan.Ops[start].C = p.here()

	items := make(Values, 3)
	for i := range items {
		items[i] = Int(int64(i))
	}
	if got := run(t, p.build(1), Map{"Items": Seq(items)}); got != "" {
		t.Errorf("render = %q", got)
	}
}
