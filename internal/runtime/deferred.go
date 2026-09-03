package runtime

import (
	"time"

	"github.com/sonquer/rill/internal/ir"
)

const (
	SlotTag       = "rill-slot"
	SlotAttribute = "data-rill-slot"
)

type Budget time.Duration

const NoWait Budget = -1

func (b Budget) Unlimited() bool {
	return b == 0
}

type Deferred interface {
	Await(fragment ir.Fragment) (Accessible, error)
	Settle(fragment ir.Fragment, budget Budget) bool
	Flush()
}

const FetchAttribute = "fetch"

func SlotOpen(name string, fetched bool, strategy string) string {
	switch {
	case !fetched:
		return `<` + SlotTag + ` name="` + name + `">`
	case strategy == "":
		return `<` + SlotTag + ` name="` + name + `" ` + FetchAttribute + `>`
	default:
		return `<` + SlotTag + ` name="` + name + `" ` + FetchAttribute + `="` + strategy + `">`
	}
}

func SlotClose() string {
	return `</` + SlotTag + `>`
}

func TemplateOpen(name string) string {
	return `<template ` + SlotAttribute + `="` + name + `">`
}

func TemplateClose() string {
	return `</template>`
}

func (s *scope) deferredFragment(op ir.Op, hook Deferred) (ir.Fragment, bool) {
	fragment, ok := s.plan.Fragment(op.A)
	if !ok || !fragment.Deferred || hook == nil {
		return fragment, false
	}
	return fragment, true
}

func RenderFragment(plan *ir.Plan, fragment ir.Fragment, props Accessible, out *Buffer, opts Options) error {
	from, to, ok := bounds(plan, fragment)
	if !ok {
		return nil
	}
	state := scope{plan: plan, props: props}
	if len(plan.Messages) > 0 {
		state.catalog = opts.Catalog
		state.plural = opts.Plural
	}
	if plan.Locals > 0 {
		state.locals = make([]Value, plan.Locals)
	}
	return runRange([]*ir.Plan{plan}, 0, &state, out, &opts, from, to)
}

func bounds(plan *ir.Plan, fragment ir.Fragment) (int, int, bool) {
	for index, op := range plan.Ops {
		if op.Kind != ir.OpFragment {
			continue
		}
		named, ok := plan.Fragment(op.A)
		if !ok || named.Name != fragment.Name {
			continue
		}
		if named.BodyEnd > uint32(index) {
			return index + 1, int(named.BodyEnd), true
		}
		return index + 1, int(op.B), true
	}
	return 0, 0, false
}
