package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/config"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/logs"
	"github.com/apptivitypl/rill/internal/runtime"
)

func watched(t *testing.T, opts Options) (*App, *strings.Builder) {
	t.Helper()
	var written strings.Builder
	opts.Logger = slog.New(slog.NewTextHandler(&written, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return New(opts), &written
}

func hostConfig() config.Config {
	settings := config.Default()
	settings.Hosts = []config.Host{{Pattern: "demo.test"}}
	return settings
}

func plainManifest() *ir.Manifest {
	plan := ir.Plan{Blob: []byte("<p>page</p>"), Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 11}}}
	return &ir.Manifest{
		Plans:  []ir.Plan{plan},
		Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
	}
}

func TestEveryRequestLeavesOneLine(t *testing.T) {
	app, written := watched(t, Options{Manifest: plainManifest(), AccessLog: true})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	line := written.String()
	for _, want := range []string{"level=INFO", "msg=request", "method=GET", "path=/", "status=200"} {
		if !strings.Contains(line, want) {
			t.Errorf("log = %q, want it to carry %q", line, want)
		}
	}
	if !strings.Contains(line, "bytes=11") {
		t.Errorf("log = %q, want the byte count", line)
	}
}

func TestTheAccessLogStaysQuietWhenItIsOff(t *testing.T) {
	app, written := watched(t, Options{Manifest: plainManifest()})
	app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(written.String(), "msg=request") {
		t.Errorf("log = %q, want nothing when the platform logs requests itself", written.String())
	}
}

func TestTheLevelFollowsTheStatus(t *testing.T) {
	app, written := watched(t, Options{Manifest: plainManifest(), AccessLog: true})
	app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope", nil))
	if !strings.Contains(written.String(), "level=WARN") {
		t.Errorf("log = %q, want a missing page to warn", written.String())
	}
	for status, want := range map[int]slog.Level{
		200: slog.LevelInfo, 302: slog.LevelInfo, 404: slog.LevelWarn,
		422: slog.LevelWarn, 500: slog.LevelError, 503: slog.LevelError,
	} {
		if got := levelFor(status); got != want {
			t.Errorf("levelFor(%d) = %v, want %v", status, got, want)
		}
	}
}

func TestATraceHeaderReachesEveryLine(t *testing.T) {
	t.Setenv(logs.ProjectVar, "demo")
	app, written := watched(t, Options{Manifest: plainManifest(), AccessLog: true})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(logs.CloudTraceHeader, "abc123/7;o=1")
	app.Handler().ServeHTTP(httptest.NewRecorder(), request)

	line := written.String()
	if !strings.Contains(line, "projects/demo/traces/abc123") {
		t.Errorf("log = %q, want the trace resolved against the project", line)
	}
	if !strings.Contains(line, "spanId=7") {
		t.Errorf("log = %q, want the span", line)
	}
}

func TestAPanickingHandlerDoesNotTakeTheServerDown(t *testing.T) {
	app, written := watched(t, Options{
		Manifest: plainManifest(),
		Props: map[string]PropsProvider{
			"home": func(*http.Request, Params) (runtime.Accessible, error) { panic("loader exploded") },
		},
	})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the error page", recorder.Code)
	}
	line := written.String()
	if !strings.Contains(line, "handler panicked") || !strings.Contains(line, "loader exploded") {
		t.Errorf("log = %q, want the panic named", line)
	}
	if !strings.Contains(line, "stack=") {
		t.Errorf("log = %q, want a stack", line)
	}
}

func TestAPanicAfterTheHeaderLeavesTheResponseAlone(t *testing.T) {
	app, written := watched(t, Options{Manifest: plainManifest()})
	handler := app.observe(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		panic("too late")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusTeapot {
		t.Errorf("status = %d, want the status the handler already sent", recorder.Code)
	}
	if !strings.Contains(written.String(), "handler panicked") {
		t.Errorf("log = %q", written.String())
	}
}

func TestARefusedHostAndMethodLeaveALine(t *testing.T) {
	app, written := watched(t, Options{
		Manifest: plainManifest(),
		Config:   hostConfig(),
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = "elsewhere.test"
	app.Handler().ServeHTTP(httptest.NewRecorder(), request)
	if !strings.Contains(written.String(), "host refused") {
		t.Errorf("log = %q, want a refused host recorded", written.String())
	}

	app, written = watched(t, Options{Manifest: plainManifest()})
	app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/", nil))
	if !strings.Contains(written.String(), "method not allowed") {
		t.Errorf("log = %q, want a refused method recorded", written.String())
	}
}

func TestTheRecorderKeepsStreamingAlive(t *testing.T) {
	recorder := httptest.NewRecorder()
	wrapped := &Recorder{ResponseWriter: recorder}
	if wrapped.Status() != http.StatusOK {
		t.Errorf("status = %d, want a default of 200", wrapped.Status())
	}
	if _, err := wrapped.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	wrapped.Flush()
	if !recorder.Flushed {
		t.Error("a wrapped writer must still flush, or streaming and sse stop")
	}
	if wrapped.Status() != http.StatusOK || wrapped.written != 3 {
		t.Errorf("status = %d, written = %d", wrapped.Status(), wrapped.written)
	}
	wrapped.WriteHeader(http.StatusTeapot)
	if wrapped.Status() != http.StatusOK {
		t.Error("the first status wins, as net/http does")
	}
}

func TestAPanicAfterEarlyHintsStillRendersTheErrorPage(t *testing.T) {
	app, written := watched(t, Options{
		Manifest:  withFallbacks(manifest()),
		AssetLink: `</assets/app.abc.css>; rel=preload; as=style`,
		AccessLog: true,
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) { panic("loader exploded") },
		},
	})
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if response.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want the error page after a hint", response.StatusCode)
	}
	if !strings.Contains(string(body), "boom!") {
		t.Errorf("body = %q, want the error fallback", body)
	}
	if !strings.Contains(written.String(), "status=500") {
		t.Errorf("log = %q, want the final status, not the hint", written.String())
	}
}

func TestTheHintDoesNotBecomeTheLoggedStatus(t *testing.T) {
	app, written := watched(t, Options{
		Manifest:  manifest(),
		AssetLink: `</assets/app.abc.css>; rel=preload; as=style`,
		AccessLog: true,
	})
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if !strings.Contains(written.String(), "status=200") {
		t.Errorf("log = %q, want 200", written.String())
	}
}
