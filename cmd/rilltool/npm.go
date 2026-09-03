package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/apptivitypl/rill/internal/paths"
	"github.com/apptivitypl/rill/internal/tool/examplecheck"
	"github.com/apptivitypl/rill/internal/tool/npmpkg"
	"github.com/apptivitypl/rill/internal/tool/shell"
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
	switch name {
	case npmpkg.CLI:
		return copyDir(filepath.Join(root, "npm", "rill"), dir)
	case npmpkg.Create:
		return copyDir(filepath.Join(root, "npm", "create-rill"), dir)
	}
	for _, platform := range npmpkg.Platforms() {
		if platform.Package() != name {
			continue
		}
		if from == "" {
			return fmt.Errorf("%s carries a binary; point --from at the directory holding %s", name, platform.Archive(version))
		}
		return unpackBinary(filepath.Join(from, platform.Archive(version)), platform.Binary(), filepath.Join(dir, "bin"))
	}
	for _, example := range examplecheck.Examples() {
		if npmpkg.DemoPackage(example.Name) != name {
			continue
		}
		return buildDemo(root, example, dir)
	}
	return fmt.Errorf("nothing knows how to assemble %q", name)
}

func buildDemo(root string, example examplecheck.Example, dir string) error {
	if err := shell.Run("go", "run", "./cmd/rill", "build", "--dir", example.Dir(), "--target", "demo"); err != nil {
		return err
	}
	built := filepath.Join(root, filepath.FromSlash(example.Dir()), filepath.FromSlash(paths.DemoDir))
	if err := copyDir(built, dir); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.md"), demoReadme(example.Name), 0o644)
}

func demoReadme(example string) []byte {
	return []byte(fmt.Sprintf(`# %s

The rill [%s example](https://github.com/apptivitypl/rill/tree/main/examples/%s), compiled to
WebAssembly against the browser runtime. It is the same module the Cloudflare Worker build produces,
so the pages, the loaders and the api routes are answered by the Go code, not by a snapshot.

`+"```bash"+`
pnpm dlx %s
`+"```"+`

It listens on 3000, or on PORT. Editing a template does not rebuild anything: that needs the Go
toolchain, which is what the repository itself is for.

Licensed MIT OR Apache-2.0.
`, npmpkg.DemoPackage(example), example, example, npmpkg.DemoPackage(example)))
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

func unpackBinary(archive, wanted, into string) error {
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}
	if filepath.Ext(archive) == ".zip" {
		return unpackZip(archive, wanted, into)
	}
	return unpackTar(archive, wanted, into)
}

func unpackZip(archive, wanted, into string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	for _, file := range reader.File {
		if filepath.Base(file.Name) != wanted {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			return err
		}
		defer func() { _ = opened.Close() }()
		return drain(opened, filepath.Join(into, wanted))
	}
	return fmt.Errorf("%s holds no %s", archive, wanted)
}

func unpackTar(archive, wanted, into string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	unzipped, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = unzipped.Close() }()
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s holds no %s", archive, wanted)
		}
		if err != nil {
			return err
		}
		if filepath.Base(header.Name) == wanted {
			return drain(reader, filepath.Join(into, wanted))
		}
	}
}

func drain(source io.Reader, path string) error {
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return err
	}
	return target.Close()
}
