package demo

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed serve.mjs
var serveSource string

//go:embed server.mjs
var serverSource string

//go:embed runtime.mjs
var runtimeSource string

const (
	MetaFile    = "demo.json"
	PackageFile = "package.json"
	Entry       = "server.mjs"
)

var scripts = map[string]string{
	"serve.mjs":   serveSource,
	Entry:         serverSource,
	"runtime.mjs": runtimeSource,
}

type Meta struct {
	Name        string   `json:"name"`
	WorkerFirst []string `json:"workerFirst"`
}

type manifest struct {
	Name    string            `json:"name"`
	Private bool              `json:"private"`
	Type    string            `json:"type"`
	Scripts map[string]string `json:"scripts"`
}

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

func PackageName(name string) string {
	cleaned := unsafeName.ReplaceAllString(strings.ToLower(name), "-")
	cleaned = strings.Trim(cleaned, "-._")
	if cleaned == "" {
		cleaned = "gopage"
	}
	return cleaned + "-demo"
}

func Write(dir string, meta Meta) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, source := range scripts {
		if err := write(filepath.Join(dir, name), []byte(source), mode(name)); err != nil {
			return err
		}
	}
	if meta.WorkerFirst == nil {
		meta.WorkerFirst = []string{}
	}
	if err := writeJSON(filepath.Join(dir, MetaFile), meta); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, PackageFile), manifest{
		Name:    PackageName(meta.Name),
		Private: true,
		Type:    "module",
		Scripts: map[string]string{"start": "node " + Entry},
	})
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	return write(path, append(data, '\n'), 0o644)
}

func mode(name string) os.FileMode {
	if name == Entry {
		return 0o755
	}
	return 0o644
}

func write(path string, data []byte, perm os.FileMode) error {
	if held, err := os.ReadFile(path); err == nil && string(held) == string(data) {
		return os.Chmod(path, perm)
	}
	return os.WriteFile(path, data, perm)
}
