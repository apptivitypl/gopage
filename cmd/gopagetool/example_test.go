package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apptivitypl/gopage/internal/tool/examplecheck"
)

func writeExample(t *testing.T, root string, example examplecheck.Example, text string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(example.Dir()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(text), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestThePinComesFromTheCommittedExample(t *testing.T) {
	root := t.TempDir()
	example := examplecheck.Examples()[0]
	writeExample(t, root, example, "module "+exampleModule+"/examples/x\n\ngo 1.26.0\n\nrequire (\n\t"+
		exampleModule+" v0.4.2\n\tgithub.com/syumai/workers v0.33.0\n)\n")
	got, err := pinnedVersion(root, example)
	if err != nil {
		t.Fatalf("pinnedVersion: %v", err)
	}
	if got != "v0.4.2" {
		t.Errorf("pinnedVersion = %q, want the version the example requires", got)
	}
}

func TestTheModuleLineIsNotMistakenForThePin(t *testing.T) {
	root := t.TempDir()
	example := examplecheck.Examples()[0]
	writeExample(t, root, example, "module "+exampleModule+"/examples/x\n\ngo 1.26.0\n")
	if _, err := pinnedVersion(root, example); err == nil {
		t.Error("an example that requires nothing must not report a pin")
	}
}

func TestAMissingExampleReportsWhereItLooked(t *testing.T) {
	example := examplecheck.Examples()[0]
	_, err := pinnedVersion(t.TempDir(), example)
	if err == nil {
		t.Fatal("a missing go.mod must be an error")
	}
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}
