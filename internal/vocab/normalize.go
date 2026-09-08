package vocab

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/apptivitypl/gopage/internal/config"
)

func Normalise(path string, rules config.Normalize) string {
	if !rules.Any() || path == "" {
		return path
	}
	out := path
	if rules.Lowers() {
		out = strings.ToLower(out)
	}
	if rules.Folds() {
		out = fold(out)
	}
	return slashed(out, rules)
}

func slashed(path string, rules config.Normalize) string {
	if path == "/" {
		return path
	}
	switch {
	case rules.Strips():
		return strings.TrimSuffix(path, "/")
	case rules.Keeps() && !strings.HasSuffix(path, "/"):
		return path + "/"
	default:
		return path
	}
}

func fold(path string) string {
	if isASCII(path) {
		return path
	}
	var out strings.Builder
	out.Grow(len(path))
	for _, letter := range norm.NFD.String(path) {
		if unicode.Is(unicode.Mn, letter) {
			continue
		}
		out.WriteRune(letter)
	}
	return out.String()
}

func isASCII(text string) bool {
	for index := range len(text) {
		if text[index] >= unicode.MaxASCII {
			return false
		}
	}
	return true
}
