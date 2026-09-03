package compile

import (
	"slices"

	"github.com/sonquer/rill/internal/action"
	"github.com/sonquer/rill/internal/form"
	"github.com/sonquer/rill/internal/runtime"
)

var metaFields = []string{"Title", "Description", "Canonical", "Image", "Robots", runtime.AlternatesField}

var formSections = []string{"Values", "Errors"}

var formFlags = []string{"Failed", form.TokenField}

var localeFields = []string{"Tag", "Dir", "Prefix", "Default"}

func RootPath(segments []string) bool {
	if len(segments) == 0 {
		return false
	}
	switch segments[0] {
	case action.FlashRoot:
		return len(segments) == 1
	case runtime.MetaRoot:
		return len(segments) == 2 && slices.Contains(metaFields, segments[1])
	case form.Root:
		return formPath(segments[1:])
	case runtime.LocaleRoot:
		return len(segments) == 2 && slices.Contains(localeFields, segments[1])
	default:
		return false
	}
}

func formPath(rest []string) bool {
	switch len(rest) {
	case 1:
		return slices.Contains(formFlags, rest[0])
	case 2:
		return slices.Contains(formSections, rest[0])
	default:
		return false
	}
}

func RootNames() []string {
	return []string{action.FlashRoot, form.Root, runtime.LocaleRoot, runtime.MetaRoot}
}
