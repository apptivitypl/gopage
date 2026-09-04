package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	version = ""
	commit  = ""
	date    = ""
	module  = ""
)

func release() string {
	name, revision, built := stamped()
	var b strings.Builder
	fmt.Fprintf(&b, "gopage %s", name)
	if revision != "" {
		fmt.Fprintf(&b, " (%s)", revision)
	}
	if built != "" {
		fmt.Fprintf(&b, " built %s", built)
	}
	fmt.Fprintf(&b, "\n%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return b.String()
}

func stamped() (name, revision, built string) {
	name, revision, built = version, commit, date
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return orDev(name), revision, built
	}
	if name == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		name = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if revision == "" {
				revision = short(setting.Value)
			}
		case "vcs.time":
			if built == "" {
				built = setting.Value
			}
		}
	}
	return orDev(name), revision, built
}

func orDev(name string) string {
	if name == "" {
		return "(devel)"
	}
	return name
}

func short(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}

func ownVersion() string {
	name, _, _ := stamped()
	for _, candidate := range []string{module, name} {
		if pinnable(candidate) {
			return candidate
		}
	}
	return ""
}

func pinnable(name string) bool {
	if len(name) < 2 || name[0] != 'v' || name[1] < '0' || name[1] > '9' {
		return false
	}
	return !strings.ContainsAny(name, "+ ")
}
