package devserver

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/apptivitypl/gopage/internal/paths"
)

var Watched = []string{paths.AppDir, paths.ComponentsDir, paths.LocalesDir, paths.StylesDir}

var ignored = ignoredPaths()

func ignoredPaths() []string {
	list := append(paths.Generated(), ".wrangler", "node_modules", ".git")
	for i, name := range list {
		list[i] = filepath.FromSlash(name)
	}
	return list
}

func Watch(dir string, changed chan<- string, done <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	addTree(watcher, dir)
	go func() {
		defer func() { _ = watcher.Close() }()
		var timer *time.Timer
		for {
			select {
			case <-done:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) {
					addTree(watcher, event.Name)
				}
				if !Relevant(event.Name) {
					continue
				}
				timer = reset(timer, changed, Relative(dir, event.Name))
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

func reset(timer *time.Timer, changed chan<- string, name string) *time.Timer {
	if timer != nil {
		timer.Stop()
	}
	return time.AfterFunc(Debounce, func() {
		select {
		case changed <- name:
		default:
		}
	})
}

func addTree(watcher *fsnotify.Watcher, root string) {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return
	}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		if skip(path) {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

func Relevant(path string) bool {
	if skip(path) {
		return false
	}
	return !strings.Contains(filepath.Base(path), ".generated.")
}

func skip(path string) bool {
	cleaned := filepath.Clean(path)
	separator := string(filepath.Separator)
	for _, name := range ignored {
		switch {
		case cleaned == name:
			return true
		case strings.HasSuffix(cleaned, separator+name):
			return true
		case strings.Contains(cleaned, separator+name+separator):
			return true
		}
	}
	return false
}
