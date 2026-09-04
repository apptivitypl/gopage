package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/apptivitypl/gopage/internal/action"
	"github.com/apptivitypl/gopage/internal/csrf"
	"github.com/apptivitypl/gopage/internal/form"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

type SubmitProvider func(*http.Request, Params) (action.Action, form.Result, error)

func (a *App) submitPage(w http.ResponseWriter, r *http.Request, route ir.Route, params Params) {
	provider, ok := a.submit[route.Name]
	if !ok {
		a.logger.Warn("method not allowed", "route", route.Name, "method", r.Method, "allow", "GET, HEAD")
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r = r.WithContext(WithTranslator(r.Context(), a.translator(r)))
	if err := readSubmission(r); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			a.logger.Warn("submission too large", "route", route.Name, "limit", tooBig.Limit)
			http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
			return
		}
		a.logger.Warn("submission unreadable", "route", route.Name, "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := csrf.Verify(r, r.PostFormValue(csrf.Field)); err != nil {
		a.logger.Error("submission rejected", "route", route.Name, "error", err)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	act, result, err := provider(r, params)
	if err != nil {
		a.logger.Error("submission failed", "route", route.Name, "error", err)
		a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
		return
	}
	if act != nil {
		if err := act.Apply(w, r, a.resolve); err != nil {
			a.logger.Error("action failed", "route", route.Name, "error", err)
			a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
		}
		return
	}
	a.rerender(w, r, route, params, result)
}

func readSubmission(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	if err := r.ParseMultipartForm(form.MaxMemory); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return err
	}
	return nil
}

func (a *App) rerender(w http.ResponseWriter, r *http.Request, route ir.Route, params Params, result form.Result) {
	props, err := a.providers(route, r, params)
	if err != nil {
		a.logger.Error("render failed", "route", route.Name, "error", err)
		a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
		return
	}
	rooted := runtime.WithRoot(props, action.FlashRoot, runtime.Leaf(""))
	token, err := csrf.Issue(w, r, a.entropy)
	if err != nil {
		a.logger.Error("token failed", "route", route.Name, "error", err)
		a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
		return
	}
	body, err := a.renderResolved(route, form.With(rooted, result, token), nil,
		LocaleOf(r), a.resolved(r, params, route))
	if err != nil {
		a.logger.Error("render failed", "route", route.Name, "error", err)
		a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
		return
	}
	defer runtime.Release(body)
	a.write(w, r, body, http.StatusUnprocessableEntity)
}

func (a *App) resolve(name string, params map[string]string) (string, error) {
	for _, route := range a.manifest.Routes {
		if route.Name == name {
			return fill(route.Pattern, params)
		}
	}
	return "", fmt.Errorf("no route named %q", name)
}

func fill(pattern string, params map[string]string) (string, error) {
	var parts []string
	for segment := range strings.SplitSeq(strings.Trim(pattern, "/"), "/") {
		switch {
		case segment == "":
		case strings.HasPrefix(segment, "[["):
			if value := params[strings.Trim(segment, "[.]")]; value != "" {
				parts = append(parts, value)
			}
		case strings.HasPrefix(segment, "["):
			value := params[strings.Trim(segment, "[.]")]
			if value == "" {
				return "", fmt.Errorf("route %s needs a value for %s", pattern, strings.Trim(segment, "[.]"))
			}
			parts = append(parts, value)
		default:
			parts = append(parts, segment)
		}
	}
	if len(parts) == 0 {
		return "/", nil
	}
	return "/" + strings.Join(parts, "/"), nil
}

func (a *App) pageProps(w http.ResponseWriter, r *http.Request, route ir.Route, params Params) (runtime.Accessible, error) {
	props, err := a.providers(route, r, params)
	if err != nil {
		return nil, err
	}
	props = runtime.WithRoot(props, action.FlashRoot, runtime.Leaf(action.TakeFlash(w, r)))
	if _, ok := a.submit[route.Name]; !ok {
		return form.With(props, form.Result{}, ""), nil
	}
	token, err := csrf.Issue(w, r, a.entropy)
	if err != nil {
		return nil, err
	}
	return form.With(props, form.Result{}, token), nil
}
