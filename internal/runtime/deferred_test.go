package runtime

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/apptivitypl/rill/internal/ir"
)

type stubDeferred struct {
	props   map[string]Accessible
	ready   map[string]bool
	fail    map[string]error
	awaits  []string
	budgets []Budget
	writes  int
}

func (s *stubDeferred) Await(fragment ir.Fragment) (Accessible, error) {
	s.awaits = append(s.awaits, fragment.Name)
	if err, ok := s.fail[fragment.Name]; ok {
		return nil, err
	}
	return s.props[fragment.Name], nil
}

func (s *stubDeferred) Settle(fragment ir.Fragment, budget Budget) bool {
	s.budgets = append(s.budgets, budget)
	if budget.Unlimited() {
		return true
	}
	return s.ready[fragment.Name]
}

func (s *stubDeferred) Flush() {
	s.writes++
}

type leaf string

func (l leaf) Get(path []string) (Value, bool) {
	if len(path) != 0 {
		return Nil(), false
	}
	return String(string(l)), true
}

func deferredPlan() *ir.Plan {
	plan := &ir.Plan{
		Fragments: []ir.Fragment{{Name: "Reviews", Deferred: true}},
		Blob:      []byte("<p>before</p><b></b><p>after</p>"),
	}
	plan.Ops = []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 13},
		{Kind: ir.OpFragment, A: 0, B: 5},
		{Kind: ir.OpStatic, A: 13, B: 3},
		{Kind: ir.OpText, A: 0},
		{Kind: ir.OpStatic, A: 16, B: 4},
		{Kind: ir.OpStatic, A: 20, B: 12},
	}
	plan.Exprs = []ir.ExprNode{{Kind: ir.ExprPath, A: 0}}
	plan.Paths = [][]string{{"Reviews"}}
	return plan
}

func renderDeferred(t *testing.T, hook *stubDeferred, budget Budget) string {
	t.Helper()
	out := Acquire(64)
	defer Release(out)
	opts := Options{Deferred: hook, Budget: budget}
	if err := RenderOptions([]*ir.Plan{deferredPlan()}, Empty{}, out, opts); err != nil {
		t.Fatalf("render: %v", err)
	}
	return out.String()
}

func TestInOrderStreamingWaitsAndFlushesAroundTheFragment(t *testing.T) {
	hook := &stubDeferred{props: map[string]Accessible{"Reviews": leaf("late")}}
	html := renderDeferred(t, hook, 0)
	if !strings.Contains(html, "<b>late</b>") {
		t.Errorf("html = %q, want the fragment rendered in place", html)
	}
	if !strings.Contains(html, "<p>before</p>") || !strings.Contains(html, "<p>after</p>") {
		t.Errorf("html = %q, want the surrounding markup", html)
	}
	if hook.writes != 2 {
		t.Errorf("flushes = %d, want one before and one after the fragment", hook.writes)
	}
	if len(hook.awaits) != 1 {
		t.Errorf("awaits = %v, want the loader awaited once", hook.awaits)
	}
}

func TestOutOfOrderStreamingLeavesASlot(t *testing.T) {
	hook := &stubDeferred{props: map[string]Accessible{"Reviews": leaf("late")}}
	html := renderDeferred(t, hook, NoWait)
	if !strings.Contains(html, `<rill-slot name="Reviews">`) || !strings.Contains(html, "</rill-slot>") {
		t.Errorf("html = %q, want a slot in place of the fragment", html)
	}
	if strings.Contains(html, "late") {
		t.Error("the fragment body must not block the document")
	}
	if len(hook.awaits) != 0 {
		t.Errorf("awaits = %v, want nothing awaited while the loader runs", hook.awaits)
	}
}

func TestOutOfOrderStreamingRendersInPlaceWhenTheLoaderIsDone(t *testing.T) {
	hook := &stubDeferred{
		props: map[string]Accessible{"Reviews": leaf("early")},
		ready: map[string]bool{"Reviews": true},
	}
	html := renderDeferred(t, hook, NoWait)
	if !strings.Contains(html, "<b>early</b>") {
		t.Errorf("html = %q, want a finished loader rendered in place", html)
	}
}

func TestALoaderFailureStopsTheRender(t *testing.T) {
	hook := &stubDeferred{fail: map[string]error{"Reviews": errors.New("upstream is down")}}
	out := Acquire(64)
	defer Release(out)
	err := RenderOptions([]*ir.Plan{deferredPlan()}, Empty{}, out, Options{Deferred: hook})
	if err == nil || !strings.Contains(err.Error(), "upstream is down") {
		t.Errorf("err = %v, want the loader failure", err)
	}
}

func TestAPlanWithoutTheHookRendersTheFragmentInline(t *testing.T) {
	out := Acquire(64)
	defer Release(out)
	props := WithRoot(Empty{}, "Reviews", leaf("inline"))
	if err := RenderOptions([]*ir.Plan{deferredPlan()}, props, out, Options{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out.String(), "<b>inline</b>") {
		t.Errorf("html = %q, want the body rendered without a deferred hook", out.String())
	}
}

func TestRenderFragmentRendersOnlyTheFragment(t *testing.T) {
	out := Acquire(64)
	defer Release(out)
	plan := deferredPlan()
	props := WithRoot(Empty{}, "Reviews", leaf("tail"))
	if err := RenderFragment(plan, plan.Fragments[0], props, out, Options{}); err != nil {
		t.Fatalf("RenderFragment: %v", err)
	}
	if got := out.String(); got != "<b>tail</b>" {
		t.Errorf("html = %q, want just the fragment body", got)
	}
}

func TestRenderFragmentIgnoresAFragmentThatIsNotInThePlan(t *testing.T) {
	out := Acquire(64)
	defer Release(out)
	if err := RenderFragment(deferredPlan(), ir.Fragment{Name: "Absent"}, Empty{}, out, Options{}); err != nil {
		t.Fatalf("RenderFragment: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("html = %q, want nothing", out.String())
	}
}

func TestSlotMarkupIsStable(t *testing.T) {
	if SlotOpen("Reviews", false, "") != `<rill-slot name="Reviews">` || SlotClose() != "</rill-slot>" {
		t.Error("the slot markup changed; the client runtime matches on it")
	}
	if TemplateOpen("Reviews") != `<template data-rill-slot="Reviews">` || TemplateClose() != "</template>" {
		t.Error("the template markup changed; the client runtime matches on it")
	}
}

func TestRenderFragmentCarriesTheCatalog(t *testing.T) {
	plan := deferredPlan()
	plan.Messages = []string{"hello"}
	out := Acquire(64)
	defer Release(out)
	props := WithRoot(Empty{}, "Reviews", leaf("text"))
	opts := Options{Catalog: &ir.Catalog{Locale: "pl"}}
	if err := RenderFragment(plan, plan.Fragments[0], props, out, opts); err != nil {
		t.Fatalf("RenderFragment: %v", err)
	}
	if out.String() != "<b>text</b>" {
		t.Errorf("html = %q", out.String())
	}
}

func TestRenderFragmentAllocatesLocals(t *testing.T) {
	plan := deferredPlan()
	plan.Locals = 2
	out := Acquire(64)
	defer Release(out)
	props := WithRoot(Empty{}, "Reviews", leaf("text"))
	if err := RenderFragment(plan, plan.Fragments[0], props, out, Options{}); err != nil {
		t.Fatalf("RenderFragment: %v", err)
	}
}

func TestAFailureInsideTheFragmentRestoresTheScope(t *testing.T) {
	plan := deferredPlan()
	plan.Exprs = []ir.ExprNode{{Kind: ir.ExprPath, A: 1}}
	plan.Paths = [][]string{{"Reviews"}, {"Missing"}}
	hook := &stubDeferred{props: map[string]Accessible{"Reviews": leaf("late")}}
	out := Acquire(64)
	defer Release(out)
	err := RenderOptions([]*ir.Plan{plan}, Empty{}, out, Options{Deferred: hook})
	if err == nil {
		t.Fatal("a fragment body that cannot render must fail the page")
	}
	if hook.writes != 1 {
		t.Errorf("flushes = %d, want the head flushed before the failure", hook.writes)
	}
}

func TestABudgetRendersALoaderThatFinishedInTime(t *testing.T) {
	hook := &stubDeferred{
		props: map[string]Accessible{"Reviews": leaf("quick")},
		ready: map[string]bool{"Reviews": true},
	}
	html := renderDeferred(t, hook, Budget(25*time.Millisecond))
	if !strings.Contains(html, "<b>quick</b>") {
		t.Errorf("html = %q, want a loader inside the budget rendered in place", html)
	}
	if len(hook.budgets) != 1 || hook.budgets[0] != Budget(25*time.Millisecond) {
		t.Errorf("budgets = %v, want the render to pass its budget down", hook.budgets)
	}
}

func TestABudgetLeavesASlotForALoaderThatRanLong(t *testing.T) {
	hook := &stubDeferred{props: map[string]Accessible{"Reviews": leaf("slow")}}
	html := renderDeferred(t, hook, Budget(time.Millisecond))
	if !strings.Contains(html, `<rill-slot name="Reviews">`) {
		t.Errorf("html = %q, want a slot once the budget ran out", html)
	}
	if strings.Contains(html, "slow") {
		t.Error("a loader past its budget must not hold the document")
	}
	if !strings.Contains(html, "<p>after</p>") {
		t.Errorf("html = %q, want the document to continue past the slot", html)
	}
}

func TestAnUnlimitedBudgetIsTheZeroValue(t *testing.T) {
	if !Budget(0).Unlimited() || NoWait.Unlimited() || Budget(time.Millisecond).Unlimited() {
		t.Error("only the zero budget waits for every loader")
	}
}

func heldPlan() *ir.Plan {
	plan := &ir.Plan{
		Fragments: []ir.Fragment{{Name: "Reviews", Deferred: true, BodyEnd: 5, Hold: 6, HoldEnd: 7}},
		Blob:      []byte("<p>before</p><b></b><p>after</p><i>hold</i>"),
	}
	plan.Ops = []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 13},
		{Kind: ir.OpFragment, A: 0, B: 7},
		{Kind: ir.OpStatic, A: 13, B: 3},
		{Kind: ir.OpText, A: 0},
		{Kind: ir.OpStatic, A: 16, B: 4},
		{Kind: ir.OpJump, B: 7},
		{Kind: ir.OpStatic, A: 32, B: 11},
		{Kind: ir.OpStatic, A: 20, B: 12},
	}
	plan.Exprs = []ir.ExprNode{{Kind: ir.ExprPath, A: 0}}
	plan.Paths = [][]string{{"Reviews"}}
	return plan
}

func renderHeld(t *testing.T, hook *stubDeferred, budget Budget) string {
	t.Helper()
	out := Acquire(64)
	defer Release(out)
	opts := Options{Deferred: hook, Budget: budget, Fetched: true}
	if err := RenderOptions([]*ir.Plan{heldPlan()}, Empty{}, out, opts); err != nil {
		t.Fatalf("render: %v", err)
	}
	return out.String()
}

func TestAPlaceholderFillsTheSlot(t *testing.T) {
	hook := &stubDeferred{props: map[string]Accessible{"Reviews": leaf("late")}}
	html := renderHeld(t, hook, NoWait)
	if !strings.Contains(html, `<rill-slot name="Reviews" fetch><i>hold</i></rill-slot>`) {
		t.Errorf("html = %q, want the placeholder inside the slot", html)
	}
	if strings.Contains(html, "late") {
		t.Error("the body must not render while the loader is out")
	}
	if !strings.Contains(html, "<p>after</p>") {
		t.Errorf("html = %q, want the document to continue past the slot", html)
	}
}

func TestAReadyLoaderJumpsOverThePlaceholder(t *testing.T) {
	hook := &stubDeferred{
		props: map[string]Accessible{"Reviews": leaf("early")},
		ready: map[string]bool{"Reviews": true},
	}
	html := renderHeld(t, hook, NoWait)
	if !strings.Contains(html, "<b>early</b>") {
		t.Errorf("html = %q, want the body in place", html)
	}
	if strings.Contains(html, "hold") {
		t.Error("a rendered body must not be followed by its placeholder")
	}
}

func TestRenderFragmentStopsAtTheBodyEnd(t *testing.T) {
	out := Acquire(64)
	defer Release(out)
	plan := heldPlan()
	props := WithRoot(Empty{}, "Reviews", leaf("tail"))
	if err := RenderFragment(plan, plan.Fragments[0], props, out, Options{}); err != nil {
		t.Fatalf("RenderFragment: %v", err)
	}
	if got := out.String(); got != "<b>tail</b>" {
		t.Errorf("html = %q, want the body without the placeholder", got)
	}
}

func TestASlotSaysWhetherTheClientShouldFetchIt(t *testing.T) {
	if SlotOpen("Reviews", true, "") != `<rill-slot name="Reviews" fetch>` {
		t.Error("a fetched slot carries the attribute the client matches on")
	}
	if SlotOpen("Reviews", false, "") != `<rill-slot name="Reviews">` {
		t.Error("a tail slot carries no attribute")
	}
}

func TestAPlaceholderThatCannotRenderFailsThePage(t *testing.T) {
	plan := heldPlan()
	plan.Ops[6] = ir.Op{Kind: ir.OpText, A: 1}
	plan.Exprs = append(plan.Exprs, ir.ExprNode{Kind: ir.ExprPath, A: 1})
	plan.Paths = append(plan.Paths, []string{"Missing"})
	out := Acquire(64)
	defer Release(out)
	hook := &stubDeferred{props: map[string]Accessible{"Reviews": leaf("late")}}
	err := RenderOptions([]*ir.Plan{plan}, Empty{}, out, Options{Deferred: hook, Budget: NoWait})
	if err == nil {
		t.Fatal("a placeholder that cannot render must fail the page")
	}
}

func TestASlotAdvertisesTheStrategyThatWillFetchIt(t *testing.T) {
	cases := map[string]string{
		SlotOpen("Latest", true, "visible"):  `<rill-slot name="Latest" fetch="visible">`,
		SlotOpen("Latest", true, ""):         `<rill-slot name="Latest" fetch>`,
		SlotOpen("Latest", false, "visible"): `<rill-slot name="Latest">`,
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("slot = %q, want %q", got, want)
		}
	}
}
