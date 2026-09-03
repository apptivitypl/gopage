package logs

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	reset = "\x1b[0m"
	dim   = "\x1b[2m"
	red   = "\x1b[31m"
	amber = "\x1b[33m"
	cyan  = "\x1b[36m"
)

type pretty struct {
	writer io.Writer
	level  slog.Level
	color  bool
	group  string
	attrs  []slog.Attr
	mu     *sync.Mutex
}

func (p *pretty) Enabled(_ context.Context, level slog.Level) bool {
	return level >= p.level
}

func (p *pretty) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := p.clone()
	next.attrs = append(next.attrs, attrs...)
	return next
}

func (p *pretty) WithGroup(name string) slog.Handler {
	next := p.clone()
	if name != "" {
		next.group = strings.TrimPrefix(next.group+"."+name, ".")
	}
	return next
}

func (p *pretty) clone() *pretty {
	next := *p
	next.attrs = append([]slog.Attr(nil), p.attrs...)
	if p.mu == nil {
		next.mu = &sync.Mutex{}
	}
	return &next
}

func (p *pretty) Handle(_ context.Context, record slog.Record) error {
	var b strings.Builder
	stamp := record.Time
	if stamp.IsZero() {
		stamp = time.Now()
	}
	p.paint(&b, dim, stamp.Format("15:04:05.000"))
	b.WriteByte(' ')
	p.paint(&b, tint(record.Level), label(record.Level))
	b.WriteByte(' ')
	if record.Message == RequestMessage {
		p.request(&b, record)
	} else {
		b.WriteString(record.Message)
		for _, attr := range p.attrs {
			p.field(&b, attr)
		}
		record.Attrs(func(attr slog.Attr) bool {
			p.field(&b, attr)
			return true
		})
	}
	b.WriteByte('\n')

	if p.mu != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
	}
	_, err := io.WriteString(p.writer, b.String())
	return err
}

func (p *pretty) request(b *strings.Builder, record slog.Record) {
	fields := map[string]string{}
	var rest []slog.Attr
	record.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "took":
			fields[attr.Key] = elapsed(attr.Value)
		case "method", "path", "status", "bytes":
			fields[attr.Key] = attr.Value.Resolve().String()
		case RequestKey:
		default:
			rest = append(rest, attr)
		}
		return true
	})
	b.WriteString(fields["method"] + " " + fields["path"])
	b.WriteString("  ")
	p.paint(b, statusTint(fields["status"]), fields["status"])
	b.WriteString("  ")
	p.paint(b, dim, fields["took"]+"  "+fields["bytes"]+" B")
	for _, attr := range p.attrs {
		p.field(b, attr)
	}
	for _, attr := range rest {
		p.field(b, attr)
	}
}

func elapsed(v slog.Value) string {
	if v.Kind() != slog.KindDuration {
		return v.Resolve().String()
	}
	took := v.Duration()
	switch {
	case took < time.Millisecond:
		return strconv.FormatInt(int64(took/time.Microsecond), 10) + "µs"
	case took < time.Second:
		return strconv.FormatFloat(float64(took)/float64(time.Millisecond), 'f', 1, 64) + "ms"
	default:
		return strconv.FormatFloat(took.Seconds(), 'f', 2, 64) + "s"
	}
}

func statusTint(status string) string {
	switch {
	case strings.HasPrefix(status, "5"):
		return red
	case strings.HasPrefix(status, "4"):
		return amber
	default:
		return dim
	}
}

func (p *pretty) field(b *strings.Builder, attr slog.Attr) {
	if attr.Equal(slog.Attr{}) || attr.Key == RequestKey {
		return
	}
	name := attr.Key
	if p.group != "" {
		name = p.group + "." + name
	}
	b.WriteByte(' ')
	p.paint(b, dim, name+"=")
	b.WriteString(value(attr.Value))
}

func value(v slog.Value) string {
	text := v.Resolve().String()
	if strings.ContainsAny(text, " \t\"") {
		return strconv.Quote(text)
	}
	return text
}

func (p *pretty) paint(b *strings.Builder, code, text string) {
	if !p.color {
		b.WriteString(text)
		return
	}
	b.WriteString(code)
	b.WriteString(text)
	b.WriteString(reset)
}

func label(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO "
	case level < slog.LevelError:
		return "WARN "
	default:
		return "ERROR"
	}
}

func tint(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return dim
	case level < slog.LevelWarn:
		return cyan
	case level < slog.LevelError:
		return amber
	default:
		return red
	}
}
