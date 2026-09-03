package runtime

import "strings"

var safeSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
	"tel":    true,
}

func SafeURL(raw string) bool {
	scheme, ok := schemeOf(raw)
	if !ok {
		return true
	}
	return safeSchemes[scheme]
}

func schemeOf(raw string) (string, bool) {
	var scheme strings.Builder
	for i := range len(raw) {
		c := raw[i]
		if c <= 0x20 || c == 0x7f {
			continue
		}
		switch c {
		case ':':
			if scheme.Len() == 0 {
				return "", false
			}
			return strings.ToLower(scheme.String()), true
		case '/', '?', '#', '\\':
			return "", false
		}
		if !schemeByte(c, scheme.Len() == 0) {
			return "", false
		}
		scheme.WriteByte(c)
	}
	return "", false
}

func schemeByte(c byte, first bool) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	case first:
		return false
	case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
		return true
	}
	return false
}
