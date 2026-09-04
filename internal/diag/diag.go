package diag

import (
	"fmt"
	"sort"
	"strings"
)

type Span struct {
	Start uint32
	End   uint32
}

func At(offset uint32) Span {
	return Span{Start: offset, End: offset + 1}
}

func (s Span) Len() uint32 {
	if s.End <= s.Start {
		return 0
	}
	return s.End - s.Start
}

type Severity uint8

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

type Diagnostic struct {
	Code     Code
	Severity Severity
	File     string
	Span     Span
	Message  string
	Help     string
}

func New(code Code, file string, span Span, message string) Diagnostic {
	return Diagnostic{Code: code, Severity: Error, File: file, Span: span, Message: message}
}

func Warn(code Code, file string, span Span, message string) Diagnostic {
	return Diagnostic{Code: code, Severity: Warning, File: file, Span: span, Message: message}
}

func (d Diagnostic) WithHelp(help string) Diagnostic {
	d.Help = help
	return d
}

type Bag struct {
	items []Diagnostic
}

func (b *Bag) Add(d Diagnostic) {
	for _, held := range b.items {
		if held == d {
			return
		}
	}
	b.items = append(b.items, d)
}

func (b *Bag) Items() []Diagnostic {
	return b.items
}

func (b *Bag) HasErrors() bool {
	for _, item := range b.items {
		if item.Severity == Error {
			return true
		}
	}
	return false
}

func (b *Bag) Len() int {
	return len(b.items)
}

func (b *Bag) Sorted() []Diagnostic {
	sorted := make([]Diagnostic, len(b.items))
	copy(sorted, b.items)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Span.Start < sorted[j].Span.Start
	})
	return sorted
}

type Position struct {
	Line   int
	Column int
}

func PositionOf(source string, offset uint32) Position {
	limit := min(int(offset), len(source))
	line, lineStart := 1, 0
	for i := range limit {
		if source[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return Position{Line: line, Column: limit - lineStart + 1}
}

func lineText(source string, line int) string {
	current := 1
	start := 0
	for i := range len(source) {
		if current == line && source[i] == '\n' {
			return source[start:i]
		}
		if source[i] == '\n' {
			current++
			start = i + 1
		}
	}
	if current == line {
		return source[start:]
	}
	return ""
}

func Render(d Diagnostic, source string) string {
	pos := PositionOf(source, d.Span.Start)
	text := lineText(source, pos.Line)
	gutter := len(fmt.Sprint(pos.Line))

	var b strings.Builder
	fmt.Fprintf(&b, "%s[GOPAGE-%s]: %s\n", d.Severity, d.Code, d.Message)
	fmt.Fprintf(&b, "%s--> %s:%d:%d\n", strings.Repeat(" ", gutter), d.File, pos.Line, pos.Column)
	fmt.Fprintf(&b, "%s |\n", strings.Repeat(" ", gutter))
	fmt.Fprintf(&b, "%d | %s\n", pos.Line, text)
	fmt.Fprintf(&b, "%s | %s%s\n", strings.Repeat(" ", gutter),
		strings.Repeat(" ", pos.Column-1), strings.Repeat("^", caretWidth(d.Span, text, pos)))
	if d.Help != "" {
		fmt.Fprintf(&b, "%s |\n", strings.Repeat(" ", gutter))
		fmt.Fprintf(&b, "%s = help: %s\n", strings.Repeat(" ", gutter), d.Help)
	}
	return b.String()
}

func caretWidth(span Span, text string, pos Position) int {
	width := int(span.Len())
	if remaining := len(text) - pos.Column + 1; width > remaining {
		width = remaining
	}
	if width < 1 {
		width = 1
	}
	return width
}
