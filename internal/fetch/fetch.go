package fetch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const downloadTimeout = 10 * time.Minute

func Download(url, target, digest string) error {
	client := &http.Client{Timeout: downloadTimeout}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %d", url, response.StatusCode)
	}
	file, err := os.CreateTemp(filepath.Dir(target), "download-")
	if err != nil {
		return err
	}
	sum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, sum), response.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); digest != "" && got != digest {
		_ = os.Remove(file.Name())
		return fmt.Errorf("%s: sha256 %s, want %s; refusing to install", url, got, digest)
	}
	if err := os.Chmod(file.Name(), 0o755); err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	return os.Rename(file.Name(), target)
}

func Checksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
