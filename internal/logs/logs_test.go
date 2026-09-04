package logs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func lines(t *testing.T, opts Options, write func(*slog.Logger)) string {
	t.Helper()
	var out bytes.Buffer
	opts.Writer = &out
	write(slog.New(Handler(opts)))
	return out.String()
}

func TestTheGoogleHandlerSpeaksCloudLogging(t *testing.T) {
	text := lines(t, Options{Format: FormatGCP}, func(logger *slog.Logger) {
		logger.Error("render failed", "route", "home")
	})

	var entry map[string]any
	if err := json.Unmarshal([]byte(text), &entry); err != nil {
		t.Fatalf("Unmarshal: %v, line = %q", err, text)
	}
	if entry[SeverityKey] != "ERROR" {
		t.Errorf("severity = %v, want ERROR", entry[SeverityKey])
	}
	if entry[MessageKey] != "render failed" {
		t.Errorf("message = %v", entry[MessageKey])
	}
	if entry["route"] != "home" {
		t.Errorf("route = %v", entry["route"])
	}
	if _, ok := entry["msg"]; ok {
		t.Error("the default message key must not survive")
	}
}

func TestEveryLevelMapsToASeverity(t *testing.T) {
	cases := map[slog.Level]string{
		slog.LevelDebug:      "DEBUG",
		slog.LevelInfo:       "INFO",
		slog.LevelWarn:       "WARNING",
		slog.LevelError:      "ERROR",
		slog.LevelError + 4:  "CRITICAL",
		slog.LevelDebug - 4:  "DEBUG",
		slog.LevelInfo + 1:   "INFO",
		slog.LevelError + 40: "CRITICAL",
	}
	for level, want := range cases {
		if got := Severity(level); got != want {
			t.Errorf("Severity(%v) = %q, want %q", level, got, want)
		}
	}
}

func TestAGroupedAttributeKeepsItsName(t *testing.T) {
	text := lines(t, Options{Format: FormatGCP}, func(logger *slog.Logger) {
		logger.WithGroup("upstream").Info("called", "status", 200)
	})
	if !strings.Contains(text, `"upstream":{"status":200}`) {
		t.Errorf("line = %q, want the group untouched", text)
	}
}

func TestTheCloudTraceHeaderIsSplit(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(CloudTraceHeader, "105445aa7843bc8bf206b120001000/1234;o=1")
	trace := TraceOf(request)
	if trace.ID != "105445aa7843bc8bf206b120001000" || trace.Span != "1234" || !trace.Sampled {
		t.Errorf("trace = %+v", trace)
	}

	attrs := trace.Attrs("demo")
	if len(attrs) != 3 || attrs[0].Value.String() != "projects/demo/traces/"+trace.ID {
		t.Errorf("attrs = %v", attrs)
	}
	if bare := trace.Attrs(""); bare[0].Value.String() != trace.ID {
		t.Errorf("attrs = %v, want the bare id without a project", bare)
	}
}

func TestATraceWithoutASpanOrSampling(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(CloudTraceHeader, "abc")
	trace := TraceOf(request)
	if trace.ID != "abc" || trace.Span != "" || trace.Sampled {
		t.Errorf("trace = %+v", trace)
	}
	if len(trace.Attrs("")) != 1 {
		t.Errorf("attrs = %v, want the id alone", trace.Attrs(""))
	}
}

func TestTheTraceParentHeaderIsRead(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(TraceParent, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	trace := TraceOf(request)
	if trace.ID != "4bf92f3577b34da6a3ce929d0e0e4736" || trace.Span != "00f067aa0ba902b7" || !trace.Sampled {
		t.Errorf("trace = %+v", trace)
	}

	request.Header.Set(TraceParent, "broken")
	if got := TraceOf(request); got.ID != "" {
		t.Errorf("trace = %+v, want nothing from a malformed header", got)
	}
}

func TestTheCloudflareRayIsTheLastResort(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RayHeader, "8f1c2d3e4f5a6b7c-WAW")
	if got := TraceOf(request); got.ID != "8f1c2d3e4f5a6b7c-WAW" {
		t.Errorf("trace = %+v", got)
	}
	if got := TraceOf(httptest.NewRequest(http.MethodGet, "/", nil)); got.ID != "" {
		t.Errorf("trace = %+v, want nothing without a header", got)
	}
}

func TestTheRequestAttributeCarriesWhatCloudLoggingWants(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/listings?city=krakow", nil)
	request.Header.Set("User-Agent", "probe")
	fields, ok := Request(request, 200, 1234, 1500*time.Microsecond).Value.Any().(map[string]any)
	if !ok {
		t.Fatal("the attribute must carry a map")
	}
	if fields["requestUrl"] != "/listings?city=krakow" || fields["status"] != 200 {
		t.Errorf("fields = %v", fields)
	}
	if fields["latency"] != "0.001500000s" {
		t.Errorf("latency = %v", fields["latency"])
	}
	if fields["responseSize"] != "1234" || fields["userAgent"] != "probe" {
		t.Errorf("fields = %v", fields)
	}
}

func TestThePrettyHandlerWritesOneReadableLine(t *testing.T) {
	text := lines(t, Options{Format: FormatPretty}, func(logger *slog.Logger) {
		logger.Info("served", "path", "/", "took", "1.2ms")
	})
	for _, want := range []string{"INFO", "served", "path=/", "took=1.2ms"} {
		if !strings.Contains(text, want) {
			t.Errorf("line = %q, want it to carry %q", text, want)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Errorf("line = %q, want no colour without a terminal", text)
	}
	if strings.Count(text, "\n") != 1 {
		t.Errorf("line = %q, want exactly one line", text)
	}
}

func TestThePrettyHandlerPaintsWhenAskedTo(t *testing.T) {
	text := lines(t, Options{Format: FormatPretty, Color: true}, func(logger *slog.Logger) {
		logger.Error("broken")
	})
	if !strings.Contains(text, red) || !strings.Contains(text, reset) {
		t.Errorf("line = %q, want the level painted", text)
	}
}

func TestThePrettyHandlerQuotesAValueWithSpaces(t *testing.T) {
	text := lines(t, Options{Format: FormatPretty}, func(logger *slog.Logger) {
		logger.Info("served", "error", "upstream is down")
	})
	if !strings.Contains(text, `error="upstream is down"`) {
		t.Errorf("line = %q", text)
	}
}

func TestThePrettyHandlerKeepsGroupsAndAttributes(t *testing.T) {
	text := lines(t, Options{Format: FormatPretty}, func(logger *slog.Logger) {
		logger.With("app", "demo").WithGroup("cache").Info("hit", "key", "home")
	})
	if !strings.Contains(text, "app=demo") || !strings.Contains(text, "cache.key=home") {
		t.Errorf("line = %q", text)
	}
}

func TestThePrettyHandlerHonoursItsLevel(t *testing.T) {
	text := lines(t, Options{Format: FormatPretty, Level: slog.LevelWarn}, func(logger *slog.Logger) {
		logger.Info("quiet")
		logger.Warn("loud")
	})
	if strings.Contains(text, "quiet") || !strings.Contains(text, "loud") {
		t.Errorf("lines = %q", text)
	}
}

func TestEveryLevelHasALabelAndATint(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if strings.TrimSpace(label(level)) == "" || tint(level) == "" {
			t.Errorf("level %v has no label or tint", level)
		}
	}
}

func TestTheFormatIsPickedFromTheEnvironment(t *testing.T) {
	for _, want := range formats {
		if got := pick(want); got != want {
			t.Errorf("pick(%q) = %q", want, got)
		}
	}
	t.Setenv(ServiceVar, "demo")
	if got := pick("nonsense"); got != FormatGCP {
		t.Errorf("pick = %q, want cloud run to be recognised", got)
	}
	t.Setenv(ServiceVar, "")
	if got := pick(""); got != FormatJSON {
		t.Errorf("pick = %q, want json away from a terminal", got)
	}
}

func TestTheLevelIsReadFromText(t *testing.T) {
	cases := []struct {
		text string
		want slog.Level
	}{
		{"debug", slog.LevelDebug}, {"DEBUG", slog.LevelDebug},
		{"warn", slog.LevelWarn}, {"warning", slog.LevelWarn},
		{" \tERROR ", slog.LevelError}, {"error", slog.LevelError},
		{"", slog.LevelInfo}, {"nonsense", slog.LevelInfo},
	}
	for _, entry := range cases {
		text, want := entry.text, entry.want
		if got := LevelOf(text); got != want {
			t.Errorf("LevelOf(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestTheAccessLogFollowsThePlatform(t *testing.T) {
	t.Setenv(ServiceVar, "")
	t.Setenv(AccessVar, "")
	if !Access() {
		t.Error("a plain binary logs its own requests")
	}
	t.Setenv(ServiceVar, "demo")
	if Access() {
		t.Error("cloud run already logs every request")
	}
	t.Setenv(AccessVar, "on")
	if !Access() {
		t.Error("the switch wins over the platform")
	}
	t.Setenv(AccessVar, "off")
	if Access() {
		t.Error("the switch turns it off too")
	}
}

func TestResolveDecidesWriterFormatAndColour(t *testing.T) {
	t.Setenv(ServiceVar, "")
	opts := Resolve(FormatPretty, "debug")
	if opts.Writer != os.Stderr {
		t.Error("logs belong on stderr, stdout carries results")
	}
	if opts.Level != slog.LevelDebug || opts.Format != FormatPretty {
		t.Errorf("opts = %+v", opts)
	}
}

func TestColourIsRefusedWhenAsked(t *testing.T) {
	t.Setenv(ColorVar, "1")
	if colored(os.Stderr) {
		t.Error("NO_COLOR must be honoured")
	}
	t.Setenv(ColorVar, "")
	t.Setenv("TERM", "dumb")
	if colored(os.Stderr) {
		t.Error("a dumb terminal takes no colour")
	}
	if terminal(new(bytes.Buffer)) {
		t.Error("a buffer is not a terminal")
	}
}

func TestNewBuildsALoggerThatWrites(t *testing.T) {
	t.Setenv(FormatVar, FormatJSON)
	if New() == nil {
		t.Fatal("New must return a logger")
	}
	if Handler(Options{Format: FormatConsole, Writer: nil}) == nil {
		t.Error("every format resolves to a handler")
	}
}

func TestTheDevServerChildPrintsPrettyColouredLines(t *testing.T) {
	t.Setenv(ServiceVar, "")
	t.Setenv(ColorVar, "")
	t.Setenv("TERM", "xterm")
	t.Setenv(DevVar, "1")
	opts := Resolve("", "")
	if opts.Format != FormatPretty || !opts.Color {
		t.Errorf("opts = %+v, want pretty and coloured through a pipe under gopage dev", opts)
	}
	t.Setenv(ColorVar, "1")
	if Resolve("", "").Color {
		t.Error("NO_COLOR still wins under gopage dev")
	}
	t.Setenv(DevVar, "")
	if Resolve("", "").Format != FormatJSON {
		t.Error("without gopage dev a pipe gets json")
	}
}

func TestThePrettyHandlerFoldsARequestIntoOneReadableLine(t *testing.T) {
	text := lines(t, Options{Format: FormatPretty}, func(logger *slog.Logger) {
		logger.Info(RequestMessage, "method", "GET", "path", "/api/stories", "status", 200,
			"bytes", 763, "took", "993ms", RequestKey, map[string]any{"latency": "0.9s"})
	})
	if !strings.Contains(text, "GET /api/stories  200  993ms  763 B") {
		t.Errorf("line = %q, want method, path, status, time and size in that order", text)
	}
	if strings.Contains(text, "httpRequest") || strings.Contains(text, "latency") {
		t.Errorf("line = %q, want the cloud logging payload kept out of a terminal", text)
	}
	if strings.Contains(text, "method=") {
		t.Errorf("line = %q, want no key=value noise on a request line", text)
	}
}

func TestARequestLineTintsTheStatus(t *testing.T) {
	text := lines(t, Options{Format: FormatPretty, Color: true}, func(logger *slog.Logger) {
		logger.Warn(RequestMessage, "method", "GET", "path", "/nope", "status", 404, "bytes", 1, "took", "1ms")
	})
	if !strings.Contains(text, amber+"404"+reset) {
		t.Errorf("line = %q, want a 4xx painted amber", text)
	}
	if got := statusTint("500"); got != red {
		t.Errorf("tint = %q", got)
	}
	if got := statusTint("200"); got != dim {
		t.Errorf("tint = %q", got)
	}
}

func TestARequestLineRoundsItsDuration(t *testing.T) {
	cases := map[time.Duration]string{
		123083 * time.Nanosecond:   "123µs",
		2169375 * time.Nanosecond:  "2.2ms",
		1268995 * time.Microsecond: "1.27s",
	}
	for took, want := range cases {
		if got := elapsed(slog.DurationValue(took)); got != want {
			t.Errorf("elapsed(%v) = %q, want %q", took, got, want)
		}
	}
	if got := elapsed(slog.StringValue("raw")); got != "raw" {
		t.Errorf("elapsed = %q, want a plain value passed through", got)
	}
}

func TestAControlCharacterCannotForgeALogLine(t *testing.T) {
	var out strings.Builder
	logger := slog.New(Handler(Options{Writer: &out, Format: FormatPretty}))
	logger.Info(RequestMessage,
		"method", "GET",
		"path", "/x\n12:00:00 INFO  forged",
		"status", 200,
		"bytes", 0,
		"took", time.Duration(0),
	)
	logger.Warn("rewrite refused", "path", "/y\nanother forged line")

	text := out.String()
	if lines := strings.Count(strings.TrimRight(text, "\n"), "\n"); lines != 1 {
		t.Errorf("wrote %d lines, want one per record:\n%s", lines+1, text)
	}
	if !strings.Contains(text, `\n`) {
		t.Errorf("the newline was not escaped:\n%s", text)
	}
}
