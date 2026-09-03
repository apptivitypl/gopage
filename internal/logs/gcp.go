package logs

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	TraceKey       = "logging.googleapis.com/trace"
	SpanKey        = "logging.googleapis.com/spanId"
	SampledKey     = "logging.googleapis.com/trace_sampled"
	RequestKey     = "httpRequest"
	RequestMessage = "request"

	SeverityKey = "severity"
	MessageKey  = "message"

	CloudTraceHeader = "X-Cloud-Trace-Context"
	TraceParent      = "traceparent"
	RayHeader        = "CF-Ray"
	ProjectVar       = "GOOGLE_CLOUD_PROJECT"
	ServiceVar       = "K_SERVICE"
)

func google(opts Options) slog.Handler {
	return slog.NewJSONHandler(opts.Writer, &slog.HandlerOptions{
		Level:       opts.Level,
		ReplaceAttr: rename,
	})
}

func rename(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return attr
	}
	switch attr.Key {
	case slog.MessageKey:
		return slog.Attr{Key: MessageKey, Value: attr.Value}
	case slog.LevelKey:
		level, ok := attr.Value.Any().(slog.Level)
		if !ok {
			return attr
		}
		return slog.String(SeverityKey, Severity(level))
	default:
		return attr
	}
}

func Severity(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARNING"
	case level < slog.LevelError+4:
		return "ERROR"
	default:
		return "CRITICAL"
	}
}

type Trace struct {
	ID      string
	Span    string
	Sampled bool
}

func TraceOf(r *http.Request) Trace {
	if header := r.Header.Get(CloudTraceHeader); header != "" {
		return cloudTrace(header)
	}
	if header := r.Header.Get(TraceParent); header != "" {
		return parentTrace(header)
	}
	if ray := r.Header.Get(RayHeader); ray != "" {
		return Trace{ID: ray}
	}
	return Trace{}
}

func cloudTrace(header string) Trace {
	trace := Trace{ID: header}
	if cut := strings.IndexByte(trace.ID, ';'); cut >= 0 {
		trace.Sampled = strings.Contains(trace.ID[cut:], "o=1")
		trace.ID = trace.ID[:cut]
	}
	if cut := strings.IndexByte(trace.ID, '/'); cut >= 0 {
		trace.Span, trace.ID = trace.ID[cut+1:], trace.ID[:cut]
	}
	return trace
}

func parentTrace(header string) Trace {
	parts := strings.Split(header, "-")
	if len(parts) < 4 {
		return Trace{}
	}
	return Trace{ID: parts[1], Span: parts[2], Sampled: strings.HasSuffix(parts[3], "1")}
}

func (t Trace) Attrs(project string) []slog.Attr {
	if t.ID == "" {
		return nil
	}
	name := t.ID
	if project != "" {
		name = "projects/" + project + "/traces/" + t.ID
	}
	attrs := []slog.Attr{slog.String(TraceKey, name)}
	if t.Span != "" {
		attrs = append(attrs, slog.String(SpanKey, t.Span))
	}
	if t.Sampled {
		attrs = append(attrs, slog.Bool(SampledKey, true))
	}
	return attrs
}

func Request(r *http.Request, status, size int, took time.Duration) slog.Attr {
	return slog.Any(RequestKey, map[string]any{
		"requestMethod": r.Method,
		"requestUrl":    r.URL.RequestURI(),
		"status":        status,
		"responseSize":  strconv.Itoa(size),
		"userAgent":     r.UserAgent(),
		"remoteIp":      r.RemoteAddr,
		"protocol":      r.Proto,
		"latency":       strconv.FormatFloat(took.Seconds(), 'f', 9, 64) + "s",
	})
}
