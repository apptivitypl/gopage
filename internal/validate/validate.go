package validate

import (
	"fmt"
	"strconv"
	"strings"
)

const Tag = "validate"

type Kind uint8

const (
	KindString Kind = iota
	KindInt
	KindFloat
	KindBool
)

type Value struct {
	Kind    Kind
	Str     string
	Num     float64
	Bool    bool
	Present bool
}

type Violation struct {
	Field   string
	Rule    string
	Message string
}

type Constraint struct {
	Rule string
	Arg  string
}

type Checker func(Value, string) string

var checkers = map[string]Checker{
	"required": required,
	"len":      length,
	"min":      minimum,
	"max":      maximum,
	"email":    email,
	"accepted": accepted,
	"in":       oneOf,
}

func Known(rule string) bool {
	_, ok := checkers[rule]
	return ok
}

func Rules() []string {
	names := make([]string, 0, len(checkers))
	for name := range checkers {
		names = append(names, name)
	}
	return names
}

func Parse(tag string) ([]Constraint, error) {
	if strings.TrimSpace(tag) == "" {
		return nil, nil
	}
	var constraints []Constraint
	for part := range strings.SplitSeq(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rule, arg, _ := strings.Cut(part, ":")
		rule = strings.TrimSpace(rule)
		if !Known(rule) {
			return nil, fmt.Errorf("unknown validation rule %q", rule)
		}
		constraints = append(constraints, Constraint{Rule: rule, Arg: strings.TrimSpace(arg)})
	}
	return constraints, nil
}

func Check(name string, value Value, constraints []Constraint) []Violation {
	var violations []Violation
	for _, constraint := range constraints {
		if !value.Present && constraint.Rule != "required" && constraint.Rule != "accepted" {
			continue
		}
		if message := checkers[constraint.Rule](value, constraint.Arg); message != "" {
			violations = append(violations, Violation{Field: name, Rule: constraint.Rule, Message: message})
		}
	}
	return violations
}

func required(value Value, _ string) string {
	if !value.Present || (value.Kind == KindString && strings.TrimSpace(value.Str) == "") {
		return "this field is required"
	}
	return ""
}

func length(value Value, arg string) string {
	low, high, ok := bounds(arg)
	if !ok {
		return "the len rule needs bounds written as min..max"
	}
	size := float64(len([]rune(value.Str)))
	switch {
	case size < low:
		return fmt.Sprintf("this field needs at least %s characters", trim(low))
	case high > 0 && size > high:
		return fmt.Sprintf("this field takes at most %s characters", trim(high))
	default:
		return ""
	}
}

func minimum(value Value, arg string) string {
	limit, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return "the min rule needs a number"
	}
	if measure(value) < limit {
		return belowMessage(value, limit)
	}
	return ""
}

func maximum(value Value, arg string) string {
	limit, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return "the max rule needs a number"
	}
	if measure(value) > limit {
		return aboveMessage(value, limit)
	}
	return ""
}

func measure(value Value) float64 {
	if value.Kind == KindString {
		return float64(len([]rune(value.Str)))
	}
	return value.Num
}

func belowMessage(value Value, limit float64) string {
	if value.Kind == KindString {
		return fmt.Sprintf("this field needs at least %s characters", trim(limit))
	}
	return fmt.Sprintf("this field starts at %s", trim(limit))
}

func aboveMessage(value Value, limit float64) string {
	if value.Kind == KindString {
		return fmt.Sprintf("this field takes at most %s characters", trim(limit))
	}
	return fmt.Sprintf("this field stops at %s", trim(limit))
}

func email(value Value, _ string) string {
	address := strings.TrimSpace(value.Str)
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return "this does not look like an email address"
	}
	domain := address[at+1:]
	if strings.ContainsAny(address, " \t") || !strings.Contains(domain, ".") ||
		strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "this does not look like an email address"
	}
	return ""
}

func accepted(value Value, _ string) string {
	if !value.Bool {
		return "this box has to be ticked"
	}
	return ""
}

func oneOf(value Value, arg string) string {
	for candidate := range strings.SplitSeq(arg, "|") {
		if strings.TrimSpace(candidate) == value.Str {
			return ""
		}
	}
	return "this is not one of the accepted values"
}

func bounds(arg string) (float64, float64, bool) {
	low, high, found := strings.Cut(arg, "..")
	if !found {
		return 0, 0, false
	}
	from, err := strconv.ParseFloat(strings.TrimSpace(low), 64)
	if err != nil {
		return 0, 0, false
	}
	if strings.TrimSpace(high) == "" {
		return from, 0, true
	}
	to, err := strconv.ParseFloat(strings.TrimSpace(high), 64)
	if err != nil {
		return 0, 0, false
	}
	return from, to, true
}

func trim(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
