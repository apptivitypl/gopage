package npmname

import (
	"regexp"
	"strings"
)

var unsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

const fallback = "gopage"

func Clean(name string) string {
	cleaned := unsafe.ReplaceAllString(strings.ToLower(name), "-")
	cleaned = strings.Trim(cleaned, "-._")
	if cleaned == "" {
		return fallback
	}
	return cleaned
}
