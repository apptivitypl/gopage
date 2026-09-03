package i18n

import "math"

type Form uint8

const (
	FormOther Form = iota
	FormZero
	FormOne
	FormTwo
	FormFew
	FormMany
)

var formNames = map[Form]string{
	FormOther: "other",
	FormZero:  "zero",
	FormOne:   "one",
	FormTwo:   "two",
	FormFew:   "few",
	FormMany:  "many",
}

var formsByName = buildFormIndex()

func buildFormIndex() map[string]Form {
	index := make(map[string]Form, len(formNames))
	for form, name := range formNames {
		index[name] = form
	}
	return index
}

func (f Form) String() string {
	return formNames[f]
}

func FormOf(name string) (Form, bool) {
	form, ok := formsByName[name]
	return form, ok
}

type Rule func(n float64) Form

var rules = map[string]Rule{
	"en": english,
	"de": english,
	"nl": english,
	"it": english,
	"es": romance,
	"pt": romance,
	"fr": french,
	"pl": polish,
	"cs": czech,
	"sk": czech,
	"ru": russian,
	"uk": russian,
	"ja": always,
	"zh": always,
	"ko": always,
	"vi": always,
	"tr": always,
}

func RuleFor(locale string) Rule {
	if rule, ok := rules[base(locale)]; ok {
		return rule
	}
	return english
}

func Localised(locale string) bool {
	_, ok := rules[base(locale)]
	return ok
}

func Locales() []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	return names
}

func base(locale string) string {
	for i := range len(locale) {
		if locale[i] == '-' || locale[i] == '_' {
			return locale[:i]
		}
	}
	return locale
}

func always(float64) Form {
	return FormOther
}

func english(n float64) Form {
	if whole(n) && n == 1 {
		return FormOne
	}
	return FormOther
}

func romance(n float64) Form {
	if n == 1 {
		return FormOne
	}
	return FormOther
}

func french(n float64) Form {
	if n >= 0 && n < 2 {
		return FormOne
	}
	return FormOther
}

func polish(n float64) Form {
	if !whole(n) {
		return FormOther
	}
	count := int64(math.Abs(n))
	last := count % 10
	lastTwo := count % 100
	switch {
	case count == 1:
		return FormOne
	case last >= 2 && last <= 4 && (lastTwo < 12 || lastTwo > 14):
		return FormFew
	default:
		return FormMany
	}
}

func czech(n float64) Form {
	if !whole(n) {
		return FormMany
	}
	count := int64(math.Abs(n))
	switch {
	case count == 1:
		return FormOne
	case count >= 2 && count <= 4:
		return FormFew
	default:
		return FormOther
	}
}

func russian(n float64) Form {
	if !whole(n) {
		return FormOther
	}
	count := int64(math.Abs(n))
	last := count % 10
	lastTwo := count % 100
	switch {
	case last == 1 && lastTwo != 11:
		return FormOne
	case last >= 2 && last <= 4 && (lastTwo < 12 || lastTwo > 14):
		return FormFew
	default:
		return FormMany
	}
}

func whole(n float64) bool {
	return n == math.Trunc(n) && !math.IsInf(n, 0)
}
