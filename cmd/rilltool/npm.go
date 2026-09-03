package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/apptivitypl/rill/internal/fetch"
	"github.com/apptivitypl/rill/internal/tool/npmpkg"
)

const npmOut = "dist/npm"

func assembleNPM(root, name, version, from, out string) (string, error) {
	dir := filepath.Join(out, name)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	manifest, err := npmpkg.Manifest(name, version)
	if err != nil {
		return "", err
	}
	if err := fillNPM(root, name, version, from, dir); err != nil {
		return "", err
	}
	return dir, os.WriteFile(filepath.Join(dir, "package.json"), manifest, 0o644)
}

func fillNPM(root, name, version, from, dir string) error {
	if name == npmpkg.CLI {
		return copyDir(filepath.Join(root, "npm", "rill"), dir)
	}
	for _, platform := range npmpkg.Platforms() {
		if platform.Package() != name {
			continue
		}
		if from == "" {
			return fmt.Errorf("%s carries a binary; point --from at the directory holding %s", name, platform.Archive(version))
		}
		return fetch.File(filepath.Join(from, platform.Archive(version)), platform.Binary(), filepath.Join(dir, "bin"))
	}
	return fmt.Errorf("nothing knows how to assemble %q", name)
}

func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
