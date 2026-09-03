package fetch

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type entry struct {
	name string
	body string
	mode int64
	kind byte
	link string
}

func tarball(t *testing.T, name string, entries []entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	zipped := gzip.NewWriter(file)
	writer := tar.NewWriter(zipped)
	for _, item := range entries {
		header := &tar.Header{Name: item.name, Mode: item.mode, Typeflag: item.kind, Linkname: item.link}
		if item.kind == tar.TypeReg {
			header.Size = int64(len(item.body))
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("header %s: %v", item.name, err)
		}
		if _, err := writer.Write([]byte(item.body)); err != nil {
			t.Fatalf("write %s: %v", item.name, err)
		}
	}
	for _, closer := range []func() error{writer.Close, zipped.Close, file.Close} {
		if err := closer(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
	return path
}

func zipped(t *testing.T, entries []entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	writer := zip.NewWriter(file)
	for _, item := range entries {
		header := &zip.FileHeader{Name: item.name}
		header.SetMode(os.FileMode(item.mode))
		sink, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("header %s: %v", item.name, err)
		}
		if _, err := sink.Write([]byte(item.body)); err != nil {
			t.Fatalf("write %s: %v", item.name, err)
		}
	}
	for _, closer := range []func() error{writer.Close, file.Close} {
		if err := closer(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func TestUnpackWritesTheWholeTreeFromATarball(t *testing.T) {
	archive := tarball(t, "go.tar.gz", []entry{
		{name: "go/", mode: 0o755, kind: tar.TypeDir},
		{name: "go/bin/go", body: "binary", mode: 0o755, kind: tar.TypeReg},
		{name: "go/VERSION", body: "go1.27.1", mode: 0o644, kind: tar.TypeReg},
		{name: "go/bin/gofmt", mode: 0o755, kind: tar.TypeSymlink, link: "go"},
	})
	into := filepath.Join(t.TempDir(), "unpacked")
	if err := Unpack(archive, into); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if got := read(t, filepath.Join(into, "go", "bin", "go")); got != "binary" {
		t.Errorf("go = %q", got)
	}
	if got := read(t, filepath.Join(into, "go", "VERSION")); got != "go1.27.1" {
		t.Errorf("VERSION = %q", got)
	}
	info, err := os.Stat(filepath.Join(into, "go", "bin", "go"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("mode = %v, want the binary executable", info.Mode())
	}
	if _, err := os.Lstat(filepath.Join(into, "go", "bin", "gofmt")); err != nil {
		t.Errorf("the symlink was not written: %v", err)
	}
}

func TestUnpackWritesTheWholeTreeFromAZip(t *testing.T) {
	archive := zipped(t, []entry{
		{name: "go/", mode: int64(os.ModeDir | 0o755)},
		{name: "go/bin/go.exe", body: "binary", mode: 0o755},
	})
	into := filepath.Join(t.TempDir(), "unpacked")
	if err := Unpack(archive, into); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if got := read(t, filepath.Join(into, "go", "bin", "go.exe")); got != "binary" {
		t.Errorf("go.exe = %q", got)
	}
}

func TestUnpackRefusesAnEntryThatEscapesTheDirectory(t *testing.T) {
	into := filepath.Join(t.TempDir(), "unpacked")
	escaping := tarball(t, "evil.tar.gz", []entry{
		{name: "../escaped", body: "x", mode: 0o644, kind: tar.TypeReg},
	})
	err := Unpack(escaping, into)
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("err = %v, want the escaping entry refused", err)
	}
	archive := zipped(t, []entry{{name: "../escaped", body: "x", mode: 0o644}})
	if err := Unpack(archive, filepath.Join(t.TempDir(), "z")); err == nil {
		t.Error("a zip entry that escapes must be refused")
	}
}

func TestUnpackRefusesASymlinkThatEscapesTheDirectory(t *testing.T) {
	archive := tarball(t, "evil.tar.gz", []entry{
		{name: "go/loose", mode: 0o777, kind: tar.TypeSymlink, link: "../../elsewhere"},
	})
	err := Unpack(archive, filepath.Join(t.TempDir(), "unpacked"))
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("err = %v, want the escaping symlink refused", err)
	}
}

func TestUnpackReplacesASymlinkItAlreadyWrote(t *testing.T) {
	archive := tarball(t, "twice.tar.gz", []entry{
		{name: "go/link", mode: 0o777, kind: tar.TypeSymlink, link: "first"},
		{name: "go/link", mode: 0o777, kind: tar.TypeSymlink, link: "second"},
	})
	into := filepath.Join(t.TempDir(), "unpacked")
	if err := Unpack(archive, into); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	name, err := os.Readlink(filepath.Join(into, "go", "link"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if name != "second" {
		t.Errorf("link = %q, want the last entry to win", name)
	}
}

func TestUnpackSkipsAnEntryKindItDoesNotHandle(t *testing.T) {
	archive := tarball(t, "odd.tar.gz", []entry{
		{name: "go/fifo", mode: 0o644, kind: tar.TypeFifo},
		{name: "go/real", body: "kept", mode: 0o644, kind: tar.TypeReg},
	})
	into := filepath.Join(t.TempDir(), "unpacked")
	if err := Unpack(archive, into); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if got := read(t, filepath.Join(into, "go", "real")); got != "kept" {
		t.Errorf("real = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(into, "go", "fifo")); err == nil {
		t.Error("an entry kind rill does not handle must be skipped, not written")
	}
}

func TestUnpackReportsAnArchiveItCannotRead(t *testing.T) {
	if err := Unpack(filepath.Join(t.TempDir(), "absent.tar.gz"), t.TempDir()); err == nil {
		t.Error("a missing tarball must be reported")
	}
	if err := Unpack(filepath.Join(t.TempDir(), "absent.zip"), t.TempDir()); err == nil {
		t.Error("a missing zip must be reported")
	}
	plain := filepath.Join(t.TempDir(), "plain.tar.gz")
	if err := os.WriteFile(plain, []byte("not gzip"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := Unpack(plain, t.TempDir()); err == nil {
		t.Error("a tarball that is not gzip must be reported")
	}
}

func TestUnpackReportsADirectoryItCannotCreate(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	archive := tarball(t, "go.tar.gz", []entry{{name: "go/bin/go", body: "x", mode: 0o755, kind: tar.TypeReg}})
	if err := Unpack(archive, filepath.Join(blocked, "under")); err == nil {
		t.Error("a directory that cannot be created must be reported")
	}
}

func TestFileTakesOneBinaryOutOfAnArchive(t *testing.T) {
	archive := tarball(t, "rill.tar.gz", []entry{
		{name: "LICENSE", body: "mit", mode: 0o644, kind: tar.TypeReg},
		{name: "rill", body: "binary", mode: 0o644, kind: tar.TypeReg},
	})
	into := filepath.Join(t.TempDir(), "bin")
	if err := File(archive, "rill", into); err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := read(t, filepath.Join(into, "rill")); got != "binary" {
		t.Errorf("rill = %q", got)
	}
	info, err := os.Stat(filepath.Join(into, "rill"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("mode = %v, want the binary executable", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(into, "LICENSE")); err == nil {
		t.Error("File must take only the binary it was asked for")
	}
}

func TestFileTakesOneBinaryOutOfAZip(t *testing.T) {
	archive := zipped(t, []entry{
		{name: "LICENSE", body: "mit", mode: 0o644},
		{name: "rill.exe", body: "binary", mode: 0o644},
	})
	into := filepath.Join(t.TempDir(), "bin")
	if err := File(archive, "rill.exe", into); err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := read(t, filepath.Join(into, "rill.exe")); got != "binary" {
		t.Errorf("rill.exe = %q", got)
	}
}

func TestFileReportsABinaryTheArchiveDoesNotHold(t *testing.T) {
	archive := tarball(t, "rill.tar.gz", []entry{{name: "LICENSE", body: "mit", mode: 0o644, kind: tar.TypeReg}})
	err := File(archive, "rill", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "holds no rill") {
		t.Errorf("err = %v, want the missing binary named", err)
	}
	empty := zipped(t, nil)
	err = File(empty, "rill.exe", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "holds no rill.exe") {
		t.Errorf("err = %v, want the missing binary named", err)
	}
	if err := File(filepath.Join(t.TempDir(), "absent.tar.gz"), "rill", t.TempDir()); err == nil {
		t.Error("a missing archive must be reported")
	}
	if err := File(filepath.Join(t.TempDir(), "absent.zip"), "rill", t.TempDir()); err == nil {
		t.Error("a missing zip must be reported")
	}
}

func TestFileReportsADirectoryItCannotCreate(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := File("archive.tar.gz", "rill", filepath.Join(blocked, "bin")); err == nil {
		t.Error("a directory that cannot be created must be reported")
	}
}

func TestUnpackReportsAFileItCannotWrite(t *testing.T) {
	archive := tarball(t, "clash.tar.gz", []entry{
		{name: "go/bin", body: "a file where a directory belongs", mode: 0o644, kind: tar.TypeReg},
		{name: "go/bin/go", body: "binary", mode: 0o755, kind: tar.TypeReg},
	})
	if err := Unpack(archive, filepath.Join(t.TempDir(), "unpacked")); err == nil {
		t.Error("a file under a file must be reported")
	}
}

func TestUnpackReportsATruncatedArchive(t *testing.T) {
	archive := tarball(t, "cut.tar.gz", []entry{
		{name: "go/bin/go", body: strings.Repeat("binary", 4096), mode: 0o755, kind: tar.TypeReg},
	})
	whole, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cut := filepath.Join(t.TempDir(), "cut.tar.gz")
	if err := os.WriteFile(cut, whole[:len(whole)/2], 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := Unpack(cut, filepath.Join(t.TempDir(), "unpacked")); err == nil {
		t.Error("an archive that stops halfway must be reported")
	}
}

func TestUnpackReportsASymlinkItCannotWrite(t *testing.T) {
	archive := tarball(t, "clash.tar.gz", []entry{
		{name: "go/bin", body: "a file where a directory belongs", mode: 0o644, kind: tar.TypeReg},
		{name: "go/bin/gofmt", mode: 0o777, kind: tar.TypeSymlink, link: "go"},
	})
	if err := Unpack(archive, filepath.Join(t.TempDir(), "unpacked")); err == nil {
		t.Error("a symlink under a file must be reported")
	}
}
