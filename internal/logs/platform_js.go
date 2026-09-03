//go:build js

package logs

import (
	"context"
	"log/slog"
	"strings"
	"syscall/js"
)

func platform() string {
	return FormatConsole
}

func console(opts Options) slog.Handler {
	return &browser{level: opts.Level}
}

type browser struct {
	level  slog.Level
	group  string
	attrs  []slog.Attr
	fields map[string]any
}

func (b *browser) Enabled(_ context.Context, level slog.Level) bool {
	return level >= b.level
}

func (b *browser) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *b
	next.attrs = append(append([]slog.Attr(nil), b.attrs...), attrs...)
	return &next
}

func (b *browser) WithGroup(name string) slog.Handler {
	next := *b
	if name != "" {
		next.group = strings.TrimPrefix(next.group+"."+name, ".")
	}
	return &next
}

func (b *browser) Handle(_ context.Context, record slog.Record) error {
	entry := map[string]any{"level": strings.TrimSpace(label(record.Level)), "message": record.Message}
	for _, attr := range b.attrs {
		b.put(entry, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		b.put(entry, attr)
		return true
	})
	js.Global().Get("console").Call(method(record.Level), js.ValueOf(entry))
	return nil
}

func (b *browser) put(entry map[string]any, attr slog.Attr) {
	name := attr.Key
	if b.group != "" {
		name = b.group + "." + name
	}
	entry[name] = attr.Value.Resolve().String()
}

func method(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "debug"
	case level < slog.LevelWarn:
		return "log"
	case level < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}
