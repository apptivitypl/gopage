package logs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

const (
	FormatVar = "RILL_LOG"
	LevelVar  = "RILL_LOG_LEVEL"
	AccessVar = "RILL_ACCESS_LOG"
	ColorVar  = "NO_COLOR"
	DevVar    = "RILL_DEV"
)

const (
	FormatPretty  = "pretty"
	FormatJSON    = "json"
	FormatGCP     = "gcp"
	FormatConsole = "console"
)

var formats = []string{FormatPretty, FormatJSON, FormatGCP, FormatConsole}

type Options struct {
	Writer io.Writer
	Format string
	Level  slog.Level
	Color  bool
}

func New() *slog.Logger {
	return slog.New(Handler(Resolve(os.Getenv(FormatVar), os.Getenv(LevelVar))))
}

func Resolve(format, level string) Options {
	opts := Options{Writer: os.Stderr, Format: pick(format), Level: LevelOf(level)}
	opts.Color = opts.Format == FormatPretty && colored(opts.Writer)
	return opts
}

func Handler(opts Options) slog.Handler {
	if opts.Writer == nil {
		opts.Writer = os.Stderr
	}
	switch opts.Format {
	case FormatGCP:
		return google(opts)
	case FormatConsole:
		return console(opts)
	case FormatJSON:
		return slog.NewJSONHandler(opts.Writer, &slog.HandlerOptions{Level: opts.Level})
	default:
		return &pretty{writer: opts.Writer, level: opts.Level, color: opts.Color}
	}
}

func pick(format string) string {
	for _, known := range formats {
		if format == known {
			return known
		}
	}
	if hosted := platform(); hosted != "" {
		return hosted
	}
	if developing() || terminal(os.Stderr) {
		return FormatPretty
	}
	return FormatJSON
}

func developing() bool {
	return os.Getenv(DevVar) != ""
}

func LevelOf(text string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func Access() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(AccessVar))) {
	case "on", "1", "true":
		return true
	case "off", "0", "false":
		return false
	default:
		return platform() == ""
	}
}

func colored(w io.Writer) bool {
	if os.Getenv(ColorVar) != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return developing() || terminal(w)
}

func terminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

type key struct{}

func With(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, logger)
}

func From(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(key{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
