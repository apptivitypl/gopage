package server

import (
	"context"
	"github.com/apptivitypl/gopage/internal/runtime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/i18n"
	"github.com/apptivitypl/gopage/internal/ir"
)

func translated(t *testing.T, locale string) Translator {
	t.Helper()
	manifest := &ir.Manifest{
		Messages: []string{"hello", "items"},
		Catalogs: []ir.Catalog{
			{Locale: "en", Texts: [][ir.PluralForms]string{
				{i18n.FormOther: "hello"},
				{i18n.FormOne: "{count} item", i18n.FormOther: "{count} items"},
			}},
			{Locale: "pl", Texts: [][ir.PluralForms]string{
				{i18n.FormOther: "czesc"},
				{i18n.FormOne: "{count} rzecz", i18n.FormFew: "{count} rzeczy", i18n.FormOther: "{count} rzeczy"},
			}},
		},
	}
	app := New(Options{Manifest: manifest})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if locale != "" {
		request = request.WithContext(context.WithValue(request.Context(), localeKey{}, locale))
	}
	return app.translator(request)
}

func TestTheTranslatorAnswersInTheRequestLocale(t *testing.T) {
	if got := translated(t, "pl")("hello", 0, false); got != "czesc" {
		t.Errorf("pl hello = %q", got)
	}
	if got := translated(t, "en")("hello", 0, false); got != "hello" {
		t.Errorf("en hello = %q", got)
	}
}

func TestTheTranslatorPicksThePluralFormAndFillsTheCount(t *testing.T) {
	if got := translated(t, "pl")("items", 3, true); got != "3 rzeczy" {
		t.Errorf("pl items(3) = %q, want the few form with the count filled in", got)
	}
	if got := translated(t, "en")("items", 1, true); got != "1 item" {
		t.Errorf("en items(1) = %q", got)
	}
}

func TestAnUnknownKeyComesBackAsItself(t *testing.T) {
	if got := translated(t, "pl")("absent.key", 0, false); got != "absent.key" {
		t.Errorf("absent = %q, want the key", got)
	}
}

func TestALocaleWithoutACatalogFallsBackToTheKey(t *testing.T) {
	if got := translated(t, "de")("hello", 0, false); got != "hello" {
		t.Errorf("de hello = %q, want the key when the locale has no catalog", got)
	}
}

func TestAContextWithoutATranslatorReturnsTheKey(t *testing.T) {
	if got := TranslatorFrom(context.Background())("hello", 0, false); got != "hello" {
		t.Errorf("translator = %q, want the key", got)
	}
}

func TestTheTranslatorTravelsInTheContext(t *testing.T) {
	ctx := WithTranslator(context.Background(), func(key string, _ int, _ bool) string { return "seen:" + key })
	if got := TranslatorFrom(ctx)("hello", 0, false); got != "seen:hello" {
		t.Errorf("translator = %q", got)
	}
}

func TestTheTranslatorFallsBackToTheOtherForm(t *testing.T) {
	if got := translated(t, "pl")("items", 7, true); got != "7 rzeczy" {
		t.Errorf("pl items(7) = %q, want the other form when many is absent", got)
	}
}

func TestAManifestWithoutCatalogsAnswersWithKeys(t *testing.T) {
	app := New(Options{Manifest: &ir.Manifest{Messages: []string{"hello"}}})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := app.translator(request)("hello", 0, false); got != "hello" {
		t.Errorf("translator = %q, want the key", got)
	}
}

func TestPublicPathsAreRoutedToTheAssetHandler(t *testing.T) {
	served := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("asset:" + r.URL.Path))
	})
	app := New(Options{
		Manifest: &ir.Manifest{},
		Assets:   served,
		Public:   []string{"/favicon.ico"},
	})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if got := recorder.Body.String(); got != "asset:/favicon.ico" {
		t.Errorf("body = %q, want the asset handler to answer", got)
	}
}

func TestAKeyWithNoTextAtAllComesBackAsTheKey(t *testing.T) {
	manifest := &ir.Manifest{
		Messages: []string{"empty"},
		Catalogs: []ir.Catalog{{Locale: "pl", Texts: [][ir.PluralForms]string{{}}}},
	}
	app := New(Options{Manifest: manifest})
	request := httptest.NewRequest(http.MethodGet, "/", nil).
		WithContext(context.WithValue(context.Background(), localeKey{}, "pl"))
	if got := app.translator(request)("empty", 0, false); got != "empty" {
		t.Errorf("translator = %q, want the key when the catalog holds no text", got)
	}
}

func TestAMessageBeyondTheCatalogComesBackAsTheKey(t *testing.T) {
	manifest := &ir.Manifest{
		Messages: []string{"first", "second"},
		Catalogs: []ir.Catalog{{Locale: "pl", Texts: [][ir.PluralForms]string{{i18n.FormOther: "pierwszy"}}}},
	}
	app := New(Options{Manifest: manifest})
	request := httptest.NewRequest(http.MethodGet, "/", nil).
		WithContext(context.WithValue(context.Background(), localeKey{}, "pl"))
	if got := app.translator(request)("second", 0, false); got != "second" {
		t.Errorf("translator = %q, want the key for a message the catalog does not reach", got)
	}
}

func TestAFallbackPageIsLocalised(t *testing.T) {
	base := withFallbacks(manifest())
	base.Messages = []string{"missing.lead"}
	base.Catalogs = []ir.Catalog{{Locale: "en", Texts: [][ir.PluralForms]string{{i18n.FormOther: "nothing lives here"}}}}
	base.Plans[3] = ir.Plan{
		Ops:      []ir.Op{{Kind: ir.OpText, A: 0}},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprMessage, A: 0, B: runtime.NoArgument}},
		Messages: []string{"missing.lead"},
		Capacity: 16,
	}
	app := New(Options{Manifest: base})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/absent", nil))
	if got := recorder.Body.String(); !strings.Contains(got, "nothing lives here") {
		t.Errorf("body = %q, want the translated fallback", got)
	}
}
