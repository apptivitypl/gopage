package css

import (
	"io/fs"
	"os"
)

const muslHelp = "the standalone tailwind build needs glibc; on alpine either build on a glibc image " +
	"such as golang:1.26-bookworm, or set \"css\": {\"engine\": \"plain\"} in "

func Musl(root fs.FS) bool {
	return glob(root, "lib/ld-musl-*") && !glob(root, "lib/ld-linux-*")
}

func glob(root fs.FS, pattern string) bool {
	matches, err := fs.Glob(root, pattern)
	if err != nil {
		return false
	}
	return len(matches) > 0
}

func (t Tailwind) root() fs.FS {
	if t.Root != nil {
		return t.Root
	}
	return os.DirFS("/")
}
