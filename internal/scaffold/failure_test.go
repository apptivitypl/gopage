package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderReportsABrokenTemplate(t *testing.T) {
	if _, err := render("broken", []byte("{{ .Module"), data{}); err == nil {
		t.Error("a template that does not parse must fail")
	}
}

func TestRenderReportsAFailingExecution(t *testing.T) {
	_, err := render("broken", []byte("{{ .Module.Missing }}"), data{})
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v, want the template named", err)
	}
}

func TestWriteOneFailsWhenTheTargetPathIsBlocked(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "site"), []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeOne(dir, "site/site.go", "templates/hello-world/site/site.go.tmpl", data{Module: "example.com/x"})
	if err == nil {
		t.Error("writing under a regular file must fail")
	}
}

func TestWriteOneReportsAMissingTemplateFile(t *testing.T) {
	if err := writeOne(t.TempDir(), "x", "templates/hello-world/nope", data{}); err == nil {
		t.Error("a missing template file must fail")
	}
}

func TestEnsureEmptyRejectsARegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureEmpty(path); err == nil {
		t.Error("a regular file is not a usable project directory")
	}
}

func TestCreateFailsWhenTheDirectoryIsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Create(Config{Dir: path, Module: "example.com/x", Template: "hello-world"})
	if err == nil {
		t.Error("Create must fail when the target is a file")
	}
}

func TestTargetNameHandlesRenamesAndPlainFiles(t *testing.T) {
	cases := []struct {
		relative   string
		wantName   string
		wantRender bool
	}{
		{"gomod.tmpl", "go.mod", true},
		{"gitignore.tmpl", ".gitignore", true},
		{"app/page.gopage", "app/page.gopage", false},
		{"site/site.go.tmpl", "site/site.go", true},
	}
	for _, c := range cases {
		name, isTemplate := targetName(c.relative)
		if name != c.wantName || isTemplate != c.wantRender {
			t.Errorf("targetName(%q) = %q, %v; want %q, %v",
				c.relative, name, isTemplate, c.wantName, c.wantRender)
		}
	}
}
