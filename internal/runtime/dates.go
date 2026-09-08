package runtime

import (
	"strconv"
	"strings"
	"time"
)

const (
	RelativeNow     = "time.now"
	RelativePrefix  = "time."
	relativeFuture  = "time.in_"
	layoutISO       = "iso"
	layoutDate      = "date"
	layoutTime      = "time"
	layoutDateTime  = "datetime"
	layoutHTTPStamp = "http"
)

var layouts = map[string]string{
	layoutISO:       time.RFC3339,
	layoutDate:      "2006-01-02",
	layoutTime:      "15:04",
	layoutDateTime:  "2006-01-02 15:04",
	layoutHTTPStamp: time.RFC1123,
}

var units = []struct {
	key  string
	span time.Duration
}{
	{"years", 365 * 24 * time.Hour},
	{"months", 30 * 24 * time.Hour},
	{"days", 24 * time.Hour},
	{"hours", time.Hour},
	{"minutes", time.Minute},
}

func RelativeKeys() []string {
	keys := make([]string, 0, len(units)*2+1)
	keys = append(keys, RelativeNow)
	for _, unit := range units {
		keys = append(keys, RelativePrefix+unit.key+"_ago", relativeFuture+unit.key)
	}
	return keys
}

func dated(env Env, value, argument Value) (Value, error) {
	moment, ok := momentOf(value)
	if !ok {
		return String(value.Text()), nil
	}
	layout := argument.Text()
	if named, known := layouts[strings.ToLower(layout)]; known {
		layout = named
	}
	if layout == "" {
		layout = time.RFC3339
	}
	return String(moment.In(env.Zone()).Format(layout)), nil
}

func relative(env Env, value, _ Value) (Value, error) {
	moment, ok := momentOf(value)
	if !ok {
		return String(value.Text()), nil
	}
	gap := env.Moment().Sub(moment)
	ahead := gap < 0
	if ahead {
		gap = -gap
	}
	for _, unit := range units {
		if gap < unit.span {
			continue
		}
		count := int(gap / unit.span)
		return String(phrase(env, unit.key, count, ahead)), nil
	}
	if text, ok := env.Text(RelativeNow, 0); ok {
		return String(text), nil
	}
	return String(RelativeNow), nil
}

func phrase(env Env, unit string, count int, ahead bool) string {
	key := RelativePrefix + unit + "_ago"
	if ahead {
		key = relativeFuture + unit
	}
	text, ok := env.Text(key, count)
	if !ok {
		return key
	}
	return strings.ReplaceAll(text, CountPlaceholder, strconv.Itoa(count))
}

func momentOf(value Value) (time.Time, bool) {
	if value.Kind == KindTime {
		return value.Moment(), true
	}
	if value.Kind != KindString {
		return time.Time{}, false
	}
	moment, err := time.Parse(time.RFC3339, value.Str)
	if err != nil {
		return time.Time{}, false
	}
	return moment, true
}
