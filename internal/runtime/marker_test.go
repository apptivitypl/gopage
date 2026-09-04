package runtime

import (
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/ir"
)

func nestedChain() []*ir.Plan {
	return []*ir.Plan{
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 6}, {Kind: ir.OpOutlet}, {Kind: ir.OpStatic, A: 6, B: 7}},
			Blob: []byte("<main></main>"), Capacity: 64},
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 7}, {Kind: ir.OpOutlet}, {Kind: ir.OpStatic, A: 7, B: 8}},
			Blob: []byte("<aside></aside>"), Capacity: 64},
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 4}}, Blob: []byte("page"), Capacity: 16},
	}
}

func renderMarked(t *testing.T, markers bool) string {
	t.Helper()
	chain := nestedChain()
	out := Acquire(Capacity(chain))
	defer Release(out)
	if err := RenderOptions(chain, Empty{}, out, Options{Markers: markers}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestMarkersWrapEveryOutlet(t *testing.T) {
	html := renderMarked(t, true)
	for _, want := range []string{
		MarkerOpen + "0-->", MarkerClose + "0-->",
		MarkerOpen + "1-->", MarkerClose + "1-->",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html = %q, want %q", html, want)
		}
	}
	if strings.Index(html, MarkerOpen+"0-->") > strings.Index(html, MarkerOpen+"1-->") {
		t.Errorf("html = %q, want the outer marker first", html)
	}
}

func TestMarkersAreOffByDefault(t *testing.T) {
	if html := renderMarked(t, false); strings.Contains(html, MarkerOpen) {
		t.Errorf("html = %q, want no markers", html)
	}
	if html := renderMarked(t, false); html != "<main><aside>page</aside></main>" {
		t.Errorf("html = %q", html)
	}
}

func TestMarkersStopAtTheTable(t *testing.T) {
	for _, level := range []int{-1, maxMarker, maxMarker + 5} {
		if openMarker(level) != nil || closeMarker(level) != nil {
			t.Errorf("level %d produced a marker", level)
		}
	}
	if openMarker(0) == nil || closeMarker(maxMarker-1) == nil {
		t.Error("levels inside the table must have markers")
	}
}

func TestADeepChainStillRenders(t *testing.T) {
	chain := make([]*ir.Plan, 0, maxMarker+2)
	for range maxMarker + 1 {
		chain = append(chain, &ir.Plan{
			Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 1}, {Kind: ir.OpOutlet}},
			Blob:     []byte("x"),
			Capacity: 8,
		})
	}
	out := Acquire(Capacity(chain))
	defer Release(out)
	if err := RenderOptions(chain, Empty{}, out, Options{Markers: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Count(out.String(), "x") != maxMarker+1 {
		t.Errorf("html = %q", out.String())
	}
}
