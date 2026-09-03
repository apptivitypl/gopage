package devserver

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apptivitypl/rill/internal/diag"
)

func buildApp(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "app")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./testdata/app")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return binary
}

func TestStartServesTheApplicationThroughTheProxy(t *testing.T) {
	app, err := Start(Launch{Dir: t.TempDir(), Binary: buildApp(t), Env: []string{"TAG=first"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer app.Stop()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example.test/features", nil)
	app.Handler().ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if !strings.Contains(body, "path=/features") {
		t.Errorf("body = %q, want the proxied path", body)
	}
	if !strings.Contains(body, "host=example.test") {
		t.Errorf("body = %q, want the original host preserved", body)
	}
	if !strings.Contains(body, "tag=first") {
		t.Errorf("body = %q, want the environment passed through", body)
	}
}

func TestStopEndsTheProcess(t *testing.T) {
	app, err := Start(Launch{Dir: t.TempDir(), Binary: buildApp(t)})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	app.Stop()

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://example.test/", nil))
	if recorder.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 once the process is gone", recorder.Code)
	}
}

func TestStopOnNothingIsSafe(t *testing.T) {
	var app *App
	app.Stop()
	(&App{}).Stop()
}

func TestStartReportsABinaryThatNeverListens(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "silent")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Start(Launch{Dir: t.TempDir(), Binary: binary, Timeout: 200 * time.Millisecond})
	if err == nil {
		t.Fatal("Start should fail when the application never listens")
	}
	if !strings.Contains(err.Error(), "never listened") {
		t.Errorf("err = %v", err)
	}
}

func TestStartReportsAMissingBinary(t *testing.T) {
	if _, err := Start(Launch{Dir: t.TempDir(), Binary: filepath.Join(t.TempDir(), "absent")}); err == nil {
		t.Fatal("Start should fail when the binary does not exist")
	}
}

func TestFreePortIsFree(t *testing.T) {
	port, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("the reported port is not free: %v", err)
	}
	_ = listener.Close()
}

func TestAToolchainFailureIsShownInTheOverlay(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return nil, nil, nil, errors.New("go build: undefined: Listing")
	}, nil)
	if server.Rebuild() {
		t.Fatal("Rebuild should report the failure")
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
	if !strings.Contains(body, "undefined: Listing") {
		t.Errorf("overlay = %q, want the toolchain output", body)
	}
	if !strings.Contains(body, ReloadPath) {
		t.Error("the overlay should keep listening for the next build")
	}
}

func TestADiagnosticFailureKeepsTheDiagnosticOverlay(t *testing.T) {
	broken := diag.New("C107", "app/page.rill", diag.At(0), "unterminated expression")
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return nil, []diag.Diagnostic{broken}, map[string]string{"app/page.rill": "<p>{{"}, errors.New("build failed")
	}, nil)
	server.Rebuild()

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(recorder.Body.String(), "unterminated expression") {
		t.Errorf("overlay = %q, want the diagnostic", recorder.Body.String())
	}
}

func TestRelevantIgnoresGeneratedAndBuildOutput(t *testing.T) {
	cases := map[string]bool{
		filepath.Join("app", "page.rill"):                   true,
		filepath.Join("locales", "en.json"):                 true,
		filepath.Join("components", "Counter", "client.ts"): true,
		filepath.Join("public", "favicon.ico"):              true,
		filepath.Join("project", "dist"):                    false,
		filepath.Join("project", "internal", "gen"):         false,
		filepath.Join("project", ".wrangler"):               false,
	}
	for path, want := range cases {
		if got := Relevant(path); got != want {
			t.Errorf("Relevant(%q) = %v, want %v", path, got, want)
		}
	}
}
