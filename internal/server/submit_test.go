package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/rill/internal/action"
	"github.com/apptivitypl/rill/internal/cookie"
	"github.com/apptivitypl/rill/internal/csrf"
	"github.com/apptivitypl/rill/internal/form"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

func formManifest() *ir.Manifest {
	m := manifest()
	m.Plans = append(m.Plans, ir.Plan{
		Ops: []ir.Op{
			{Kind: ir.OpStatic, A: 0, B: 6},
			{Kind: ir.OpText, A: 0},
			{Kind: ir.OpStatic, A: 6, B: 1},
			{Kind: ir.OpText, A: 1},
		},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}, {Kind: ir.ExprPath, A: 1}},
		Paths:    [][]string{{form.Root, "Token"}, {action.FlashRoot}},
		Blob:     []byte("token:|"),
		Capacity: 32,
	})
	m.Routes = append(m.Routes, ir.Route{
		Pattern: "/apply", Name: "apply", Plan: uint32(len(m.Plans) - 1), Class: ir.ClassDynamic,
	})
	return m
}

func hosted(base string) string {
	return cookie.HostPrefix + base
}

func submitApp(t *testing.T, provider SubmitProvider) *App {
	t.Helper()
	return New(Options{
		Manifest: formManifest(),
		Submit:   map[string]SubmitProvider{"apply": provider},
	})
}

func postForm(t *testing.T, handler http.Handler, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func rendered(t *testing.T, handler http.Handler) (*http.Cookie, string) {
	t.Helper()
	recorder := get(t, handler, "/apply")
	var held *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == hosted(csrf.CookieName) {
			held = cookie
		}
	}
	if held == nil {
		t.Fatal("no csrf cookie was issued")
	}
	body := recorder.Body.String()
	start := strings.Index(body, "token:")
	if start < 0 {
		t.Fatalf("body = %q, want a rendered token", body)
	}
	rest := body[start+len("token:"):]
	end := strings.IndexByte(rest, '|')
	if end < 0 {
		t.Fatalf("body = %q, want the token delimited", body)
	}
	return held, rest[:end]
}

func validToken(t *testing.T) (*http.Cookie, string) {
	t.Helper()
	secret, err := csrf.Generate(nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	masked, err := csrf.Mask(secret, nil)
	if err != nil {
		t.Fatalf("Mask: %v", err)
	}
	return &http.Cookie{Name: hosted(csrf.CookieName), Value: secret}, masked
}

func issued(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	recorder := get(t, handler, "/apply")
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == hosted(csrf.CookieName) {
			return cookie
		}
	}
	t.Fatal("no csrf cookie was issued")
	return nil
}

func TestGetIssuesATokenAndRendersIt(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		return nil, form.Result{}, nil
	})
	handler := app.Handler()
	recorder := get(t, handler, "/apply")
	token := issued(t, handler).Value
	if !strings.Contains(recorder.Body.String(), "token:") || token == "" {
		t.Errorf("body = %q, token = %q", recorder.Body.String(), token)
	}
}

func TestARouteWithoutASubmitProviderRejectsPost(t *testing.T) {
	app := New(Options{Manifest: formManifest()})
	recorder := postForm(t, app.Handler(), "")
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("status = %d, allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func TestASubmissionWithoutATokenIsRejected(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		t.Error("the provider must not run")
		return nil, form.Result{}, nil
	})
	if recorder := postForm(t, app.Handler(), "Name=Ada"); recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestAValidSubmissionRedirects(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		return action.To("/thanks").Flash("sent"), form.Result{}, nil
	})
	handler := app.Handler()
	cookie, token := rendered(t, handler)
	recorder := postForm(t, handler, "__csrf="+token+"&Name=Ada", cookie)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/thanks" {
		t.Errorf("status = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestAFlashIsShownOnceAndCleared(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		return nil, form.Result{}, nil
	})
	handler := app.Handler()
	request := httptest.NewRequest(http.MethodGet, "/apply", nil)
	request.AddCookie(&http.Cookie{Name: hosted(action.FlashCookie), Value: "sent"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), "|sent") {
		t.Errorf("body = %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("the flash must be cleared: %q", recorder.Header().Get("Set-Cookie"))
	}
}

func TestAnInvalidSubmissionRerendersWith422(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		return nil, form.Result{Submitted: true, Errors: map[string][]string{"Name": {"too short"}}}, nil
	})
	handler := app.Handler()
	cookie, token := rendered(t, handler)
	recorder := postForm(t, handler, "__csrf="+token+"&Name=A", cookie)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "token:") {
		t.Errorf("the rerender keeps the token: %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "token:"+cookie.Value) {
		t.Errorf("the rerender wrote the secret itself: %q", recorder.Body.String())
	}
}

func TestAFailingProviderRendersTheErrorPage(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		return nil, form.Result{}, errors.New("nope")
	})
	handler := app.Handler()
	cookie, token := rendered(t, handler)
	recorder := postForm(t, handler, "__csrf="+token, cookie)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestAnUnresolvableActionRendersTheErrorPage(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		return action.Route("nope"), form.Result{}, nil
	})
	handler := app.Handler()
	cookie, token := rendered(t, handler)
	recorder := postForm(t, handler, "__csrf="+token, cookie)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func TestATokenThatCannotBeIssuedFailsTheRender(t *testing.T) {
	app := New(Options{
		Manifest: formManifest(),
		Entropy:  failingEntropy{},
		Submit: map[string]SubmitProvider{"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
			return nil, form.Result{}, nil
		}},
	})
	if recorder := get(t, app.Handler(), "/apply"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestPropsFailureDuringRerender(t *testing.T) {
	app := New(Options{
		Manifest: formManifest(),
		Props: map[string]PropsProvider{
			"apply": func(*http.Request, Params) (runtime.Accessible, error) { return nil, errors.New("nope") },
		},
		Submit: map[string]SubmitProvider{"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
			return nil, form.Result{Submitted: true, Errors: map[string][]string{"Name": {"x"}}}, nil
		}},
	})
	handler := app.Handler()
	held, masked := validToken(t)
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("__csrf="+masked))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(held)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestResolveFillsThePattern(t *testing.T) {
	app := New(Options{Manifest: formManifest()})
	got, err := app.resolve("listings.id", map[string]string{"id": "7"})
	if err != nil || got != "/listings/7" {
		t.Errorf("resolve = %q, err = %v", got, err)
	}
	if _, err := app.resolve("nope", nil); err == nil {
		t.Error("an unknown route must be reported")
	}
	if _, err := app.resolve("listings.id", nil); err == nil {
		t.Error("a missing parameter must be reported")
	}
}

func TestFill(t *testing.T) {
	cases := []struct {
		pattern string
		params  map[string]string
		want    string
	}{
		{"/", nil, "/"},
		{"/thanks", nil, "/thanks"},
		{"/listings/[id]", map[string]string{"id": "7"}, "/listings/7"},
		{"/docs/[[...slug]]", nil, "/docs"},
		{"/docs/[[...slug]]", map[string]string{"slug": "a/b"}, "/docs/a/b"},
		{"/[[...slug]]", nil, "/"},
	}
	for _, c := range cases {
		got, err := fill(c.pattern, c.params)
		if err != nil || got != c.want {
			t.Errorf("fill(%q) = %q, %v, want %q", c.pattern, got, err, c.want)
		}
	}
}

func TestAPageRejectsOtherMethods(t *testing.T) {
	app := submitApp(t, func(*http.Request, Params) (action.Action, form.Result, error) {
		return nil, form.Result{}, nil
	})
	request := httptest.NewRequest(http.MethodPut, "/apply", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != "GET, HEAD, POST" {
		t.Errorf("status = %d, allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func TestARerenderThatCannotRenderFallsBack(t *testing.T) {
	broken := formManifest()
	broken.Plans[len(broken.Plans)-1] = ir.Plan{
		Ops:      []ir.Op{{Kind: ir.OpText, A: 0}},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{"Missing"}},
		Capacity: 8,
	}
	app := New(Options{
		Manifest: broken,
		Submit: map[string]SubmitProvider{"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
			return nil, form.Result{Submitted: true, Errors: map[string][]string{"Name": {"x"}}}, nil
		}},
	})
	held, masked := validToken(t)
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("__csrf="+masked))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(held)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func secureApp(t *testing.T, text string) *App {
	t.Helper()
	return New(Options{
		Manifest: formManifest(),
		Config:   settings(t, text),
		Submit: map[string]SubmitProvider{
			"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
				return action.Route("index").Flash("sent"), form.Result{}, nil
			},
		},
	})
}

func firstCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	held := recorder.Result().Cookies()
	if len(held) == 0 {
		t.Fatal("no cookie was issued")
	}
	return held[0]
}

func TestATerminatingProxyStillGetsASecureCsrfCookie(t *testing.T) {
	app := secureApp(t, "{\"app\": {\"scheme\": \"https\"}}")
	held := firstCookie(t, get(t, app.Handler(), "/apply"))
	if !held.Secure {
		t.Errorf("cookie = %+v, want Secure even without r.TLS", held)
	}
	if held.Name != cookie.HostPrefix+csrf.CookieName {
		t.Errorf("name = %q, want the __Host- prefix", held.Name)
	}
}

func TestACookieIsSecureWithoutBeingToldAnything(t *testing.T) {
	app := secureApp(t, "")
	held := firstCookie(t, get(t, app.Handler(), "/apply"))
	if !held.Secure || held.Name != cookie.HostPrefix+csrf.CookieName {
		t.Errorf("cookie = %+v, want Secure and __Host- behind a terminating proxy", held)
	}
}

func TestNoProxyHeaderCanDowngradeACookie(t *testing.T) {
	for _, settings := range []string{"", "{\"security\": {\"trustedProxy\": true}}"} {
		app := secureApp(t, settings)
		request := httptest.NewRequest(http.MethodGet, "/apply", nil)
		request.Header.Set(ForwardedProto, "http")
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if held := firstCookie(t, recorder); !held.Secure {
			t.Errorf("%s: cookie = %+v, want a spoofable header unable to drop Secure", settings, held)
		}
	}
}

func TestOnlyTheConfiguredSchemeDropsSecure(t *testing.T) {
	app := secureApp(t, "{\"app\": {\"scheme\": \"http\"}}")
	held := firstCookie(t, get(t, app.Handler(), "/apply"))
	if held.Secure || held.Name != csrf.CookieName {
		t.Errorf("cookie = %+v, want the declared http scheme honoured", held)
	}
}

func TestASecureFlashRoundTripsUnderThePrefixedName(t *testing.T) {
	app := secureApp(t, "{\"app\": {\"scheme\": \"https\"}}")
	handler := app.Handler()
	token, masked := rendered(t, handler)

	recorder := postForm(t, handler, "__csrf="+masked+"&Name=Ada", token)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	var flash *http.Cookie
	for _, held := range recorder.Result().Cookies() {
		if held.Name == cookie.HostPrefix+action.FlashCookie {
			flash = held
		}
	}
	if flash == nil || !flash.Secure {
		t.Fatalf("cookies = %v, want a secure prefixed flash", recorder.Result().Cookies())
	}

	request := httptest.NewRequest(http.MethodGet, "/apply", nil)
	request.AddCookie(flash)
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, request)
	if !strings.Contains(read.Body.String(), "sent") {
		t.Errorf("body = %q, want the flash read back", read.Body.String())
	}
}

func TestARerenderThatCannotIssueATokenFallsBack(t *testing.T) {
	app := New(Options{
		Manifest: formManifest(),
		Entropy:  failingEntropy{},
		Submit: map[string]SubmitProvider{
			"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
				return nil, form.Result{Submitted: true, Errors: map[string][]string{"Name": {"too short"}}}, nil
			},
		},
	})
	held, masked := validToken(t)
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("__csrf="+masked+"&Name=A"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(held)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the failure reported rather than a form without a token", recorder.Code)
	}
}
