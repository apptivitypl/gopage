package ir

import (
	"errors"
	"reflect"
	"testing"
)

func sample() *Manifest {
	return &Manifest{
		Version: Version,
		Plans: []Plan{
			{
				Ops:      []Op{{Kind: OpStatic, A: 0, B: 5}, {Kind: OpOutlet}},
				Blob:     []byte("<main"),
				Paths:    nil,
				Capacity: 64,
			},
			{
				Ops:      []Op{{Kind: OpText, A: 0}},
				Exprs:    []ExprNode{{Kind: ExprPath, A: 0}},
				Consts:   []Const{{Kind: ConstString, Str: "hi"}},
				Blob:     nil,
				Paths:    [][]string{{"Listing", "Title"}},
				Locals:   2,
				Capacity: 32,
			},
		},
		Routes: []Route{{
			Pattern:     "/listings/[id]",
			Name:        "listings.detail",
			Plan:        1,
			LayoutChain: []uint32{0},
			Class:       ClassDynamic,
		}},
	}
}

func TestOpKindNames(t *testing.T) {
	if OpStatic.String() != "static" || OpText.String() != "text" || OpOutlet.String() != "outlet" {
		t.Error("op names are wrong")
	}
	if OpKind(99).String() != "unknown op" {
		t.Errorf("unknown op = %q", OpKind(99))
	}
}

func TestRouteClassNames(t *testing.T) {
	if ClassStatic.String() != "static" || ClassDynamic.String() != "dynamic" {
		t.Error("class names are wrong")
	}
	if RouteClass(99).String() != "unknown class" {
		t.Errorf("unknown class = %q", RouteClass(99))
	}
}

func TestStaticReadsTheBlob(t *testing.T) {
	p := &Plan{Blob: []byte("hello world")}
	if got := string(p.Static(Op{A: 0, B: 5})); got != "hello" {
		t.Errorf("Static = %q", got)
	}
}

func TestStaticRejectsOutOfRangeOps(t *testing.T) {
	p := &Plan{Blob: []byte("abc")}
	for _, op := range []Op{{A: 9, B: 1}, {A: 1, B: 99}} {
		if got := p.Static(op); got != nil {
			t.Errorf("Static(%+v) = %q, want nil", op, got)
		}
	}
}

func TestExprAndConstAreBoundsChecked(t *testing.T) {
	p := &Plan{
		Exprs:  []ExprNode{{Kind: ExprConst}},
		Consts: []Const{{Kind: ConstInt, Int: 7}},
	}
	if _, ok := p.Expr(0); !ok {
		t.Error("Expr(0) must resolve")
	}
	if _, ok := p.Expr(9); ok {
		t.Error("Expr must reject an index past the arena")
	}
	if _, ok := p.Const(0); !ok {
		t.Error("Const(0) must resolve")
	}
	if _, ok := p.Const(9); ok {
		t.Error("Const must reject an index past the table")
	}
}

func TestExprAndOperatorNames(t *testing.T) {
	if ExprPath.String() != "props path" || ExprKind(99).String() != "unknown expression" {
		t.Error("expression names are wrong")
	}
	if BinaryAdd.String() != "+" || BinaryOp(99).String() != "unknown operator" {
		t.Error("binary operator names are wrong")
	}
	if UnaryNeg.String() != "-" || UnaryNot.String() != "!" {
		t.Error("unary operator names are wrong")
	}
	if OpIterStart.String() != "iter-start" {
		t.Error("op names are wrong")
	}
}

func TestPathReadsTheTable(t *testing.T) {
	p := &Plan{Paths: [][]string{{"A", "B"}}}
	if got := p.Path(0); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("Path = %v", got)
	}
	if got := p.Path(7); got != nil {
		t.Errorf("Path out of range = %v, want nil", got)
	}
}

func TestChainPutsLayoutsBeforeThePage(t *testing.T) {
	m := sample()
	chain := m.Chain(m.Routes[0])
	if len(chain) != 2 {
		t.Fatalf("chain = %d plans, want 2", len(chain))
	}
	if chain[0] != &m.Plans[0] || chain[1] != &m.Plans[1] {
		t.Error("chain must run outermost layout first, page last")
	}
}

func TestChainSkipsDanglingIndexes(t *testing.T) {
	m := sample()
	route := Route{Plan: 99, LayoutChain: []uint32{0, 99}}
	if got := len(m.Chain(route)); got != 1 {
		t.Errorf("chain = %d plans, want only the valid layout", got)
	}
}

func TestLookupFindsRoutesByPattern(t *testing.T) {
	m := sample()
	if _, ok := m.Lookup("/listings/[id]"); !ok {
		t.Error("Lookup must find a known pattern")
	}
	if _, ok := m.Lookup("/missing"); ok {
		t.Error("Lookup must not invent routes")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	original := sample()
	decoded, err := Decode(Encode(original))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("round trip changed the manifest:\ngot  %+v\nwant %+v", decoded, original)
	}
}

func TestEncodeDecodeEmptyManifest(t *testing.T) {
	original := &Manifest{Version: Version, Plans: []Plan{}, Routes: []Route{}}
	decoded, err := Decode(Encode(original))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded.Plans) != 0 || len(decoded.Routes) != 0 {
		t.Errorf("decoded = %+v", decoded)
	}
}

func TestDecodeRejectsForeignData(t *testing.T) {
	if _, err := Decode([]byte("nope")); !errors.Is(err, ErrBadMagic) {
		t.Errorf("Decode = %v, want ErrBadMagic", err)
	}
	if _, err := Decode(nil); !errors.Is(err, ErrBadMagic) {
		t.Errorf("Decode(nil) = %v, want ErrBadMagic", err)
	}
}

func TestDecodeRejectsAForeignVersion(t *testing.T) {
	data := Encode(&Manifest{Version: Version + 1})
	if _, err := Decode(data); err == nil {
		t.Error("expected a version mismatch error")
	}
}

func TestDecodeRejectsTruncatedData(t *testing.T) {
	full := Encode(sample())
	for cut := len(magic); cut < len(full); cut += 3 {
		if _, err := Decode(full[:cut]); err == nil {
			t.Fatalf("Decode accepted %d truncated bytes", cut)
		}
	}
}

func TestDecodeNeverPanicsOnGarbage(t *testing.T) {
	inputs := [][]byte{
		append(magic[:], 0xff, 0xff, 0xff, 0xff),
		append(magic[:], 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff),
	}
	for _, data := range inputs {
		if _, err := Decode(data); err == nil {
			t.Errorf("Decode(%v) must fail", data)
		}
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(Encode(sample()))
	f.Add(Encode(&Manifest{Version: Version}))
	f.Add([]byte("RILL"))
	f.Fuzz(func(t *testing.T, data []byte) {
		manifest, err := Decode(data)
		if err != nil {
			return
		}
		if manifest.Version != Version {
			t.Fatalf("accepted version %d", manifest.Version)
		}
		for i := range manifest.Plans {
			for _, op := range manifest.Plans[i].Ops {
				manifest.Plans[i].Static(op)
				manifest.Plans[i].Path(op.A)
				manifest.Plans[i].Expr(op.A)
				manifest.Plans[i].Const(op.A)
			}
		}
	})
}

func TestDecodeRejectsTruncationInsideEveryStructure(t *testing.T) {
	manifest := &Manifest{
		Version: Version,
		Plans: []Plan{{
			Fragments: []Fragment{
				{Name: "reviews", TTL: 300, Stale: 60, Paths: []uint32{0, 0}},
				{Name: "similar", TTL: 60},
			},
			Ops:      []Op{{Kind: OpIterStart, A: 1, B: 2, C: 3}},
			Exprs:    []ExprNode{{Kind: ExprBinary, Op: 3, A: 1, B: 2}},
			Consts:   []Const{{Kind: ConstFloat, Str: "s", Int: 5, Float: 1.5}},
			Blob:     []byte("abc"),
			Paths:    [][]string{{"A", "B"}},
			Locals:   4,
			Messages: []string{"hello"},
		}},
		Routes: []Route{{Pattern: "/p", Name: "n", Plan: 0, LayoutChain: []uint32{0}}},
		Fallbacks: []Fallback{
			{Prefix: "/", Name: "not-found", Kind: FallbackNotFound, Plan: 0, LayoutChain: []uint32{0, 0}},
			{Prefix: "/docs", Name: "docs.not-found", Kind: FallbackNotFound, Plan: 0, LayoutChain: []uint32{0}},
		},
	}
	full := Encode(manifest)
	for cut := len(magic); cut < len(full); cut++ {
		if _, err := Decode(full[:cut]); err == nil {
			t.Fatalf("Decode accepted %d of %d bytes", cut, len(full))
		}
	}
	decoded, err := Decode(full)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, manifest) {
		t.Errorf("round trip changed the manifest:\ngot  %+v\nwant %+v", decoded, manifest)
	}
}

func TestTheDeferredFlagSurvivesTheCodec(t *testing.T) {
	manifest := &Manifest{
		Version: Version,
		Plans: []Plan{{
			Fragments: []Fragment{
				{Name: "Reviews", Deferred: true, TTL: 60},
				{Name: "Similar"},
			},
		}},
	}
	decoded, err := Decode(Encode(manifest))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	fragments := decoded.Plans[0].Fragments
	if len(fragments) != 2 {
		t.Fatalf("fragments = %+v", fragments)
	}
	if !fragments[0].Deferred || fragments[1].Deferred {
		t.Errorf("fragments = %+v, want only the first deferred", fragments)
	}
}

func TestAFragmentReportsWhetherItHoldsAPlaceholder(t *testing.T) {
	if (Fragment{Hold: 4, HoldEnd: 7}).Held() != true {
		t.Error("a fragment with a placeholder range holds one")
	}
	if (Fragment{}).Held() || (Fragment{Hold: 4, HoldEnd: 4}).Held() {
		t.Error("an empty range is no placeholder")
	}
}

func TestATruncatedFragmentRangeIsRejected(t *testing.T) {
	manifest := &Manifest{
		Version: Version,
		Plans: []Plan{{
			Fragments: []Fragment{{Name: "Reviews", Deferred: true, BodyEnd: 3, Hold: 4, HoldEnd: 6, Paths: []uint32{0}}},
			Paths:     [][]string{{"Reviews"}},
		}},
	}
	encoded := Encode(manifest)
	for cut := len(encoded) - 1; cut > len(encoded)-18 && cut > 0; cut-- {
		if _, err := Decode(encoded[:cut]); err == nil {
			t.Fatalf("a manifest cut at %d decoded without an error", cut)
		}
	}
}

func TestIslandUsesSurviveTheCodec(t *testing.T) {
	manifest := &Manifest{
		Version: Version,
		Plans: []Plan{{
			Ops:     []Op{{Kind: OpPreload}},
			Islands: []IslandUse{{Name: "Stars", Strategy: "idle"}, {Name: "Stories", Strategy: "visible"}},
		}},
	}
	decoded, err := Decode(Encode(manifest))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got := decoded.Plans[0].Islands
	if len(got) != 2 || got[0].Name != "Stars" || got[0].Strategy != "idle" || got[1].Strategy != "visible" {
		t.Errorf("islands = %+v", got)
	}
	if decoded.Plans[0].Ops[0].Kind != OpPreload {
		t.Errorf("op = %v, want the preload op kept", decoded.Plans[0].Ops[0].Kind)
	}
}
