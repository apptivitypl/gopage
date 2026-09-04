package diag

import (
	"strings"
	"testing"
)

func TestSpanLength(t *testing.T) {
	cases := []struct {
		span Span
		want uint32
	}{
		{Span{Start: 2, End: 7}, 5},
		{Span{Start: 7, End: 7}, 0},
		{Span{Start: 9, End: 2}, 0},
	}
	for _, c := range cases {
		if got := c.span.Len(); got != c.want {
			t.Errorf("Span%+v.Len() = %d, want %d", c.span, got, c.want)
		}
	}
}

func TestAtCoversASingleByte(t *testing.T) {
	if got := At(4); got != (Span{Start: 4, End: 5}) {
		t.Errorf("At(4) = %+v", got)
	}
}

func TestSeverityNames(t *testing.T) {
	if Error.String() != "error" || Warning.String() != "warning" {
		t.Errorf("severity names are wrong: %q, %q", Error, Warning)
	}
}

func TestCodeTitles(t *testing.T) {
	if C001.Title() == "" || !C001.Known() {
		t.Error("C001 must have a title")
	}
	if Code("C999").Known() {
		t.Error("an unregistered code must not report itself as known")
	}
	if got := Code("C999").Title(); got != "unknown diagnostic" {
		t.Errorf("Title = %q", got)
	}
}

func TestCodesListsTheRegistry(t *testing.T) {
	if len(Codes()) != len(titles) {
		t.Errorf("Codes() returned %d entries, want %d", len(Codes()), len(titles))
	}
}

func TestBagCollectsAndReportsErrors(t *testing.T) {
	var bag Bag
	if bag.HasErrors() || bag.Len() != 0 {
		t.Fatal("a fresh bag must be empty")
	}
	bag.Add(Diagnostic{Code: C001, Severity: Warning})
	if bag.HasErrors() {
		t.Error("warnings alone must not count as errors")
	}
	bag.Add(New(C002, "a.gopage", Span{}, "boom"))
	if !bag.HasErrors() || bag.Len() != 2 {
		t.Errorf("bag = %d items, errors = %v", bag.Len(), bag.HasErrors())
	}
	if len(bag.Items()) != 2 {
		t.Errorf("Items() = %d", len(bag.Items()))
	}
}

func TestBagSortsByFileThenOffset(t *testing.T) {
	var bag Bag
	bag.Add(New(C001, "b.gopage", Span{Start: 1}, ""))
	bag.Add(New(C001, "a.gopage", Span{Start: 9}, ""))
	bag.Add(New(C001, "a.gopage", Span{Start: 2}, ""))

	sorted := bag.Sorted()
	if sorted[0].File != "a.gopage" || sorted[0].Span.Start != 2 {
		t.Errorf("first = %s:%d", sorted[0].File, sorted[0].Span.Start)
	}
	if sorted[2].File != "b.gopage" {
		t.Errorf("last = %s", sorted[2].File)
	}
	if bag.Items()[0].File != "b.gopage" {
		t.Error("Sorted must not reorder the original slice")
	}
}

func TestWithHelpAttachesASuggestion(t *testing.T) {
	d := New(C001, "a.gopage", Span{}, "boom").WithHelp("close the fence")
	if d.Help != "close the fence" {
		t.Errorf("Help = %q", d.Help)
	}
}

func TestPositionOfCountsLinesAndColumns(t *testing.T) {
	source := "one\ntwo\nthree"
	cases := []struct {
		offset uint32
		want   Position
	}{
		{0, Position{1, 1}},
		{4, Position{2, 1}},
		{6, Position{2, 3}},
		{8, Position{3, 1}},
	}
	for _, c := range cases {
		if got := PositionOf(source, c.offset); got != c.want {
			t.Errorf("PositionOf(%d) = %+v, want %+v", c.offset, got, c.want)
		}
	}
}

func TestPositionOfClampsPastTheEnd(t *testing.T) {
	if got := PositionOf("ab", 99); got != (Position{1, 3}) {
		t.Errorf("PositionOf past the end = %+v", got)
	}
}

func TestLineTextReadsTheRequestedLine(t *testing.T) {
	source := "one\ntwo\nthree"
	if got := lineText(source, 2); got != "two" {
		t.Errorf("lineText(2) = %q", got)
	}
	if got := lineText(source, 3); got != "three" {
		t.Errorf("lineText(3) = %q", got)
	}
	if got := lineText(source, 9); got != "" {
		t.Errorf("lineText past the end = %q", got)
	}
}

func TestRenderShowsCodeLocationAndCaret(t *testing.T) {
	source := "<h1>{{ title }}\n"
	d := New(C002, "app/page.gopage", Span{Start: 4, End: 6}, "unterminated interpolation").
		WithHelp("close it with }}")

	out := Render(d, source)
	for _, want := range []string{
		"error[GOPAGE-C002]: unterminated interpolation",
		"--> app/page.gopage:1:5",
		"1 | <h1>{{ title }}",
		"^^",
		"= help: close it with }}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render is missing %q:\n%s", want, out)
		}
	}
}

func TestRenderClampsTheCaretToTheLine(t *testing.T) {
	out := Render(New(C001, "a.gopage", Span{Start: 0, End: 100}, "boom"), "ab\ncd")
	if !strings.Contains(out, "^^\n") {
		t.Errorf("caret must stop at the end of the line:\n%s", out)
	}
}

func TestRenderKeepsACaretForEmptySpans(t *testing.T) {
	out := Render(New(C001, "a.gopage", Span{}, "boom"), "")
	if !strings.Contains(out, "^") {
		t.Errorf("render must always show a caret:\n%s", out)
	}
}

func TestRenderWithoutHelpHasNoHelpLine(t *testing.T) {
	out := Render(New(C001, "a.gopage", Span{Start: 0, End: 1}, "boom"), "ab")
	if strings.Contains(out, "help:") {
		t.Errorf("render must not invent a help line:\n%s", out)
	}
}

func TestTheBagKeepsOneCopyOfEachDiagnostic(t *testing.T) {
	var bag Bag
	item := New(C001, "app/page.gopage", Span{Start: 1, End: 2}, "same")
	bag.Add(item)
	bag.Add(item)
	bag.Add(New(C001, "app/page.gopage", Span{Start: 3, End: 4}, "same"))
	if len(bag.Items()) != 2 {
		t.Errorf("items = %+v, want the exact repeat dropped", bag.Items())
	}
}

func TestWarningsCarryTheirSeverity(t *testing.T) {
	item := Warn(W703, "app/page.gopage", Span{}, "careful")
	if item.Severity != Warning || item.Code != W703 {
		t.Errorf("item = %+v", item)
	}
	var bag Bag
	bag.Add(item)
	if bag.HasErrors() {
		t.Error("a warning is not an error")
	}
}
