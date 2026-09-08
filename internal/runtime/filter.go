package runtime

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/apptivitypl/gopage/internal/i18n"
	"github.com/apptivitypl/gopage/internal/ir"
)

const NoArgument = ^uint32(0)

type Env struct {
	Now      func() time.Time
	Location *time.Location
	Plan     *ir.Plan
	Catalog  *ir.Catalog
	Plural   i18n.Rule
}

func (e Env) Moment() time.Time {
	if e.Now == nil {
		return time.Now()
	}
	return e.Now()
}

func (e Env) Zone() *time.Location {
	if e.Location == nil {
		return time.UTC
	}
	return e.Location
}

func (e Env) Text(key string, count int) (string, bool) {
	if e.Plan == nil || e.Catalog == nil {
		return "", false
	}
	index, ok := e.Plan.MessageIndex(key)
	if !ok {
		return "", false
	}
	return e.Catalog.Text(index, int(e.form(count)))
}

func (e Env) form(count int) i18n.Form {
	if e.Plural == nil {
		return i18n.FormOther
	}
	return e.Plural(float64(count))
}

type Filter struct {
	Name  string
	Arity int
	Apply func(Env, Value, Value) (Value, error)
}

var filters = []Filter{
	{Name: "upper", Apply: func(_ Env, v, _ Value) (Value, error) { return String(strings.ToUpper(v.Text())), nil }},
	{Name: "lower", Apply: func(_ Env, v, _ Value) (Value, error) { return String(strings.ToLower(v.Text())), nil }},
	{Name: "trim", Apply: func(_ Env, v, _ Value) (Value, error) { return String(strings.TrimSpace(v.Text())), nil }},
	{Name: "default", Arity: 1, Apply: fallback},
	{Name: "truncate", Arity: 1, Apply: truncate},
	{Name: "number", Apply: func(_ Env, v, _ Value) (Value, error) { return String(group(v)), nil }},
	{Name: "money", Arity: 1, Apply: money},
	{Name: "len", Apply: count},
	{Name: "date", Arity: 1, Apply: dated},
	{Name: "relative", Apply: relative},
}

var filterIndex = buildFilterIndex()

func buildFilterIndex() map[string]uint32 {
	index := make(map[string]uint32, len(filters))
	for i, filter := range filters {
		index[filter.Name] = uint32(i)
	}
	return index
}

func LookupFilter(name string) (uint32, Filter, bool) {
	id, ok := filterIndex[name]
	if !ok {
		return 0, Filter{}, false
	}
	return id, filters[id], true
}

func FilterNames() []string {
	names := make([]string, 0, len(filters))
	for _, filter := range filters {
		names = append(names, filter.Name)
	}
	return names
}

func ApplyFilter(env Env, id uint32, value, argument Value) (Value, error) {
	if int(id) >= len(filters) {
		return Nil(), fmt.Errorf("plan uses an unknown filter %d", id)
	}
	return filters[id].Apply(env, value, argument)
}

func count(_ Env, value, _ Value) (Value, error) {
	switch value.Kind {
	case KindSeq:
		return Int(int64(value.Sequence().Len())), nil
	case KindString:
		return Int(int64(len([]rune(value.Str)))), nil
	case KindNil:
		return Int(0), nil
	default:
		return Nil(), fmt.Errorf("len needs a list or a string, got %s", value.Text())
	}
}

func fallback(_ Env, value, argument Value) (Value, error) {
	if value.Kind == KindNil || (value.Kind == KindString && value.Str == "") {
		return argument, nil
	}
	return value, nil
}

func truncate(_ Env, value, argument Value) (Value, error) {
	limit := int(argument.Int())
	if argument.Kind != KindInt || limit < 0 {
		return Nil(), fmt.Errorf("truncate needs a whole number, got %s", argument.Text())
	}
	runes := []rune(value.Text())
	if len(runes) <= limit {
		return String(string(runes)), nil
	}
	return String(string(runes[:limit]) + "…"), nil
}

func money(_ Env, value, argument Value) (Value, error) {
	amount, ok := value.Number()
	if !ok {
		return Nil(), fmt.Errorf("money needs a number, got %s", value.Text())
	}
	text := strconv.FormatFloat(amount, 'f', 2, 64)
	whole, fraction, _ := strings.Cut(text, ".")
	formatted := separate(whole) + "." + fraction
	if code := argument.Text(); code != "" {
		return String(formatted + " " + code), nil
	}
	return String(formatted), nil
}

func group(value Value) string {
	if value.IsInt() {
		return separate(strconv.FormatInt(value.Int(), 10))
	}
	number, ok := value.Number()
	if !ok {
		return value.Text()
	}
	whole, fraction, found := strings.Cut(strconv.FormatFloat(number, 'f', -1, 64), ".")
	if !found {
		return separate(whole)
	}
	return separate(whole) + "." + fraction
}

func separate(digits string) string {
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	if len(digits) <= 3 {
		return sign + digits
	}
	var b strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
	}
	for i := lead; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(digits[i : i+3])
	}
	return sign + b.String()
}
