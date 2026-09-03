package fetch

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Unpack(archive, into string) error {
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(archive, ".zip") {
		return unpackZip(archive, into)
	}
	return unpackTar(archive, into)
}

func File(archive, wanted, into string) error {
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(archive, ".zip") {
		return fileFromZip(archive, wanted, into)
	}
	return fileFromTar(archive, wanted, into)
}

func unpackTar(archive, into string) error {
	return walkTar(archive, func(header *tar.Header, reader io.Reader) (bool, error) {
		target, err := resolve(into, header.Name)
		if err != nil {
			return false, err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			return false, os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			return false, write(reader, target, header.FileInfo().Mode())
		case tar.TypeSymlink:
			return false, link(into, target, header.Linkname)
		}
		return false, nil
	})
}

func fileFromTar(archive, wanted, into string) error {
	found := false
	err := walkTar(archive, func(header *tar.Header, reader io.Reader) (bool, error) {
		if filepath.Base(header.Name) != wanted {
			return false, nil
		}
		found = true
		return true, write(reader, filepath.Join(into, wanted), 0o755)
	})
	if err == nil && !found {
		return fmt.Errorf("%s holds no %s", archive, wanted)
	}
	return err
}

func walkTar(archive string, visit func(*tar.Header, io.Reader) (bool, error)) error {
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
			return nil
		}
		if err != nil {
			return err
		}
		done, err := visit(header, reader)
		if err != nil || done {
			return err
		}
	}
}

func unpackZip(archive, into string) error {
	return walkZip(archive, func(entry *zip.File) (bool, error) {
		target, err := resolve(into, entry.Name)
		if err != nil {
			return false, err
		}
		if entry.FileInfo().IsDir() {
			return false, os.MkdirAll(target, 0o755)
		}
		return false, copyZip(entry, target, entry.Mode())
	})
}

func fileFromZip(archive, wanted, into string) error {
	found := false
	err := walkZip(archive, func(entry *zip.File) (bool, error) {
		if filepath.Base(entry.Name) != wanted {
			return false, nil
		}
		found = true
		return true, copyZip(entry, filepath.Join(into, wanted), 0o755)
	})
	if err == nil && !found {
		return fmt.Errorf("%s holds no %s", archive, wanted)
	}
	return err
}

func walkZip(archive string, visit func(*zip.File) (bool, error)) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	for _, entry := range reader.File {
		done, err := visit(entry)
		if err != nil || done {
			return err
		}
	}
	return nil
}

func copyZip(entry *zip.File, target string, mode os.FileMode) error {
	opened, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() { _ = opened.Close() }()
	return write(opened, target, mode)
}

func write(source io.Reader, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, source); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func link(root, target, name string) error {
	if err := resolvable(root, filepath.Join(filepath.Dir(target), name)); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(name, target)
}

func resolve(root, name string) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := resolvable(root, target); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return target, nil
}

func resolvable(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("points outside the directory it unpacks into")
	}
	return nil
}
