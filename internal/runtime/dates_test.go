package runtime

import (
	"sort"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/i18n"
	"github.com/apptivitypl/gopage/internal/ir"
)

func moment(t *testing.T, text string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return parsed
}

func TestATimeRendersAsRFC3339(t *testing.T) {
	value := Time(moment(t, "2026-09-08T10:30:00Z"))
	if got := value.Text(); got != "2026-09-08T10:30:00Z" {
		t.Errorf("text = %q", got)
	}
	if !value.Truthy() {
		t.Error("a real moment is truthy")
	}
}

func TestTheDateFilterFollowsItsLayout(t *testing.T) {
	value := Time(moment(t, "2026-09-08T10:30:00Z"))
	cases := map[string]string{
		"":              "2026-09-08T10:30:00Z",
		"iso":           "2026-09-08T10:30:00Z",
		"date":          "2026-09-08",
		"time":          "10:30",
		"datetime":      "2026-09-08 10:30",
		"02.01.2006":    "08.09.2026",
		"Monday, 2 Jan": "Tuesday, 8 Sep",
	}
	for layout, want := range cases {
		got, err := dated(Env{}, value, String(layout))
		if err != nil || got.Str != want {
			t.Errorf("date(%q) = %q, err = %v, want %q", layout, got.Str, err, want)
		}
	}
}

func TestTheDateFilterReadsAnIsoString(t *testing.T) {
	got, err := dated(Env{}, String("2026-09-08T10:30:00Z"), String("date"))
	if err != nil || got.Str != "2026-09-08" {
		t.Errorf("date = %q, err = %v", got.Str, err)
	}
	if got, _ := dated(Env{}, String("not a date"), String("date")); got.Str != "not a date" {
		t.Errorf("a value that is no date passes through: %q", got.Str)
	}
}

func TestTheDateFilterHonoursTheZone(t *testing.T) {
	zone := time.FixedZone("test", 2*60*60)
	got, err := dated(Env{Location: zone}, Time(moment(t, "2026-09-08T10:30:00Z")), String("time"))
	if err != nil || got.Str != "12:30" {
		t.Errorf("time = %q, err = %v", got.Str, err)
	}
}

func relativeEnv(now string, texts map[string]string, t *testing.T) Env {
	t.Helper()
	keys := make([]string, 0, len(texts))
	for key := range texts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	catalog := &ir.Catalog{Locale: "en", Texts: make([][ir.PluralForms]string, len(keys))}
	for index, key := range keys {
		catalog.Texts[index][i18n.FormOther] = texts[key]
	}
	return Env{
		Now:     func() time.Time { return moment(t, now) },
		Plan:    &ir.Plan{Messages: keys},
		Catalog: catalog,
	}
}

func TestRelativeNamesTheUnit(t *testing.T) {
	texts := map[string]string{
		"time.now":         "just now",
		"time.minutes_ago": "{count} minutes ago",
		"time.hours_ago":   "{count} hours ago",
		"time.days_ago":    "{count} days ago",
		"time.months_ago":  "{count} months ago",
		"time.years_ago":   "{count} years ago",
		"time.in_days":     "in {count} days",
	}
	env := relativeEnv("2026-09-08T12:00:00Z", texts, t)
	cases := map[string]string{
		"2026-09-08T11:59:30Z": "just now",
		"2026-09-08T11:30:00Z": "30 minutes ago",
		"2026-09-08T09:00:00Z": "3 hours ago",
		"2026-09-05T12:00:00Z": "3 days ago",
		"2026-06-08T12:00:00Z": "3 months ago",
		"2024-09-08T12:00:00Z": "2 years ago",
		"2026-09-11T12:00:00Z": "in 3 days",
	}
	for when, want := range cases {
		got, err := relative(env, Time(moment(t, when)), Nil())
		if err != nil || got.Str != want {
			t.Errorf("relative(%s) = %q, err = %v, want %q", when, got.Str, err, want)
		}
	}
}

func TestRelativeFallsBackToTheKey(t *testing.T) {
	env := Env{Now: func() time.Time { return moment(t, "2026-09-08T12:00:00Z") }}
	got, _ := relative(env, Time(moment(t, "2026-09-05T12:00:00Z")), Nil())
	if got.Str != "time.days_ago" {
		t.Errorf("relative = %q", got.Str)
	}
	if now, _ := relative(env, Time(moment(t, "2026-09-08T11:59:59Z")), Nil()); now.Str != "time.now" {
		t.Errorf("relative = %q", now.Str)
	}
	if passed, _ := relative(env, String("nonsense"), Nil()); passed.Str != "nonsense" {
		t.Errorf("relative = %q", passed.Str)
	}
}

func TestTheRelativeKeysAreListed(t *testing.T) {
	keys := RelativeKeys()
	if len(keys) != 11 || keys[0] != RelativeNow {
		t.Errorf("keys = %v", keys)
	}
}

func TestAnEnvWithoutAClockUsesTheWallClock(t *testing.T) {
	env := Env{}
	if env.Moment().IsZero() || env.Zone() != time.UTC {
		t.Error("an empty env still answers")
	}
	if _, ok := env.Text("time.now", 0); ok {
		t.Error("an empty env translates nothing")
	}
}

func TestAPlanTranslatesForAFilter(t *testing.T) {
	plan := &ir.Plan{
		Messages: []string{"time.days_ago"},
		Ops:      []ir.Op{{Kind: ir.OpText, A: 1}},
		Exprs: []ir.ExprNode{
			{Kind: ir.ExprPath, A: 0},
			{Kind: ir.ExprFilter, Op: relativeID(t), A: 0, B: NoArgument},
		},
		Paths:    [][]string{{"Posted"}},
		Capacity: 64,
	}
	catalog := &ir.Catalog{Locale: "en", Texts: [][ir.PluralForms]string{{i18n.FormOther: "{count} days ago"}}}
	out := NewBuffer(64)
	opts := Options{
		Catalog: catalog,
		Plural:  i18n.RuleFor("en"),
		Now:     func() time.Time { return moment(t, "2026-09-08T12:00:00Z") },
	}
	props := Map{"Posted": Time(moment(t, "2026-09-05T12:00:00Z"))}
	if err := RenderOptions([]*ir.Plan{plan}, props, out, opts); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := string(out.Bytes()); got != "3 days ago" {
		t.Errorf("render = %q", got)
	}
}

func relativeID(t *testing.T) uint8 {
	t.Helper()
	id, _, ok := LookupFilter("relative")
	if !ok {
		t.Fatal("the relative filter is missing")
	}
	return uint8(id)
}

func TestATimeWithoutAZoneReadsAsUTC(t *testing.T) {
	value := Value{Kind: KindTime, num: 0}
	if got := value.Moment().Location(); got != time.UTC {
		t.Errorf("zone = %v", got)
	}
}

func TestOnlyAStringOrATimeIsAMoment(t *testing.T) {
	if _, ok := momentOf(Int(3)); ok {
		t.Error("a number is not a moment")
	}
}

func TestTranslationNeedsTheKeyInThePlan(t *testing.T) {
	env := Env{Plan: &ir.Plan{Messages: []string{"other"}}, Catalog: &ir.Catalog{}}
	if _, ok := env.Text("time.now", 0); ok {
		t.Error("a key outside the plan translates to nothing")
	}
	if _, ok := (Env{}).Text("time.now", 0); ok {
		t.Error("an empty env translates nothing")
	}
	if got := (Env{Plural: i18n.RuleFor("en")}).form(2); got != i18n.FormOther {
		t.Errorf("form = %v", got)
	}
}

func TestAnEnvWithoutACatalogTranslatesNothing(t *testing.T) {
	env := Env{Plan: &ir.Plan{Messages: []string{"time.now"}}}
	if _, ok := env.Text("time.now", 0); ok {
		t.Error("no catalog, no text")
	}
}
