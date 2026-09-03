package compile

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/i18n"
	"github.com/apptivitypl/rill/internal/runtime"
)

func localised(page string, catalogs map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		"rill.jsonc":    &fstest.MapFile{Data: []byte("{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")},
		"app/page.rill": &fstest.MapFile{Data: []byte(page)},
	}
	for locale, text := range catalogs {
		fsys["locales/"+locale+".json"] = &fstest.MapFile{Data: []byte(text)}
	}
	return fsys
}

func compileLocalised(t *testing.T, page string, catalogs map[string]string) (Result, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	result, err := Compile(localised(page, catalogs), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return result, &bag
}

func renderLocale(t *testing.T, result Result, locale string, props runtime.Accessible) string {
	t.Helper()
	chain := result.Manifest.Chain(result.Manifest.Routes[0])
	out := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(out)
	catalog, _ := result.Manifest.Catalog(locale)
	opts := runtime.Options{Catalog: catalog, Plural: i18n.RuleFor(locale)}
	if err := runtime.RenderOptions(chain, props, out, opts); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestMessagesResolvePerLocale(t *testing.T) {
	result, bag := compileLocalised(t, `{{ t("hello") }}`, map[string]string{
		"en": `{"hello": "hello"}`,
		"pl": `{"hello": "czesc"}`,
	})
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	if got := renderLocale(t, result, "en", runtime.Empty{}); got != "hello" {
		t.Errorf("en = %q", got)
	}
	if got := renderLocale(t, result, "pl", runtime.Empty{}); got != "czesc" {
		t.Errorf("pl = %q", got)
	}
}

func TestPluralFormsFollowTheLocale(t *testing.T) {
	page := `{{ t("reviews", count = Count) }}`
	result, bag := compileLocalised(t, page, map[string]string{
		"en": "{\"reviews\": {\"one\": \"{count} review\", \"other\": \"{count} reviews\"}}",
		"pl": "{\"reviews\": {\"one\": \"{count} opinia\", \"few\": \"{count} opinie\", \"many\": \"{count} opinii\"}}",
	})
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	cases := []struct {
		locale string
		count  int64
		want   string
	}{
		{"en", 1, "1 review"}, {"en", 4, "4 reviews"},
		{"pl", 1, "1 opinia"}, {"pl", 3, "3 opinie"}, {"pl", 7, "7 opinii"}, {"pl", 22, "22 opinie"},
	}
	for _, c := range cases {
		got := renderLocale(t, result, c.locale, runtime.Map{"Count": runtime.Int(c.count)})
		if got != c.want {
			t.Errorf("%s(%d) = %q, want %q", c.locale, c.count, got, c.want)
		}
	}
}

func TestAMissingTranslationIsReported(t *testing.T) {
	_, bag := compileLocalised(t, `{{ t("hello") }}`, map[string]string{"en": `{"hello": "hello"}`, "pl": `{"bye": "pa"}`})
	if !hasCode(bag, diag.C601) {
		t.Fatalf("diagnostics = %v, want C601", bag.Sorted())
	}
	if !strings.Contains(bag.Sorted()[0].Help, "locales/pl.json") {
		t.Errorf("help = %q", bag.Sorted()[0].Help)
	}
}

func TestTheDefaultLocaleFillsInTheTextButNotTheError(t *testing.T) {
	result, bag := compileLocalised(t, `{{ t("hello") }}`, map[string]string{
		"en": `{"hello": "hello"}`,
		"pl": `{"goodbye": "pa"}`,
	})
	if !hasCode(bag, diag.C601) {
		t.Fatalf("diagnostics = %v, want C601", bag.Sorted())
	}
	if got := renderLocale(t, result, "pl", runtime.Empty{}); got != "hello" {
		t.Errorf("pl = %q, want the default locale text so the page still reads", got)
	}
}

func TestPluralMismatchesAreReported(t *testing.T) {
	cases := map[string]struct {
		page    string
		catalog string
	}{
		"counted but single": {`{{ t("reviews", count = Count) }}`, `{"reviews": "opinie"}`},
		"forms but no count": {`{{ t("reviews") }}`, "{\"reviews\": {\"one\": \"a\", \"few\": \"b\"}}"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := compileLocalised(t, c.page, map[string]string{"en": c.catalog, "pl": c.catalog})
			if !hasCode(bag, diag.C602) {
				t.Errorf("diagnostics = %v, want C602", bag.Sorted())
			}
		})
	}
}

func TestAProjectWithoutCatalogsFallsBackToTheKey(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(fstest.MapFS{
		"app/page.rill": &fstest.MapFile{Data: []byte(`{{ t("listing.title") }}`)},
	}, &bag)
	if err != nil || bag.HasErrors() {
		t.Fatalf("err = %v, diagnostics = %v", err, bag.Sorted())
	}
	chain := result.Manifest.Chain(result.Manifest.Routes[0])
	out := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(out)
	if err := runtime.Render(chain, runtime.Empty{}, out); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.String() != "listing.title" {
		t.Errorf("html = %q, want the key shown", out.String())
	}
}

func TestTheMessageTableIsDeduplicated(t *testing.T) {
	result, _ := compileLocalised(t, `{{ t("hello") }}{{ t("hello") }}{{ t("bye") }}`, map[string]string{
		"en": "{\"hello\": \"hi\", \"bye\": \"bye\"}",
		"pl": "{\"hello\": \"czesc\", \"bye\": \"pa\"}",
	})
	if !slices.Equal(result.Manifest.Messages, []string{"hello", "bye"}) {
		t.Errorf("messages = %v", result.Manifest.Messages)
	}
}

func TestOrphanKeysAreListed(t *testing.T) {
	var bag diag.Bag
	table := newMessageTable()
	table.intern("used")
	catalogs := map[string]i18n.Catalog{
		"pl": {Locale: "pl", Messages: map[string]i18n.Message{
			"used":   {Key: "used"},
			"unused": {Key: "unused"},
			"stale":  {Key: "stale"},
		}},
	}
	if got := Orphans(table, catalogs, "pl"); !slices.Equal(got, []string{"stale", "unused"}) {
		t.Errorf("orphans = %v", got)
	}
	if got := Orphans(table, catalogs, "de"); got != nil {
		t.Errorf("orphans = %v, want none for a locale with no catalog", got)
	}
	if bag.HasErrors() {
		t.Errorf("diagnostics = %v", bag.Sorted())
	}
}

func TestBuildCatalogsWithoutMessages(t *testing.T) {
	var bag diag.Bag
	if got := BuildCatalogs(newMessageTable(), nil, []string{"en"}, "en", true, &bag); got != nil {
		t.Errorf("catalogs = %v", got)
	}
}

func TestMalformedMessageCallsAreReported(t *testing.T) {
	cases := []string{
		`{{ t() }}`,
		`{{ t(hello) }}`,
		`{{ t("a", 3) }}`,
		`{{ t("a", count) }}`,
		`{{ t("a", count = ) }}`,
		`{{ t("a" }}`,
		`{{ len(Items }}`,
	}
	for _, source := range cases {
		var bag diag.Bag
		if _, err := Compile(fstest.MapFS{
			"app/page.rill": &fstest.MapFile{Data: []byte(source)},
		}, &bag); err != nil {
			t.Fatalf("Compile(%q): %v", source, err)
		}
		if !bag.HasErrors() {
			t.Errorf("%q was accepted", source)
		}
	}
}

func TestALegacyTomlCatalogIsReported(t *testing.T) {
	files := localised(`{{ t("hello") }}`, map[string]string{"en": `{"hello": "hello"}`, "pl": `{"hello": "czesc"}`})
	files["locales/de.toml"] = &fstest.MapFile{Data: []byte(`hello = "hallo"`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C603) {
		t.Fatalf("diagnostics = %v, want C603", bag.Sorted())
	}
	for _, item := range bag.Sorted() {
		if item.Code == diag.C603 && !strings.Contains(item.Help, "as json") {
			t.Errorf("help = %q, want the migration command", item.Help)
		}
	}
}

func TestKeysUsedFromTheGoBlockAreTranslated(t *testing.T) {
	files := localised("<h1>{{ Heading }}</h1>", map[string]string{
		"en": `{"hero": {"title": "hello"}}`,
		"pl": `{"hero": {"title": "czesc"}}`,
	})
	files["app/page.rill"] = &fstest.MapFile{Data: []byte(`---
type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{Heading: ctx.T("hero.title")}, nil
}
---
<h1>{{ Heading }}</h1>`)}
	var bag diag.Bag
	result, err := Compile(files, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics = %v", bag.Sorted())
	}
	if !slices.Contains(result.Manifest.Messages, "hero.title") {
		t.Fatalf("messages = %v, want the key used from go", result.Manifest.Messages)
	}
	catalog, ok := result.Manifest.Catalog("pl")
	if !ok {
		t.Fatal("the polish catalog is missing")
	}
	text, _ := catalog.Text(0, int(i18n.FormOther))
	if text != "czesc" {
		t.Errorf("pl text = %q, want the translation", text)
	}
}

func TestAGoKeyMissingFromACatalogIsReported(t *testing.T) {
	files := localised("<h1>{{ Heading }}</h1>", map[string]string{
		"en": `{"hero": {"title": "hello"}}`,
		"pl": `{"other": "x"}`,
	})
	files["app/page.rill"] = &fstest.MapFile{Data: []byte(`---
type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{Heading: ctx.T("hero.title")}, nil
}
---
<h1>{{ Heading }}</h1>`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !hasCode(&bag, diag.C601) {
		t.Errorf("diagnostics = %v, want C601 for the key used from go", bag.Sorted())
	}
}

func TestGoMessagesReadsBothHelpers(t *testing.T) {
	found := GoMessages(`func Load(ctx *rill.Ctx) (Props, error) {
	_ = ctx.T("plain.key")
	_ = ctx.Count("counted.key", 3)
	_ = ctx.T(variable)
	_ = other.Method("not.a.key")
	return Props{}, nil
}`)
	keys := map[string]bool{}
	for _, message := range found {
		keys[message.Key] = message.Plural
	}
	if plural, ok := keys["plain.key"]; !ok || plural {
		t.Errorf("plain.key = %v, ok = %v", plural, ok)
	}
	if plural, ok := keys["counted.key"]; !ok || !plural {
		t.Errorf("counted.key = %v, ok = %v", plural, ok)
	}
	if _, ok := keys["not.a.key"]; ok {
		t.Error("a call on another receiver must not register a key")
	}
	if len(keys) != 2 {
		t.Errorf("keys = %v", keys)
	}
}

func TestGoMessagesIgnoresCodeItCannotParse(t *testing.T) {
	if got := GoMessages("func Load( {"); got != nil {
		t.Errorf("GoMessages = %v, want nothing from unparsable code", got)
	}
	if got := GoMessages(""); got != nil {
		t.Errorf("GoMessages = %v, want nothing from an empty block", got)
	}
}
