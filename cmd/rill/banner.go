package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	reset = "\x1b[0m"
	dim   = "\x1b[2m"
	bold  = "\x1b[1m"
	red   = "\x1b[31m"
	amber = "\x1b[33m"
	cyan  = "\x1b[36m"
)

type printer struct {
	writer io.Writer
	color  bool
}

func console(w io.Writer) *printer {
	return &printer{writer: w, color: painted(w)}
}

func painted(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (p *printer) paint(code, text string) string {
	if !p.color {
		return text
	}
	return code + text + reset
}

var taglines = []string{
	"the build took longer than any request will",
	"templates compiled, nothing left to interpret",
	"0 kB of javascript, unless you asked for some",
	"every page is already a plan",
}

type summary struct {
	pages   []string
	api     []string
	islands []string
	quiet   bool
	width   int
}

func (p *printer) banner(local, network string, took time.Duration, about summary) {
	var b strings.Builder
	b.WriteString("\n  " + p.paint(bold, "rill") + "  " + p.paint(dim, "ready in "+round(took)))
	if !about.quiet {
		b.WriteString("   " + p.paint(dim, tagline(time.Now())))
	}
	b.WriteString("\n\n")
	b.WriteString(p.row("local  ", p.paint(cyan, local)))
	if network != "" {
		b.WriteString(p.row("network", p.paint(cyan, network)))
	}
	if !about.quiet {
		b.WriteString(p.counts(about))
	}
	b.WriteString("\n")
	_, _ = io.WriteString(p.writer, b.String())
}

func (p *printer) counts(about summary) string {
	var b strings.Builder
	for _, group := range about.groups() {
		count := strconv.Itoa(len(group.items))
		b.WriteString(p.row(group.label, p.paint(bold, count)+"  "+p.paint(dim, about.fit(group.items, len(count)))))
	}
	return b.String()
}

func (p *printer) row(label, value string) string {
	return "  " + p.paint(dim, "┃") + " " + p.paint(dim, label) + " " + value + "\n"
}

func tagline(at time.Time) string {
	return taglines[at.UnixNano()/int64(time.Millisecond)%int64(len(taglines))]
}

type group struct {
	label string
	items []string
}

func (s summary) groups() []group {
	var groups []group
	for _, candidate := range []group{
		{"pages  ", s.pages},
		{"api    ", s.api},
		{"islands", s.islands},
	} {
		if len(candidate.items) > 0 {
			groups = append(groups, candidate)
		}
	}
	return groups
}

const gutter = 4 + 7 + 1 + 2

func (s summary) fit(items []string, digits int) string {
	text := strings.Join(items, "  ")
	width := s.width
	if width <= 0 {
		width = 80
	}
	if room := width - gutter - digits; len(text) > room && room > 3 {
		text = text[:room-1] + "…"
	}
	return text
}

func (p *printer) event(code, name, message string) {
	stamp := p.paint(dim, time.Now().Format("15:04:05"))
	_, _ = fmt.Fprintf(p.writer, "  %s  %s %s\n", stamp, p.paint(code, name), message)
}

func (p *printer) rebuilt(file string, took time.Duration) {
	p.event(cyan, orDash(file), p.paint(dim, "rebuilt in "+round(took)))
}

func (p *printer) broken(message string) {
	p.event(red, "failed", p.paint(dim, message))
}

func (p *printer) note(message string) {
	p.event(amber, "note", p.paint(dim, message))
}

func orDash(file string) string {
	if file == "" {
		return "project"
	}
	return file
}

func round(took time.Duration) string {
	if took < time.Second {
		return took.Round(time.Millisecond).String()
	}
	return took.Round(10 * time.Millisecond).String()
}

func (p *printer) child() io.Writer {
	return p.tagged("app")
}

func trimTail(line []byte) []byte {
	trimmed := bytes.TrimRight(line, " \t\r")
	if len(bytes.TrimLeft(trimmed, "\x1b[0123456789;m")) == 0 {
		return nil
	}
	return trimmed
}

func (p *printer) tagged(name string) io.Writer {
	return &prefixed{writer: p.writer, mark: p.paint(dim, "  "+pad(name)+" ") + p.paint(dim, "│") + " "}
}

func pad(name string) string {
	for len(name) < 5 {
		name += " "
	}
	return name
}

func (p *printer) step(name, detail string) {
	_, _ = fmt.Fprintf(p.writer, "  %s %s\n", p.paint(cyan, "•"), p.paint(bold, name)+"  "+p.paint(dim, detail))
}

func (p *printer) hint(text string) {
	_, _ = fmt.Fprintf(p.writer, "\n  %s %s\n", p.paint(dim, "next"), text)
}

type prefixed struct {
	writer io.Writer
	mark   string
	rest   []byte
}

func (p *prefixed) Write(chunk []byte) (int, error) {
	held := make([]byte, 0, len(p.rest)+len(chunk))
	held = append(held, p.rest...)
	held = append(held, chunk...)
	p.rest = nil
	for {
		cut := bytes.IndexByte(held, '\n')
		if cut < 0 {
			p.rest = append([]byte(nil), held...)
			return len(chunk), nil
		}
		if line := trimTail(held[:cut]); len(line) > 0 {
			if _, err := io.WriteString(p.writer, p.mark+string(line)+"\n"); err != nil {
				return 0, err
			}
		}
		held = held[cut+1:]
	}
}
