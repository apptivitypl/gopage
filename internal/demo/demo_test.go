package demo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestWriteLaysOutARunnableFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Write(dir, Meta{Name: "Hello World", WorkerFirst: []string{"/api/*"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, name := range []string{"serve.mjs", "server.mjs", "runtime.mjs", MetaFile, PackageFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	var meta Meta
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, MetaFile))), &meta); err != nil {
		t.Fatalf("decode %s: %v", MetaFile, err)
	}
	if len(meta.WorkerFirst) != 1 || meta.WorkerFirst[0] != "/api/*" {
		t.Errorf("WorkerFirst = %v", meta.WorkerFirst)
	}
	var pkg manifest
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, PackageFile))), &pkg); err != nil {
		t.Fatalf("decode %s: %v", PackageFile, err)
	}
	if pkg.Name != "hello-world-demo" || pkg.Scripts["start"] != "node server.mjs" {
		t.Errorf("package.json = %+v", pkg)
	}
}

func TestWritePutsAnEmptyListWhenNothingRunsWorkerFirst(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Meta{Name: "site"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := read(t, filepath.Join(dir, MetaFile)); !strings.Contains(got, `"workerFirst": []`) {
		t.Errorf("%s = %s", MetaFile, got)
	}
}

func TestWriteLeavesAnUnchangedFileAlone(t *testing.T) {
	dir := t.TempDir()
	meta := Meta{Name: "site"}
	if err := Write(dir, meta); err != nil {
		t.Fatalf("Write: %v", err)
	}
	path := filepath.Join(dir, Entry)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := Write(dir, meta); err != nil {
		t.Fatalf("Write again: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat again: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("Write rewrote a file whose bytes did not change")
	}
}

func TestWriteReportsADirectoryItCannotCreate(t *testing.T) {
	file := filepath.Join(t.TempDir(), "taken")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := Write(filepath.Join(file, "demo"), Meta{Name: "site"}); err == nil {
		t.Error("Write should refuse a path that is not a directory")
	}
}

func TestPackageNameIsSafeForNpm(t *testing.T) {
	cases := map[string]string{
		"Hello World": "hello-world-demo",
		"My Site!!":   "my-site-demo",
		"---":         "gopage-demo",
		"blog":        "blog-demo",
	}
	for name, want := range cases {
		if got := PackageName(name); got != want {
			t.Errorf("PackageName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestWriteReportsAScriptItCannotWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, Entry), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := Write(dir, Meta{Name: "site"}); err == nil {
		t.Error("Write should report a script it cannot write")
	}
}

func TestWriteReportsAManifestItCannotWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, MetaFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := Write(dir, Meta{Name: "site"}); err == nil {
		t.Error("Write should report a manifest it cannot write")
	}
}
