package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
)

const page = `---
type Props struct {
	Heading string
	Count   int
}

func (p Props) Loud() string {
	return p.Heading
}
---
<h1>{{ Heading }}</h1>
{% if Count %}<p>{{ Count }}</p>{% endif %}
`

func labels(items []CompletionItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Label)
	}
	return out
}

func at(text, marker string) Position {
	offset := strings.Index(text, marker) + len(marker)
	line := strings.Count(text[:offset], "\n")
	start := strings.LastIndex(text[:offset], "\n") + 1
	return Position{Line: line, Character: len([]rune(text[start:offset]))}
}

func TestACleanPageReportsNothing(t *testing.T) {
	if report := Analyse("app/page.gopage", page).Report(); len(report) != 0 {
		t.Errorf("report = %+v", report)
	}
}

func TestAnUnknownFieldIsReportedWithARange(t *testing.T) {
	broken := strings.Replace(page, "{{ Heading }}", "{{ Headline }}", 1)
	report := Analyse("app/page.gopage", broken).Report()
	if len(report) != 1 {
		t.Fatalf("report = %+v", report)
	}
	item := report[0]
	if item.Code != "C305" || item.Severity != SeverityError || item.Source != Source {
		t.Errorf("item = %+v", item)
	}
	if item.Range.Start.Line != 10 || item.Range.Start.Character == item.Range.End.Character {
		t.Errorf("range = %+v", item.Range)
	}
	if !strings.Contains(item.Message, "did you mean") {
		t.Errorf("message = %q, want the help folded in", item.Message)
	}
}

func TestFieldsAreOffered(t *testing.T) {
	analysis := Analyse("app/page.gopage", page)
	items := analysis.Completions(at(page, "<h1>{{ "))
	got := labels(items)
	for _, want := range []string{"Heading", "Count", "Loud"} {
		if !slices.Contains(got, want) {
			t.Errorf("completions = %v, want %q", got, want)
		}
	}
	for _, item := range items {
		if item.Label == "Loud" && !strings.Contains(item.Detail, "computed") {
			t.Errorf("computed field = %+v", item)
		}
	}
}

func TestAPrefixNarrowsTheList(t *testing.T) {
	source := "---\ntype Props struct {\n\tHeading string\n\tHeadline string\n\tCount int\n}\n---\n{{ Head"
	items := Analyse("app/page.gopage", source).Completions(at(source, "{{ Head"))
	got := labels(items)
	if !slices.Equal(got, []string{"Heading", "Headline"}) {
		t.Errorf("completions = %v", got)
	}
}

func TestCompletionWorksInAFileCutInHalf(t *testing.T) {
	source := "---\ntype Props struct {\n\tHeading string\n}\n---\n<h1>{{ Head"
	items := Analyse("app/page.gopage", source).Completions(at(source, "{{ Head"))
	if !slices.Contains(labels(items), "Heading") {
		t.Errorf("completions = %v, want the field even though the expression is unfinished", labels(items))
	}
}

func TestDirectivesAreOfferedInsideABlock(t *testing.T) {
	source := "{% fra"
	items := Analyse("app/page.gopage", source).Completions(at(source, "{% fra"))
	if !slices.Contains(labels(items), "fragment") {
		t.Errorf("completions = %v", labels(items))
	}
}

func TestFiltersAreOfferedAfterAPipe(t *testing.T) {
	source := "---\ntype Props struct {\n\tHeading string\n}\n---\n{{ Heading | up"
	items := Analyse("app/page.gopage", source).Completions(at(source, "| up"))
	if !slices.Contains(labels(items), "upper") {
		t.Errorf("completions = %v", labels(items))
	}
}

func TestBuiltinComponentsAreOffered(t *testing.T) {
	source := "<Fi"
	items := Analyse("app/page.gopage", source).Completions(at(source, "<Fi"))
	if !slices.Contains(labels(items), "Field") {
		t.Errorf("completions = %v", labels(items))
	}
}

func TestAPageWithoutAFrontmatterStillCompletes(t *testing.T) {
	source := "{{ "
	if items := Analyse("app/page.gopage", source).Completions(at(source, "{{ ")); len(items) == 0 {
		t.Error("the built-ins are always available")
	}
}

func TestHoverExplainsAField(t *testing.T) {
	hover, ok := Analyse("app/page.gopage", page).Hover(at(page, "<h1>{{ Head"))
	if !ok || !strings.Contains(hover.Contents.Value, "Heading") {
		t.Errorf("hover = %+v, ok = %v", hover, ok)
	}
}

func TestHoverExplainsAFilterAndADirective(t *testing.T) {
	filter := "---\ntype Props struct{}\n---\n{{ 'x' | upper }}"
	if hover, ok := Analyse("app/page.gopage", filter).Hover(at(filter, "| upp")); !ok ||
		!strings.Contains(hover.Contents.Value, "filter") {
		t.Errorf("hover = %+v, ok = %v", hover, ok)
	}
	directive := "{% fragment \"a\" %}x{% endfragment %}"
	if hover, ok := Analyse("app/page.gopage", directive).Hover(at(directive, "{% frag")); !ok ||
		!strings.Contains(hover.Contents.Value, "directive") {
		t.Errorf("hover = %+v, ok = %v", hover, ok)
	}
}

func TestHoverSaysNothingAboutPlainText(t *testing.T) {
	if _, ok := Analyse("app/page.gopage", page).Hover(Position{Line: 0, Character: 0}); ok {
		t.Error("the frontmatter fence is not a symbol")
	}
	if _, ok := Analyse("app/page.gopage", "<p>hello</p>").Hover(Position{Line: 0, Character: 4}); ok {
		t.Error("plain words are not symbols")
	}
}

func TestPositionsSurviveOutOfRange(t *testing.T) {
	analysis := Analyse("app/page.gopage", "x")
	if items := analysis.Completions(Position{Line: 99, Character: 99}); len(items) > 0 {
		t.Errorf("completions = %v, want nothing past the end", labels(items))
	}
	if _, ok := analysis.Hover(Position{Line: 99, Character: 99}); ok {
		t.Error("an impossible position has nothing to explain")
	}
}

func frame(t *testing.T, method string, id int, params any) string {
	t.Helper()
	body, err := json.Marshal(Request{JSONRPC: "2.0", Method: method, Params: raw(t, params), ID: rawID(t, id)})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func raw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	if value == nil {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func rawID(t *testing.T, id int) json.RawMessage {
	t.Helper()
	if id == 0 {
		return nil
	}
	return json.RawMessage(fmt.Sprintf("%d", id))
}

func run(t *testing.T, messages ...string) string {
	t.Helper()
	var out bytes.Buffer
	server := NewServer(strings.NewReader(strings.Join(messages, "")), &out)
	if err := server.Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	return out.String()
}

func TestTheServerAnswersTheHandshake(t *testing.T) {
	out := run(t,
		frame(t, MethodInitialize, 1, map[string]any{}),
		frame(t, MethodShutdown, 2, nil),
		frame(t, MethodExit, 0, nil),
	)
	if !strings.Contains(out, `"hoverProvider":true`) {
		t.Errorf("out = %q", out)
	}
	if strings.Count(out, "Content-Length:") != 2 {
		t.Errorf("out = %q, want one reply per request", out)
	}
}

func TestOpeningADocumentPublishesDiagnostics(t *testing.T) {
	broken := strings.Replace(page, "{{ Heading }}", "{{ Headline }}", 1)
	out := run(t,
		frame(t, MethodDidOpen, 0, DidOpenParams{TextDocument: TextDocument{URI: "file:///app/page.gopage", Text: broken}}),
		frame(t, MethodExit, 0, nil),
	)
	if !strings.Contains(out, MethodDiagnostics) || !strings.Contains(out, "C305") {
		t.Errorf("out = %q", out)
	}
}

func TestEditingRepublishes(t *testing.T) {
	out := run(t,
		frame(t, MethodDidOpen, 0, DidOpenParams{TextDocument: TextDocument{URI: "file:///a.gopage", Text: page}}),
		frame(t, MethodDidChange, 0, DidChangeParams{
			TextDocument:   TextDocument{URI: "file:///a.gopage"},
			ContentChanges: []ContentChange{{Text: strings.Replace(page, "{{ Heading }}", "{{ Missing }}", 1)}},
		}),
		frame(t, MethodExit, 0, nil),
	)
	if strings.Count(out, MethodDiagnostics) != 2 {
		t.Errorf("out = %q, want a report per edit", out)
	}
	if !strings.Contains(out, "C305") {
		t.Errorf("out = %q", out)
	}
}

func TestCompletionAndHoverOverTheWire(t *testing.T) {
	out := run(t,
		frame(t, MethodDidOpen, 0, DidOpenParams{TextDocument: TextDocument{URI: "file:///a.gopage", Text: page}}),
		frame(t, MethodCompletion, 3, DocumentParams{
			TextDocument: TextDocument{URI: "file:///a.gopage"},
			Position:     at(page, "<h1>{{ "),
		}),
		frame(t, MethodHover, 4, DocumentParams{
			TextDocument: TextDocument{URI: "file:///a.gopage"},
			Position:     at(page, "<h1>{{ Head"),
		}),
		frame(t, MethodExit, 0, nil),
	)
	if !strings.Contains(out, `"label":"Heading"`) {
		t.Errorf("out = %q", out)
	}
	if !strings.Contains(out, "markdown") {
		t.Errorf("out = %q", out)
	}
}

func TestAskingAboutAnUnknownDocument(t *testing.T) {
	out := run(t,
		frame(t, MethodCompletion, 5, DocumentParams{TextDocument: TextDocument{URI: "file:///gone.gopage"}}),
		frame(t, MethodHover, 6, DocumentParams{TextDocument: TextDocument{URI: "file:///gone.gopage"}}),
		frame(t, MethodExit, 0, nil),
	)
	if strings.Count(out, "Content-Length:") != 2 {
		t.Errorf("out = %q, want an answer to both", out)
	}
}

func TestAClosedDocumentIsForgotten(t *testing.T) {
	out := run(t,
		frame(t, MethodDidOpen, 0, DidOpenParams{TextDocument: TextDocument{URI: "file:///a.gopage", Text: page}}),
		frame(t, MethodDidClose, 0, DidOpenParams{TextDocument: TextDocument{URI: "file:///a.gopage"}}),
		frame(t, MethodHover, 7, DocumentParams{TextDocument: TextDocument{URI: "file:///a.gopage"}}),
		frame(t, MethodExit, 0, nil),
	)
	if strings.Contains(out, "markdown") {
		t.Errorf("out = %q, want nothing known about a closed file", out)
	}
}

func TestUnknownMethodsAreAnsweredOnlyWhenTheyAsk(t *testing.T) {
	out := run(t,
		frame(t, "textDocument/formatting", 8, map[string]any{}),
		frame(t, MethodInitialized, 0, map[string]any{}),
		frame(t, MethodExit, 0, nil),
	)
	if strings.Count(out, "Content-Length:") != 1 {
		t.Errorf("out = %q, want a reply only for the request", out)
	}
}

func TestMalformedFramesAreReported(t *testing.T) {
	cases := map[string]string{
		"no length":   "\r\n\r\n{}",
		"bad length":  "Content-Length: wide\r\n\r\n{}",
		"short body":  "Content-Length: 99\r\n\r\n{}",
		"broken json": "Content-Length: 2\r\n\r\n{{",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := NewServer(strings.NewReader(text), &out).Serve(); err == nil {
				t.Error("a malformed frame must be reported")
			}
		})
	}
}

func TestMalformedParamsAreIgnored(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"textDocument/didOpen","params":7}`
	bad := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	out := run(t, bad, frame(t, MethodExit, 0, nil))
	if out != "" {
		t.Errorf("out = %q, want silence", out)
	}
}

func TestAnEmptyChangeListChangesNothing(t *testing.T) {
	out := run(t,
		frame(t, MethodDidOpen, 0, DidOpenParams{TextDocument: TextDocument{URI: "file:///a.gopage", Text: page}}),
		frame(t, MethodDidChange, 0, DidChangeParams{TextDocument: TextDocument{URI: "file:///a.gopage"}}),
		frame(t, MethodExit, 0, nil),
	)
	if strings.Count(out, MethodDiagnostics) != 1 {
		t.Errorf("out = %q", out)
	}
}

func TestADiagnosticWithoutHelpKeepsItsMessage(t *testing.T) {
	if got := message(diag.New(diag.C001, "a.gopage", diag.Span{}, "plain")); got != "plain" {
		t.Errorf("message = %q", got)
	}
	if severity(diag.Warning) != SeverityWarning || severity(diag.Error) != SeverityError {
		t.Error("severities must map straight across")
	}
}

func TestOffsetsPastTheTextAreClamped(t *testing.T) {
	if got := offsetPosition("ab", 99); got.Line != 0 || got.Character != 2 {
		t.Errorf("position = %+v", got)
	}
	if got := wordAt("ab", 99); got != "ab" {
		t.Errorf("word = %q", got)
	}
}

func TestAFrontmatterWithoutPropsOffersNoFields(t *testing.T) {
	source := "---\ntype Other struct {\n\tName string\n}\n---\n{{ "
	items := Analyse("app/page.gopage", source).Completions(at(source, "{{ "))
	for _, item := range items {
		if item.Kind == CompletionKindField {
			t.Errorf("completions = %v, want no fields without a Props type", labels(items))
		}
	}
}

func TestTheServerStopsAtTheEndOfInput(t *testing.T) {
	var out bytes.Buffer
	server := NewServer(strings.NewReader(frame(t, MethodInitialize, 1, map[string]any{})), &out)
	if err := server.Serve(); err != nil {
		t.Errorf("Serve: %v", err)
	}
	if !strings.Contains(out.String(), "capabilities") {
		t.Errorf("out = %q", out.String())
	}
}

func TestMalformedParamsOnEveryNotification(t *testing.T) {
	bodies := []string{
		`{"jsonrpc":"2.0","method":"textDocument/didChange","params":7}`,
		`{"jsonrpc":"2.0","method":"textDocument/didClose","params":7}`,
		`{"jsonrpc":"2.0","id":9,"method":"textDocument/completion","params":7}`,
	}
	var messages []string
	for _, body := range bodies {
		messages = append(messages, fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
	}
	messages = append(messages, frame(t, MethodExit, 0, nil))
	out := run(t, messages...)
	if strings.Count(out, "Content-Length:") != 1 {
		t.Errorf("out = %q, want only the completion answered", out)
	}
}

func TestHoverOverPlainTextAnswersNothing(t *testing.T) {
	out := run(t,
		frame(t, MethodDidOpen, 0, DidOpenParams{TextDocument: TextDocument{URI: "file:///a.gopage", Text: "<p>hello</p>"}}),
		frame(t, MethodHover, 4, DocumentParams{
			TextDocument: TextDocument{URI: "file:///a.gopage"},
			Position:     Position{Line: 0, Character: 4},
		}),
		frame(t, MethodExit, 0, nil),
	)
	if strings.Contains(out, "markdown") {
		t.Errorf("out = %q", out)
	}
	if !strings.Contains(out, `"id":4`) {
		t.Errorf("out = %q, want the request answered", out)
	}
}

func TestAnEditorThatDisappearsMidHeaderIsNotAnError(t *testing.T) {
	var out bytes.Buffer
	if err := NewServer(strings.NewReader("Content-Length: 12"), &out).Serve(); err != nil {
		t.Errorf("Serve: %v, want a clean shutdown when the editor goes away", err)
	}
}
