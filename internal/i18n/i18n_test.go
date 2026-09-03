package i18n

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEnglishForms(t *testing.T) {
	rule := RuleFor("en")
	cases := map[float64]Form{0: FormOther, 1: FormOne, 2: FormOther, 1.5: FormOther}
	for n, want := range cases {
		if got := rule(n); got != want {
			t.Errorf("en(%v) = %s, want %s", n, got, want)
		}
	}
}

func TestPolishForms(t *testing.T) {
	rule := RuleFor("pl")
	cases := map[float64]Form{
		1: FormOne, 2: FormFew, 3: FormFew, 4: FormFew,
		5: FormMany, 11: FormMany, 12: FormMany, 14: FormMany,
		22: FormFew, 25: FormMany, 0: FormMany, 1.5: FormOther,
	}
	for n, want := range cases {
		if got := rule(n); got != want {
			t.Errorf("pl(%v) = %s, want %s", n, got, want)
		}
	}
}

func TestOtherFamilies(t *testing.T) {
	cases := []struct {
		locale string
		n      float64
		want   Form
	}{
		{"fr", 0, FormOne}, {"fr", 1, FormOne}, {"fr", 2, FormOther},
		{"es", 1, FormOne}, {"es", 0, FormOther},
		{"cs", 1, FormOne}, {"cs", 3, FormFew}, {"cs", 9, FormOther}, {"cs", 1.5, FormMany},
		{"ru", 1, FormOne}, {"ru", 11, FormMany}, {"ru", 3, FormFew}, {"ru", 5, FormMany}, {"ru", 1.5, FormOther},
		{"ja", 1, FormOther}, {"ja", 5, FormOther},
		{"xx", 1, FormOne},
	}
	for _, c := range cases {
		if got := RuleFor(c.locale)(c.n); got != c.want {
			t.Errorf("%s(%v) = %s, want %s", c.locale, c.n, got, c.want)
		}
	}
}

func TestRegionalTagsShareTheBaseRule(t *testing.T) {
	if RuleFor("pt-BR")(2) != FormOther || RuleFor("pl_PL")(2) != FormFew {
		t.Error("a regional tag must use the base language rule")
	}
	if !Localised("pl-PL") || Localised("qq") {
		t.Error("Localised must answer for known bases only")
	}
	if !slices.Contains(Locales(), "pl") {
		t.Errorf("locales = %v", Locales())
	}
}

func TestFormNames(t *testing.T) {
	for _, name := range []string{"zero", "one", "two", "few", "many", "other"} {
		form, ok := FormOf(name)
		if !ok || form.String() != name {
			t.Errorf("%q round trip = %s, ok = %v", name, form, ok)
		}
	}
	if _, ok := FormOf("several"); ok {
		t.Error("several is not a plural form")
	}
}

func parseCatalog(t *testing.T, text string) Catalog {
	t.Helper()
	catalog, err := Parse("pl", text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return catalog
}

func TestFlatAndNestedKeys(t *testing.T) {
	catalog := parseCatalog(t, `{
  "greeting": "czesc",
  "listing": {
    "title": "oferta",
    "status": { "active": "aktywna" }
  }
}`)
	want := []string{"greeting", "listing.status.active", "listing.title"}
	if got := catalog.Keys(); !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	text, ok := catalog.Messages["listing.status.active"].Text(FormOther)
	if !ok || text != "aktywna" {
		t.Errorf("text = %q, ok = %v", text, ok)
	}
}

func TestPluralTables(t *testing.T) {
	catalog := parseCatalog(t, `{
  "reviews": {
    "one": "jedna opinia",
    "few": "%d opinie",
    "many": "%d opinii"
  }
}`)
	message := catalog.Messages["reviews"]
	if !message.Plural() {
		t.Error("a table of forms is a plural message")
	}
	if text, _ := message.Text(FormFew); text != "%d opinie" {
		t.Errorf("few = %q", text)
	}
	if _, ok := message.Text(FormOther); ok {
		t.Error("this catalog declares no other form")
	}
}

func TestASingleMessageIsNotPlural(t *testing.T) {
	catalog := parseCatalog(t, `{"hello": "czesc"}`)
	if catalog.Messages["hello"].Plural() {
		t.Error("a plain string is not a plural message")
	}
	if text, ok := catalog.Messages["hello"].Text(FormMany); !ok || text != "czesc" {
		t.Errorf("an unknown form falls back to other: %q", text)
	}
}

func TestATableOfOtherOnlyIsNotPlural(t *testing.T) {
	catalog := parseCatalog(t, `{"hello": {"other": "czesc"}}`)
	if catalog.Messages["hello"].Plural() {
		t.Error("a table holding only other is a plain message")
	}
}

func TestMalformedCatalogsAreReported(t *testing.T) {
	cases := map[string]string{
		"broken json": "{unclosed",
		"wrong type":  `{"count": 7}`,
		"nested type": `{"listing": {"count": 7}}`,
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("pl", text); err == nil || !strings.Contains(err.Error(), "pl.json") {
				t.Errorf("err = %v, want the file named", err)
			}
		})
	}
}

func TestLoadReadsEveryCatalog(t *testing.T) {
	catalogs, err := Load(fstest.MapFS{
		"locales/en.json":  &fstest.MapFile{Data: []byte(`{"hello": "hi"}`)},
		"locales/pl.json":  &fstest.MapFile{Data: []byte(`{"hello": "czesc"}`)},
		"locales/readme":   &fstest.MapFile{Data: []byte("ignored")},
		"locales/sub/x.go": &fstest.MapFile{Data: []byte("ignored")},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(catalogs) != 2 || catalogs["pl"].Messages["hello"].Forms[FormOther] != "czesc" {
		t.Errorf("catalogs = %+v", catalogs)
	}
}

func TestLoadWithoutCatalogsIsNotAnError(t *testing.T) {
	catalogs, err := Load(fstest.MapFS{"app/page.rill": &fstest.MapFile{Data: []byte("x")}})
	if err != nil || len(catalogs) != 0 {
		t.Errorf("catalogs = %v, err = %v", catalogs, err)
	}
}

func TestABrokenCatalogStopsLoading(t *testing.T) {
	if _, err := Load(fstest.MapFS{"locales/pl.json": &fstest.MapFile{Data: []byte("[unclosed")}}); err == nil {
		t.Error("a catalog that does not parse must be reported")
	}
}

func TestAFormHoldingSomethingElseIsNotAPluralTable(t *testing.T) {
	if _, err := Parse("pl", "[reviews]\none = 7\n"); err == nil {
		t.Error("a plural form must hold a string")
	}
}

type unreadableFS struct {
	inner  fs.FS
	broken string
}

func (u unreadableFS) Open(name string) (fs.File, error) {
	if name == u.broken {
		return nil, fs.ErrPermission
	}
	return u.inner.Open(name)
}

func TestAnUnreadableCatalogIsReported(t *testing.T) {
	fsys := unreadableFS{
		inner:  fstest.MapFS{"locales/pl.json": &fstest.MapFile{Data: []byte(`hello = "czesc"`)}},
		broken: "locales/pl.json",
	}
	if _, err := Load(fsys); err == nil {
		t.Error("a catalog that cannot be read must be reported")
	}
}

func TestAuditCountsAndLists(t *testing.T) {
	catalogs := map[string]Catalog{
		"en": {Locale: "en", Messages: map[string]Message{"a": {Key: "a"}, "b": {Key: "b"}, "old": {Key: "old"}}},
		"pl": {Locale: "pl", Messages: map[string]Message{"a": {Key: "a"}}},
	}
	reports := Audit([]string{"a", "b"}, catalogs, []string{"en", "pl", "de"})
	if len(reports) != 3 {
		t.Fatalf("reports = %+v", reports)
	}
	if !reports[0].Complete() || reports[0].Percent() != 100 {
		t.Errorf("en = %+v", reports[0])
	}
	if !slices.Equal(reports[0].Orphans, []string{"old"}) {
		t.Errorf("orphans = %v", reports[0].Orphans)
	}
	if reports[1].Complete() || !slices.Equal(reports[1].Missing, []string{"b"}) {
		t.Errorf("pl = %+v", reports[1])
	}
	if reports[1].Percent() != 50 {
		t.Errorf("percent = %v", reports[1].Percent())
	}
	if reports[2].Translated != 0 || len(reports[2].Missing) != 2 {
		t.Errorf("de = %+v", reports[2])
	}
}

func TestAnEmptyProjectIsComplete(t *testing.T) {
	reports := Audit(nil, nil, []string{"en"})
	if !reports[0].Complete() || reports[0].Percent() != 100 {
		t.Errorf("report = %+v", reports[0])
	}
}

func TestSnippetStartsFromTheDefaultLocale(t *testing.T) {
	source := parseCatalog(t, `{"hello": "hello", "reviews": {"one": "one review", "other": "{count} reviews"}}`)
	snippet := Snippet([]string{"reviews", "hello", "unknown.key"}, source)
	for _, want := range []string{`"hello": "hello"`, `"reviews": {`, `"one": "one review"`, `"unknown.key"`} {
		if !strings.Contains(snippet, want) {
			t.Errorf("snippet = %q, want %q", snippet, want)
		}
	}
	if Snippet(nil, source) != "" {
		t.Error("nothing missing means no snippet")
	}
}

func TestSnippetIsValidJSONWithNestedKeys(t *testing.T) {
	snippet := Snippet([]string{"a.b", "with space", "ok-key"}, Catalog{})
	catalog, err := Parse("pl", snippet)
	if err != nil {
		t.Fatalf("the snippet must parse as a catalog: %v", err)
	}
	for _, key := range []string{"a.b", "with space", "ok-key"} {
		if _, ok := catalog.Messages[key]; !ok {
			t.Errorf("snippet = %s, key %q is missing", snippet, key)
		}
	}
}

func TestLegaciesNamesTomlCatalogs(t *testing.T) {
	names := Legacies(fstest.MapFS{
		"locales/en.json": &fstest.MapFile{Data: []byte(`{"a": "b"}`)},
		"locales/pl.toml": &fstest.MapFile{Data: []byte(`a = "b"`)},
		"locales/de.toml": &fstest.MapFile{Data: []byte(`a = "b"`)},
		"locales/sub":     &fstest.MapFile{Mode: fs.ModeDir},
	})
	if !slices.Equal(names, []string{"locales/de.toml", "locales/pl.toml"}) {
		t.Errorf("names = %v", names)
	}
}

func TestLegaciesOnAProjectWithoutCatalogs(t *testing.T) {
	if names := Legacies(fstest.MapFS{"app/page.rill": &fstest.MapFile{Data: []byte("x")}}); names != nil {
		t.Errorf("names = %v, want none", names)
	}
}

func TestAPluralTableWithANumberIsNotPlural(t *testing.T) {
	if _, err := Parse("pl", `{"reviews": {"one": 7}}`); err == nil {
		t.Error("a form holding a number must be reported")
	}
}

func TestSnippetOverwritesAConflictingBranch(t *testing.T) {
	snippet := Snippet([]string{"nav", "nav.home"}, Catalog{})
	if _, err := Parse("pl", snippet); err != nil {
		t.Errorf("the snippet must stay valid json: %v (%s)", err, snippet)
	}
	if !strings.Contains(snippet, `"home"`) {
		t.Errorf("snippet = %s, want the nested key to win", snippet)
	}
}
