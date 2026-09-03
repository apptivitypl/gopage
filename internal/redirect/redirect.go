package redirect

import (
	"net/url"
	"strings"
)

func Path(target string) (string, bool) {
	if !rooted(target) {
		return "", false
	}
	return target, true
}

func Location(target string) (string, bool) {
	if rooted(target) {
		return target, true
	}
	if !plain(target) {
		return "", false
	}
	held, err := url.Parse(target)
	if err != nil || held.Host == "" {
		return "", false
	}
	if held.Scheme != "http" && held.Scheme != "https" {
		return "", false
	}
	return target, true
}

func Safe(target string, allowed []string) (string, bool) {
	if location, ok := Path(target); ok {
		return location, true
	}
	if len(allowed) == 0 {
		return "", false
	}
	location, ok := Location(target)
	if !ok {
		return "", false
	}
	held, err := url.Parse(location)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(held.Host)
	for _, entry := range allowed {
		if strings.ToLower(entry) == host {
			return location, true
		}
	}
	return "", false
}

func rooted(target string) bool {
	if target == "" || target[0] != '/' || !plain(target) {
		return false
	}
	return len(target) == 1 || (target[1] != '/' && target[1] != '\\')
}

func plain(target string) bool {
	for index := range len(target) {
		if target[index] < 0x20 || target[index] == 0x7f {
			return false
		}
	}
	return true
}
