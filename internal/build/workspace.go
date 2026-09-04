package build

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const workspaceOff = "GOWORK=off"

type asker func(dir, name string, args ...string) ([]byte, error)

func ask(dir, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	return command.Output()
}

func OutsideWorkspace(dir, command string) []string {
	return outsideWorkspace(dir, command, ask)
}

func outsideWorkspace(dir, command string, query asker) []string {
	from := nearest(dir)
	if from == "" {
		return nil
	}
	found, err := query(from, command, "env", "GOWORK")
	if err != nil {
		return nil
	}
	file := strings.TrimSpace(string(found))
	if file == "" {
		return nil
	}
	root := filepath.Dir(file)
	described, err := query(root, command, "work", "edit", "-json")
	if err != nil {
		return nil
	}
	var workspace struct {
		Use []struct{ DiskPath string }
	}
	if err := json.Unmarshal(described, &workspace); err != nil {
		return nil
	}
	target := resolved(dir)
	for _, member := range workspace.Use {
		path := member.DiskPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if resolved(path) == target {
			return nil
		}
	}
	return []string{workspaceOff}
}

func resolved(dir string) string {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if real, err := filepath.EvalSymlinks(absolute); err == nil {
		return real
	}
	return absolute
}

func nearest(dir string) string {
	at := resolved(dir)
	for {
		if entry, err := os.Stat(at); err == nil && entry.IsDir() {
			return resolved(at)
		}
		parent := filepath.Dir(at)
		if parent == at {
			return ""
		}
		at = parent
	}
}

func EnclosingModule(dir string) string {
	at := nearest(dir)
	if at == resolved(dir) {
		at = filepath.Dir(at)
	}
	for {
		if _, err := os.Stat(filepath.Join(at, "go.mod")); err == nil {
			return at
		}
		parent := filepath.Dir(at)
		if parent == at {
			return ""
		}
		at = parent
	}
}
