package lsp

import (
	"sort"
	"strings"

	"github.com/apptivitypl/rill/internal/compile"
	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/runtime"
	"github.com/apptivitypl/rill/internal/schema"
	"github.com/apptivitypl/rill/internal/syntax"
)

const Source = "rill"

type Analysis struct {
	Document    *syntax.Document
	Schema      *schema.Schema
	Diagnostics []diag.Diagnostic
	Text        string
}

func Analyse(name, text string) Analysis {
	var bag diag.Bag
	document := syntax.Parse(name, text, &bag)
	analysis := Analysis{Document: document, Text: text}
	if document.Frontmatter != nil {
		analysis.Schema = schema.Parse([]schema.Source{{File: name, Code: document.Frontmatter.Code}}, &bag)
	}
	compile.Check(document, name, analysis.Schema, &bag)
	analysis.Diagnostics = bag.Sorted()
	return analysis
}

func (a Analysis) Report() []Diagnostic {
	out := make([]Diagnostic, 0, len(a.Diagnostics))
	for _, item := range a.Diagnostics {
		out = append(out, Diagnostic{
			Range:    spanRange(a.Text, item.Span),
			Severity: severity(item.Severity),
			Code:     string(item.Code),
			Source:   Source,
			Message:  message(item),
		})
	}
	return out
}

func message(item diag.Diagnostic) string {
	if item.Help == "" {
		return item.Message
	}
	return item.Message + "\n\n" + item.Help
}

func severity(value diag.Severity) int {
	if value == diag.Warning {
		return SeverityWarning
	}
	return SeverityError
}

func spanRange(text string, span diag.Span) Range {
	return Range{Start: offsetPosition(text, span.Start), End: offsetPosition(text, span.End)}
}

func offsetPosition(text string, offset uint32) Position {
	limit := int(offset)
	if limit > len(text) {
		limit = len(text)
	}
	line := strings.Count(text[:limit], "\n")
	start := strings.LastIndex(text[:limit], "\n") + 1
	return Position{Line: line, Character: len([]rune(text[start:limit]))}
}

func (a Analysis) Completions(position Position) []CompletionItem {
	offset := positionOffset(a.Text, position)
	prefix := wordBefore(a.Text, offset)
	if inDirective(a.Text, offset) {
		return filter(directives(), prefix)
	}
	if inFilter(a.Text, offset) {
		return filter(filters(), prefix)
	}
	return filter(append(a.fields(), builtins()...), prefix)
}

func (a Analysis) fields() []CompletionItem {
	if a.Schema == nil {
		return nil
	}
	props, ok := a.Schema.Props()
	if !ok {
		return nil
	}
	items := make([]CompletionItem, 0, len(props.Fields))
	for _, field := range props.Fields {
		detail := field.Type.Kind.String()
		if field.Computed {
			detail += " (computed)"
		}
		items = append(items, CompletionItem{Label: field.Name, Kind: CompletionKindField, Detail: detail})
	}
	return items
}

func directives() []CompletionItem {
	items := make([]CompletionItem, 0, len(syntax.Directives()))
	for _, name := range syntax.Directives() {
		items = append(items, CompletionItem{Label: name, Kind: CompletionKindModule, Detail: "directive"})
	}
	return items
}

func filters() []CompletionItem {
	names := runtime.FilterNames()
	items := make([]CompletionItem, 0, len(names))
	for _, name := range names {
		items = append(items, CompletionItem{Label: name, Kind: CompletionKindText, Detail: "filter"})
	}
	return items
}

func builtins() []CompletionItem {
	items := make([]CompletionItem, 0, len(compile.BuiltinNames()))
	for _, name := range compile.BuiltinNames() {
		items = append(items, CompletionItem{Label: name, Kind: CompletionKindModule, Detail: "built-in component"})
	}
	return items
}

func filter(items []CompletionItem, prefix string) []CompletionItem {
	if prefix == "" {
		sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
		return items
	}
	var kept []CompletionItem
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), strings.ToLower(prefix)) {
			kept = append(kept, item)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Label < kept[j].Label })
	return kept
}

func positionOffset(text string, position Position) int {
	line, offset := 0, 0
	for offset < len(text) && line < position.Line {
		if text[offset] == '\n' {
			line++
		}
		offset++
	}
	for range position.Character {
		if offset >= len(text) || text[offset] == '\n' {
			break
		}
		offset++
	}
	return offset
}

func wordBefore(text string, offset int) string {
	start := offset
	for start > 0 && isWord(text[start-1]) {
		start--
	}
	return text[start:offset]
}

func isWord(c byte) bool {
	return c == '_' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func inDirective(text string, offset int) bool {
	open := strings.LastIndex(text[:offset], "{%")
	return open >= 0 && !strings.Contains(text[open:offset], "%}")
}

func inFilter(text string, offset int) bool {
	pipe := strings.LastIndex(text[:offset], "|")
	if pipe < 0 {
		return false
	}
	rest := text[pipe:offset]
	return !strings.ContainsAny(rest, "}%<>") && strings.Count(text[:offset], "{{") > strings.Count(text[:offset], "}}")
}

func (a Analysis) Hover(position Position) (Hover, bool) {
	offset := positionOffset(a.Text, position)
	word := wordAt(a.Text, offset)
	if word == "" {
		return Hover{}, false
	}
	if detail, ok := a.describe(word); ok {
		return Hover{Contents: MarkupContent{Kind: "markdown", Value: detail}}, true
	}
	return Hover{}, false
}

func (a Analysis) describe(word string) (string, bool) {
	if a.Schema != nil {
		if typ, ok := a.Schema.Resolve(schema.PropsName, strings.Split(word, ".")); ok {
			return "`" + word + "` — " + typ.Kind.String(), true
		}
	}
	for _, item := range filters() {
		if item.Label == word {
			return "`" + word + "` — filter", true
		}
	}
	for _, name := range syntax.Directives() {
		if name == word {
			return "`{% " + word + " %}` — directive", true
		}
	}
	return "", false
}

func wordAt(text string, offset int) string {
	if offset > len(text) {
		offset = len(text)
	}
	start := offset
	for start > 0 && isWord(text[start-1]) {
		start--
	}
	end := offset
	for end < len(text) && isWord(text[end]) {
		end++
	}
	return strings.Trim(text[start:end], ".")
}
