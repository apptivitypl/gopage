package build

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/paths"
)

func TestErrorMessageCountsDiagnostics(t *testing.T) {
	err := &Error{Diagnostics: []diag.Diagnostic{{Code: diag.C001}, {Code: diag.C002}}}
	if got := err.Error(); !strings.Contains(got, "2 errors") {
		t.Errorf("Error() = %q", got)
	}
}

func TestManifestWriteFailureStopsTheBuild(t *testing.T) {
	dir := project(t, helloWorld())
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(paths.Manifest)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("a manifest that cannot be written must fail the build")
	}
}

func TestStaticRenderFailureIsReported(t *testing.T) {
	dir := project(t, map[string]string{"app/page.gopage": "<p>{{ Missing }}</p>"})
	_, err := Run(Options{Dir: dir, Runner: &recorder{}})
	if err == nil || !strings.Contains(err.Error(), "render /") {
		t.Errorf("err = %v, want the failing route named", err)
	}
}

func TestAssetWriteFailureStopsTheBuild(t *testing.T) {
	dir := project(t, helloWorld())
	assets := filepath.Join(dir, "dist", "assets")
	if err := os.MkdirAll(filepath.Dir(assets), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assets, []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Runner: &recorder{}}); err == nil {
		t.Error("assets that cannot be written must fail the build")
	}
}

func TestWorkerBuildStopsAtTheFirstFailingCommand(t *testing.T) {
	dir := project(t, helloWorld())
	runner := &recorder{fail: os.ErrPermission}
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, Runner: runner}); err == nil {
		t.Fatal("expected the failing shim generator to stop the build")
	}
	if len(runner.commands) != 1 {
		t.Errorf("commands = %v, want the build to stop after the first failure", runner.names())
	}
	if _, err := os.Stat(filepath.Join(dir, paths.Wrangler)); err == nil {
		t.Error("no wrangler config should be written when the worker build failed")
	}
}

func TestWranglerWriteFailureIsReported(t *testing.T) {
	dir := project(t, helloWorld())
	if err := os.Mkdir(filepath.Join(dir, paths.Wrangler), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Dir: dir, Target: TargetWorkers, Runner: &recorder{}}); err == nil {
		t.Error("a wrangler config that cannot be written must fail the build")
	}
}

func TestSourcesAreReadOncePerFile(t *testing.T) {
	dir := project(t, map[string]string{"app/page.gopage": "{{ # }}{{ # }}"})
	_, err := Run(Options{Dir: dir, Runner: &recorder{}})

	var buildErr *Error
	if !errors.As(err, &buildErr) {
		t.Fatalf("err = %v", err)
	}
	if len(buildErr.Diagnostics) < 2 {
		t.Fatalf("diagnostics = %d, want both errors", len(buildErr.Diagnostics))
	}
	if len(buildErr.Sources) != 1 {
		t.Errorf("sources = %d entries, want one per file", len(buildErr.Sources))
	}
	if !strings.Contains(buildErr.Render(), "app/page.gopage") {
		t.Error("the rendered output lost the file name")
	}
}

func TestSourcesToleratesAMissingFile(t *testing.T) {
	err := &Error{
		Diagnostics: []diag.Diagnostic{diag.New(diag.C001, "gone.gopage", diag.Span{}, "boom")},
		Sources:     sourcesOf(t.TempDir(), []diag.Diagnostic{diag.New(diag.C001, "gone.gopage", diag.Span{}, "boom")}),
	}
	if got := err.Render(); !strings.Contains(got, "gone.gopage") {
		t.Errorf("render = %q, want it to survive a missing source file", got)
	}
}
