package css

import (
	"io/fs"
	"os"
	"runtime"
)

const muslHelp = "on a system without a build of its own, set \"css\": {\"engine\": \"plain\"} in "

func (t Tailwind) musl() bool {
	return runtime.GOOS == "linux" && Musl(t.root())
}

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
