package runtime

import (
	"fmt"
	"sync"

	"github.com/sonquer/rill/internal/i18n"
	"github.com/sonquer/rill/internal/ir"
)

var escapes = buildEscapes()

func buildEscapes() [256]string {
	var table [256]string
	table['&'] = "&amp;"
	table['<'] = "&lt;"
	table['>'] = "&gt;"
	table['"'] = "&#34;"
	table['\''] = "&#39;"
	return table
}

func AppendEscaped(dst []byte, s string) []byte {
	last := 0
	for i := range len(s) {
		escape := escapes[s[i]]
		if escape == "" {
			continue
		}
		dst = append(dst, s[last:i]...)
		dst = append(dst, escape...)
		last = i + 1
	}
	return append(dst, s[last:]...)
}

type Buffer struct {
	buf []byte
}

func NewBuffer(capacity uint32) *Buffer {
	return &Buffer{buf: make([]byte, 0, capacity)}
}

func (b *Buffer) Write(p []byte) {
	b.buf = append(b.buf, p...)
}

func (b *Buffer) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

func (b *Buffer) WriteEscaped(s string) {
	b.buf = AppendEscaped(b.buf, s)
}

func (b *Buffer) WriteJSON(value Value) {
	b.buf = AppendJSON(b.buf, value)
}

func (b *Buffer) WriteURL(s string) {
	if !SafeURL(s) {
		return
	}
	b.buf = AppendEscaped(b.buf, s)
}

func (b *Buffer) Bytes() []byte {
	return b.buf
}

func (b *Buffer) String() string {
	return string(b.buf)
}

func (b *Buffer) Len() int {
	return len(b.buf)
}

func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
}

var pool = sync.Pool{New: func() any { return &Buffer{} }}

func Acquire(capacity uint32) *Buffer {
	b, _ := pool.Get().(*Buffer)
	b.Reset()
	if uint32(cap(b.buf)) < capacity {
		b.buf = make([]byte, 0, capacity)
	}
	return b
}

func Release(b *Buffer) {
	pool.Put(b)
}

func Render(chain []*ir.Plan, props Accessible, out *Buffer) error {
	return RenderWith(chain, props, out, nil)
}

type Fragments interface {
	Load(fragment ir.Fragment, key string) ([]byte, bool)
	Save(fragment ir.Fragment, key string, body []byte)
}

type Options struct {
	Fragments Fragments
	Deferred  Deferred
	Budget    Budget
	Fetched   bool
	Markers   bool
	Catalog   *ir.Catalog
	Plural    i18n.Rule
	Preload   string

	recording int
}

func RenderWith(chain []*ir.Plan, props Accessible, out *Buffer, hook Fragments) error {
	return RenderOptions(chain, props, out, Options{Fragments: hook})
}

func RenderOptions(chain []*ir.Plan, props Accessible, out *Buffer, opts Options) error {
	if len(chain) == 0 {
		return nil
	}
	return renderPlan(chain, 0, props, out, &opts)
}

type cursor struct {
	seq   Sequence
	index int
	slot  uint32
}

func renderPlan(chain []*ir.Plan, planIndex int, props Accessible, out *Buffer, opts *Options) error {
	plan := chain[planIndex]
	state := scope{plan: plan, props: props}
	if len(plan.Messages) > 0 {
		state.catalog = opts.Catalog
		state.plural = opts.Plural
	}
	if plan.Locals > 0 {
		state.locals = make([]Value, plan.Locals)
	}
	return runOps(chain, planIndex, &state, out, opts)
}

func runOps(chain []*ir.Plan, planIndex int, state *scope, out *Buffer, opts *Options) error {
	return runRange(chain, planIndex, state, out, opts, 0, len(state.plan.Ops))
}

func runRange(chain []*ir.Plan, planIndex int, state *scope, out *Buffer, opts *Options, from, to int) error {
	plan := state.plan
	ops := plan.Ops
	var cursors []cursor
	var pending []fragmentSave
	nextSave := to

	for pc := from; pc < to; {
		for pc >= nextSave {
			top := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			opts.recording--
			opts.Fragments.Save(top.fragment, top.key, out.Bytes()[top.start:])
			nextSave = to
			if len(pending) > 0 {
				nextSave = pending[len(pending)-1].end
			}
		}
		op := ops[pc]
		switch op.Kind {
		case ir.OpStatic:
			out.Write(plan.Static(op))
			pc++
		case ir.OpText:
			value, err := state.text(op.A)
			if err != nil {
				return err
			}
			out.WriteEscaped(value)
			pc++
		case ir.OpURL:
			value, err := state.text(op.A)
			if err != nil {
				return err
			}
			out.WriteURL(value)
			pc++
		case ir.OpOutlet:
			if opts.Markers {
				out.Write(openMarker(planIndex))
			}
			if planIndex+1 < len(chain) {
				if err := renderPlan(chain, planIndex+1, state.props, out, opts); err != nil {
					return err
				}
			}
			if opts.Markers {
				out.Write(closeMarker(planIndex))
			}
			pc++
		case ir.OpJSON:
			value, err := state.eval(op.A)
			if err != nil {
				return err
			}
			out.WriteJSON(value)
			pc++
		case ir.OpPreload:
			out.WriteString(opts.Preload)
			pc++
		case ir.OpFragment:
			fragment, deferred := state.deferredFragment(op, opts.Deferred)
			if deferred {
				if opts.recording == 0 {
					opts.Deferred.Flush()
				}
				if !opts.Deferred.Settle(fragment, opts.Budget) {
					out.Write([]byte(SlotOpen(fragment.Name, opts.Fetched, fragment.Strategy)))
					if fragment.Held() {
						if err := runRange(chain, planIndex, state, out, opts,
							int(fragment.Hold), int(fragment.HoldEnd)); err != nil {
							return err
						}
					}
					out.Write([]byte(SlotClose()))
					pc = int(op.B)
					continue
				}
				held, err := opts.Deferred.Await(fragment)
				if err != nil {
					return err
				}
				outer := state.props
				state.props = WithRoot(outer, fragment.Name, held)
				if err := runRange(chain, planIndex, state, out, opts, pc+1, int(op.B)); err != nil {
					state.props = outer
					return err
				}
				state.props = outer
				if opts.recording == 0 {
					opts.Deferred.Flush()
				}
				pc = int(op.B)
				continue
			}
			skip, save, err := state.openFragment(op, out, opts.Fragments)
			if err != nil {
				return err
			}
			if skip {
				pc = int(op.B)
				continue
			}
			if save.key != "" {
				pending = append(pending, save)
				opts.recording++
				nextSave = save.end
			}
			pc++
		case ir.OpJumpIfFalse:
			value, err := state.eval(op.A)
			if err != nil {
				return err
			}
			if value.Truthy() {
				pc++
			} else {
				pc = int(op.B)
			}
		case ir.OpJump:
			if int(op.B) <= pc {
				return fmt.Errorf("plan jumps backwards from %d to %d, which no compiler output does", pc, op.B)
			}
			pc = int(op.B)
		case ir.OpLet:
			value, err := state.eval(op.B)
			if err != nil {
				return err
			}
			if err := state.assign(op.A, value); err != nil {
				return err
			}
			pc++
		case ir.OpIterStart:
			value, err := state.eval(op.A)
			if err != nil {
				return err
			}
			seq := value.Sequence()
			if value.Kind != KindSeq || seq == nil {
				return fmt.Errorf("cannot loop over a %s", kindName(value.Kind))
			}
			if seq.Len() == 0 {
				pc = int(op.C)
				continue
			}
			if err := state.assign(op.B, seq.At(0)); err != nil {
				return err
			}
			cursors = append(cursors, cursor{seq: seq, index: 0, slot: op.B})
			pc++
		case ir.OpIterNext:
			if len(cursors) == 0 {
				return fmt.Errorf("plan advances a loop that never started")
			}
			top := &cursors[len(cursors)-1]
			top.index++
			if top.index < top.seq.Len() {
				state.locals[top.slot] = top.seq.At(top.index)
				pc = int(op.B)
				continue
			}
			cursors = cursors[:len(cursors)-1]
			pc++
		default:
			return fmt.Errorf("plan uses %s, which this runtime does not know", op.Kind)
		}
	}
	for i := len(pending) - 1; i >= 0; i-- {
		opts.recording--
		opts.Fragments.Save(pending[i].fragment, pending[i].key, out.Bytes()[pending[i].start:])
	}
	return nil
}

func (s *scope) assign(slot uint32, value Value) error {
	if slot >= uint32(len(s.locals)) {
		return fmt.Errorf("local %d is out of range", slot)
	}
	s.locals[slot] = value
	return nil
}

func Capacity(chain []*ir.Plan) uint32 {
	var total uint32
	for _, plan := range chain {
		total += plan.Capacity
	}
	return total
}
