package runtime

import "strings"

const LocaleRoot = "locale"

var rightToLeft = map[string]bool{"ar": true, "fa": true, "he": true, "ur": true}

type Locale struct {
	Tag     string
	Default bool
	Prefix  string
}

func (l Locale) Dir() string {
	if rightToLeft[base(l.Tag)] {
		return "rtl"
	}
	return "ltr"
}

func base(tag string) string {
	if cut := strings.IndexAny(tag, "-_"); cut > 0 {
		return strings.ToLower(tag[:cut])
	}
	return strings.ToLower(tag)
}

func (l Locale) Get(path []string) (Value, bool) {
	if len(path) != 1 {
		return Nil(), false
	}
	switch path[0] {
	case "Tag":
		return String(l.Tag), true
	case "Dir":
		return String(l.Dir()), true
	case "Prefix":
		return String(l.Prefix), true
	case "Default":
		return Bool(l.Default), true
	default:
		return Nil(), false
	}
}

func WithLocale(props Accessible, locale Locale) Accessible {
	return WithRoot(props, LocaleRoot, locale)
}
