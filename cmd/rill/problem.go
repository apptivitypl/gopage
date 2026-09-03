package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

type problem struct {
	summary string
	detail  string
	block   string
	tries   []string
	cause   error
}

func fail(format string, args ...any) *problem {
	return &problem{summary: fmt.Sprintf(format, args...)}
}

func (p *problem) because(format string, args ...any) *problem {
	p.detail = fmt.Sprintf(format, args...)
	return p
}

func (p *problem) showing(block string) *problem {
	p.block = block
	return p
}

func (p *problem) try(commands ...string) *problem {
	p.tries = append(p.tries, commands...)
	return p
}

func (p *problem) Error() string {
	if p.cause == nil {
		return p.summary
	}
	return p.summary + ": " + p.cause.Error()
}

func (p *problem) Unwrap() error {
	return p.cause
}

func (p *printer) failure(err error) {
	known := &problem{}
	if !errors.As(err, &known) {
		known = fail("%s", err.Error())
	}
	_, _ = fmt.Fprintf(p.writer, "\n  %s %s\n", p.paint(red+bold, "✗"), p.paint(bold, known.summary))
	if known.cause != nil {
		for _, line := range wrap(known.cause.Error(), p.room()) {
			_, _ = fmt.Fprintf(p.writer, "    %s\n", p.paint(red, line))
		}
	}
	if known.detail != "" {
		_, _ = fmt.Fprintln(p.writer)
		for _, line := range wrap(known.detail, p.room()) {
			_, _ = fmt.Fprintf(p.writer, "    %s\n", p.paint(dim, line))
		}
	}
	if known.block != "" {
		_, _ = fmt.Fprintf(p.writer, "\n%s\n", known.block)
	}
	if len(known.tries) > 0 {
		_, _ = fmt.Fprintf(p.writer, "\n    %s\n", p.paint(dim, "try"))
		for _, command := range known.tries {
			_, _ = fmt.Fprintf(p.writer, "      %s\n", p.paint(cyan, command))
		}
	}
	_, _ = fmt.Fprintln(p.writer)
}

func (p *printer) room() int {
	width := 80
	if file, ok := p.writer.(*os.File); ok {
		if measured, _, err := term.GetSize(int(file.Fd())); err == nil && measured > 20 {
			width = measured
		}
	}
	if width > 96 {
		width = 96
	}
	return width - 6
}

func wrap(text string, room int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len(line)+1+len(word) > room {
				lines = append(lines, line)
				line = word
				continue
			}
			line += " " + word
		}
		lines = append(lines, line)
	}
	return lines
}
