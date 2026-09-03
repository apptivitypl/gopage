package server

import (
	"context"
	"net/http"

	"strconv"
	"strings"

	"github.com/sonquer/rill/internal/i18n"
	"github.com/sonquer/rill/internal/runtime"
)

type translatorKey struct{}

type Translator func(key string, count int, counted bool) string

func WithTranslator(ctx context.Context, translate Translator) context.Context {
	return context.WithValue(ctx, translatorKey{}, translate)
}

func TranslatorFrom(ctx context.Context) Translator {
	translate, ok := ctx.Value(translatorKey{}).(Translator)
	if !ok {
		return func(key string, _ int, _ bool) string { return key }
	}
	return translate
}

func (a *App) translator(r *http.Request) Translator {
	locale := LocaleOf(r)
	catalog, ok := a.manifest.Catalog(locale)
	if !ok {
		return func(key string, _ int, _ bool) string { return key }
	}
	rule := i18n.RuleFor(locale)
	return func(key string, count int, counted bool) string {
		message, ok := a.messages[key]
		if !ok {
			return key
		}
		form := int(i18n.FormOther)
		if counted {
			form = int(rule(float64(count)))
		}
		text, ok := catalog.Text(message, form)
		if !ok || text == "" {
			if text, ok = catalog.Text(message, int(i18n.FormOther)); !ok {
				return key
			}
		}
		if !counted {
			return text
		}
		return strings.ReplaceAll(text, runtime.CountPlaceholder, strconv.Itoa(count))
	}
}
