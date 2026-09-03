package fetch

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestChecksumReadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum, err := Checksum(path)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if sum != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("sum = %q", sum)
	}
	if _, err := Checksum(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("a missing file must be reported")
	}
}

func TestDownloadWritesTheFileAndChecksTheStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tailwindcss")
	if err := Download(server.URL+"/binary", target, ""); err != nil {
		t.Fatalf("Download: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("mode = %v, want the binary executable", info.Mode())
	}
	if err := Download(server.URL+"/missing", target, ""); err == nil {
		t.Error("a 404 must be reported")
	}
	if err := Download("http://127.0.0.1:1/binary", target, ""); err == nil {
		t.Error("a connection failure must be reported")
	}
	if err := Download(server.URL+"/binary", filepath.Join(t.TempDir(), "absent", "x"), ""); err == nil {
		t.Error("a target directory that does not exist must be reported")
	}
}

func TestDownloadReportsABodyItCannotWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = w.Write([]byte("short"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer server.Close()
	if err := Download(server.URL, filepath.Join(t.TempDir(), "out"), ""); err == nil {
		t.Error("a truncated body must be reported")
	}
}

func TestDownloadReportsAResponseItCannotRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort"))
		_ = conn.Close()
	}))
	defer server.Close()
	if err := Download(server.URL, filepath.Join(t.TempDir(), "out"), ""); err == nil {
		t.Error("a truncated response must be reported")
	}
}

func TestDownloadRefusesABinaryThatDoesNotMatchItsDigest(t *testing.T) {
	body := []byte("#!/bin/sh\nexit 0\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	sum := sha256.Sum256(body)
	target := filepath.Join(t.TempDir(), "tailwindcss")
	if err := Download(server.URL, target, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("Download with the right digest: %v", err)
	}

	swapped := filepath.Join(t.TempDir(), "tailwindcss")
	err := Download(server.URL, swapped, strings.Repeat("a", 64))
	if err == nil {
		t.Fatal("a binary that does not match its digest must be refused")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("err = %v, want the mismatch named", err)
	}
	if _, statErr := os.Stat(swapped); statErr == nil {
		t.Error("the rejected download must not be left on disk")
	}
}
