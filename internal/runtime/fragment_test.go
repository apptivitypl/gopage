package runtime

import (
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/ir"
)

func fragmentChain(paths []uint32) []*ir.Plan {
	return []*ir.Plan{{
		Fragments: []ir.Fragment{{Name: "reviews", TTL: 1, Paths: paths}},
		Ops: []ir.Op{
			{Kind: ir.OpStatic, A: 0, B: 1},
			{Kind: ir.OpFragment, A: 0, B: 3},
			{Kind: ir.OpText, A: 0},
			{Kind: ir.OpStatic, A: 1, B: 1},
			{Kind: ir.OpStatic, A: 2, B: 1},
		},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{"Note"}, {"ID"}},
		Blob:     []byte("[]!"),
		Capacity: 32,
	}}
}

type recordingFragments struct {
	stored map[string][]byte
	keys   []string
	serve  bool
}

func newFragments() *recordingFragments {
	return &recordingFragments{stored: map[string][]byte{}}
}

func (r *recordingFragments) Load(_ ir.Fragment, key string) ([]byte, bool) {
	r.keys = append(r.keys, key)
	if !r.serve {
		return nil, false
	}
	body, ok := r.stored[key]
	return body, ok
}

func (r *recordingFragments) Save(_ ir.Fragment, key string, body []byte) {
	stored := make([]byte, len(body))
	copy(stored, body)
	r.stored[key] = stored
}

func renderFragments(t *testing.T, chain []*ir.Plan, props Accessible, hook Fragments) (string, error) {
	t.Helper()
	out := Acquire(Capacity(chain))
	defer Release(out)
	if err := RenderWith(chain, props, out, hook); err != nil {
		return "", err
	}
	return out.String(), nil
}

func TestAFragmentBodyRunsBetweenItsBounds(t *testing.T) {
	got, err := renderFragments(t, fragmentChain(nil), Map{"Note": String("n")}, nil)
	if err != nil || got != "[n]!" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestASavedBodyIsServedBack(t *testing.T) {
	hook := newFragments()
	chain := fragmentChain(nil)
	if _, err := renderFragments(t, chain, Map{"Note": String("first")}, hook); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(hook.stored["reviews"]) != "first" {
		t.Fatalf("stored = %q, want only the fragment body", hook.stored["reviews"])
	}
	hook.serve = true
	got, err := renderFragments(t, chain, Map{"Note": String("second")}, hook)
	if err != nil || got != "[first]!" {
		t.Errorf("got = %q, err = %v", got, err)
	}
}

func TestTheKeyCoversEveryRecordedRead(t *testing.T) {
	hook := newFragments()
	props := Map{"Note": String("n"), "ID": String("7")}
	if _, err := renderFragments(t, fragmentChain([]uint32{0, 1}), props, hook); err != nil {
		t.Fatalf("Render: %v", err)
	}
	key := hook.keys[0]
	if !strings.HasPrefix(key, "reviews") || !strings.Contains(key, "n") || !strings.Contains(key, "7") {
		t.Errorf("key = %q", key)
	}
}

func TestAMissingReadKeepsItsPlaceInTheKey(t *testing.T) {
	hook := newFragments()
	if _, err := renderFragments(t, fragmentChain([]uint32{0, 1}), Map{"Note": String("n")}, hook); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Count(hook.keys[0], keySeparator) != 2 {
		t.Errorf("key = %q, want a slot for every read the fragment declares", hook.keys[0])
	}
}

func TestKeysDoNotCollideAcrossReadShapes(t *testing.T) {
	seen := map[string]string{}
	cases := map[string]Map{
		"the second read absent": {"Note": String("a")},
		"a separator inside the first value": {
			"Note": String("a" + keySeparator + "b"),
			"ID":   String("c"),
		},
		"the same bytes split differently": {
			"Note": String("a"),
			"ID":   String("b" + keySeparator + "c"),
		},
		"both plain": {"Note": String("a"), "ID": String("bc")},
	}
	for name, props := range cases {
		hook := newFragments()
		if _, err := renderFragments(t, fragmentChain([]uint32{0, 1}), props, hook); err != nil {
			t.Fatalf("%s: Render: %v", name, err)
		}
		key := hook.keys[0]
		if held, clash := seen[key]; clash {
			t.Errorf("%q and %q share the key %q", name, held, key)
		}
		seen[key] = name
	}
}

func TestAKeyOverAnUnknownPathIsReported(t *testing.T) {
	if _, err := renderFragments(t, fragmentChain([]uint32{9}), Map{"Note": String("n")}, newFragments()); err == nil {
		t.Error("a path outside the table must be reported")
	}
}

func TestAnUnknownFragmentIndexIsReported(t *testing.T) {
	chain := fragmentChain(nil)
	chain[0].Ops[1].A = 9
	if _, err := renderFragments(t, chain, Map{"Note": String("n")}, nil); err == nil {
		t.Error("a fragment outside the table must be reported")
	}
}

func TestAFailingBodyStopsTheRender(t *testing.T) {
	if _, err := renderFragments(t, fragmentChain(nil), Empty{}, newFragments()); err == nil {
		t.Error("a body failure must reach the caller")
	}
}

func TestAnUncacheableFragmentIsNeverStored(t *testing.T) {
	chain := fragmentChain(nil)
	chain[0].Fragments[0].TTL = 0
	hook := newFragments()
	if _, err := renderFragments(t, chain, Map{"Note": String("n")}, hook); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(hook.stored) != 0 || len(hook.keys) != 0 {
		t.Errorf("stored = %v, keys = %v", hook.stored, hook.keys)
	}
}

func TestFragmentTableLookup(t *testing.T) {
	plan := fragmentChain(nil)[0]
	if _, ok := plan.Fragment(0); !ok {
		t.Error("the first fragment must resolve")
	}
	if _, ok := plan.Fragment(9); ok {
		t.Error("an index outside the table must not resolve")
	}
	if !plan.Fragments[0].Cacheable() {
		t.Error("a fragment with a ttl is cacheable")
	}
	if (ir.Fragment{}).Cacheable() {
		t.Error("a fragment without a ttl is not cacheable")
	}
}

func TestAFragmentReachingTheEndOfThePlanIsStored(t *testing.T) {
	chain := []*ir.Plan{{
		Fragments: []ir.Fragment{{Name: "tail", TTL: 1}},
		Ops: []ir.Op{
			{Kind: ir.OpStatic, A: 0, B: 1},
			{Kind: ir.OpFragment, A: 0, B: 3},
			{Kind: ir.OpText, A: 0},
		},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{"Note"}},
		Blob:     []byte("["),
		Capacity: 32,
	}}
	hook := newFragments()
	got, err := renderFragments(t, chain, Map{"Note": String("n")}, hook)
	if err != nil || got != "[n" {
		t.Fatalf("got = %q, err = %v", got, err)
	}
	if string(hook.stored["tail"]) != "n" {
		t.Errorf("stored = %q", hook.stored["tail"])
	}
}

func TestNestedFragmentsAreSavedInnermostFirst(t *testing.T) {
	chain := []*ir.Plan{{
		Fragments: []ir.Fragment{{Name: "outer", TTL: 1}, {Name: "inner", TTL: 1}},
		Ops: []ir.Op{
			{Kind: ir.OpFragment, A: 0, B: 5},
			{Kind: ir.OpStatic, A: 0, B: 1},
			{Kind: ir.OpFragment, A: 1, B: 4},
			{Kind: ir.OpStatic, A: 1, B: 1},
			{Kind: ir.OpStatic, A: 2, B: 1},
		},
		Blob:     []byte("abc"),
		Capacity: 32,
	}}
	hook := newFragments()
	got, err := renderFragments(t, chain, Empty{}, hook)
	if err != nil || got != "abc" {
		t.Fatalf("got = %q, err = %v", got, err)
	}
	if string(hook.stored["inner"]) != "b" {
		t.Errorf("inner = %q, want only its own body", hook.stored["inner"])
	}
	if string(hook.stored["outer"]) != "abc" {
		t.Errorf("outer = %q, want the whole region", hook.stored["outer"])
	}
}

type resettingDeferred struct {
	out     *Buffer
	settled bool
	flushes int
}

func (d *resettingDeferred) Await(ir.Fragment) (Accessible, error) { return Empty{}, nil }
func (d *resettingDeferred) Settle(ir.Fragment, Budget) bool       { return d.settled }

func (d *resettingDeferred) Flush() {
	d.flushes++
	d.out.Reset()
}

func nestedDeferredChain() []*ir.Plan {
	return []*ir.Plan{{
		Fragments: []ir.Fragment{
			{Name: "shell", TTL: 1, BodyEnd: 6},
			{Name: "inner", Deferred: true, BodyEnd: 5},
		},
		Ops: []ir.Op{
			{Kind: ir.OpFragment, A: 0, B: 6},
			{Kind: ir.OpStatic, A: 0, B: 4},
			{Kind: ir.OpFragment, A: 1, B: 4},
			{Kind: ir.OpStatic, A: 4, B: 3},
			{Kind: ir.OpStatic, A: 7, B: 5},
			{Kind: ir.OpStatic, A: 12, B: 4},
		},
		Blob:     []byte("headinnaftertail"),
		Capacity: 64,
	}}
}

func TestADeferredFlushCannotTruncateAnOpenFragment(t *testing.T) {
	for _, settled := range []bool{false, true} {
		hook := newFragments()
		out := NewBuffer(64)
		deferred := &resettingDeferred{out: out, settled: settled}
		opts := Options{Fragments: hook, Deferred: deferred}

		if err := RenderOptions(nestedDeferredChain(), Map{}, out, opts); err != nil {
			t.Fatalf("settled=%v: Render: %v", settled, err)
		}
		if len(hook.stored) != 1 {
			t.Fatalf("settled=%v: stored %d fragments", settled, len(hook.stored))
		}
		for key, body := range hook.stored {
			if !strings.HasPrefix(string(body), "head") {
				t.Errorf("settled=%v: %s stored %q, want the head of the fragment kept",
					settled, key, body)
			}
		}
		if deferred.flushes != 0 {
			t.Errorf("settled=%v: flushed %d times while a fragment was open", settled, deferred.flushes)
		}
	}
}
