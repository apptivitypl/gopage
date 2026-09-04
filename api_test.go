package gopage

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNewMetaCarriesTheTitle(t *testing.T) {
	if got := NewMeta("hello"); got.Title != "hello" {
		t.Errorf("meta = %+v", got)
	}
}

func TestConfigIsParsed(t *testing.T) {
	app, err := New(Options{Manifest: demo(t), Config: []byte("{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pl", nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestBrokenConfigIsReported(t *testing.T) {
	if _, err := New(Options{Manifest: demo(t), Config: []byte("{\"i18n\": {\"mode\": \"domain\"}}")}); err == nil {
		t.Error("an unknown i18n mode must be reported")
	}
}

func TestStaticAssetsAreServed(t *testing.T) {
	static := fstest.MapFS{"styles/app.css": {Data: []byte("body{margin:0}")}}
	app, err := New(Options{Manifest: demo(t), Static: static})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := recorder.Header().Get("GOPAGE-Assets"); !strings.Contains(got, "rel=preload") {
		t.Errorf("assets header = %q", got)
	}
}

func TestAnAppWithoutStaticFilesStillStarts(t *testing.T) {
	for _, static := range []fstest.MapFS{nil, {"app/page.gopage": {Data: []byte("<h1>x</h1>")}}} {
		if _, err := New(Options{Manifest: demo(t), Static: static}); err != nil {
			t.Errorf("New: %v", err)
		}
	}
}

type brokenFS struct{ inner fs.FS }

func (b brokenFS) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, ".css") || strings.HasSuffix(name, ".js") {
		return nil, errors.New("no")
	}
	return b.inner.Open(name)
}

func TestUnreadableStaticFilesAreReported(t *testing.T) {
	static := brokenFS{inner: fstest.MapFS{"styles/app.css": {Data: []byte("body{}")}}}
	if _, err := New(Options{Manifest: demo(t), Static: static}); err == nil {
		t.Error("an unreadable static tree must be reported")
	}
}

func TestLocaleOfReadsTheRequest(t *testing.T) {
	app, err := New(Options{Manifest: demo(t), Config: []byte("{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var seen string
	app2, err := New(Options{
		Manifest: demo(t),
		Config:   []byte("{\"i18n\": {\"locales\": [\"en\", \"pl\"]}}"),
		API: map[string]http.Handler{"/api/where": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = LocaleOf(r)
		})},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = app
	app2.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/where", nil))
	if seen != "en" {
		t.Errorf("locale = %q", seen)
	}
}

func TestAPIDispatchesByMethod(t *testing.T) {
	handler := API(map[string]APIHandler{
		http.MethodGet: func(*http.Request) (Response, error) { return JSON(map[string]int{"n": 1}), nil },
		http.MethodPut: func(*http.Request) (Response, error) { return NoContent(), nil },
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if strings.TrimSpace(recorder.Body.String()) != `{"n":1}` {
		t.Errorf("body = %q", recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/x", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestTextResponse(t *testing.T) {
	handler := API(map[string]APIHandler{
		http.MethodGet: func(*http.Request) (Response, error) { return Text("pong"), nil },
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	if recorder.Body.String() != "pong" {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestParamsFromReadsPathValues(t *testing.T) {
	mux := http.NewServeMux()
	var seen Params
	mux.HandleFunc("/api/listings/{id}", func(w http.ResponseWriter, r *http.Request) {
		seen = ParamsFrom(r, []string{"id"})
	})
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/listings/7", nil))
	if seen["id"] != "7" {
		t.Errorf("params = %v", seen)
	}
}

func TestCtxExposesTheRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/listings/7?sort=price", nil)
	ctx := NewCtx(request, Params{"id": "7"})
	if ctx.Request() != request || ctx.Context() != request.Context() {
		t.Error("the context must carry the request")
	}
	if ctx.Param("id") != "7" || ctx.Params()["id"] != "7" {
		t.Errorf("params = %v", ctx.Params())
	}
	if ctx.Query("sort") != "price" {
		t.Errorf("query = %q", ctx.Query("sort"))
	}
}

func TestCtxWithoutARequestIsUsable(t *testing.T) {
	ctx := NewCtx(nil, nil)
	if ctx.Context() != context.Background() {
		t.Error("a context without a request falls back to the background context")
	}
	if ctx.Query("sort") != "" {
		t.Errorf("query = %q", ctx.Query("sort"))
	}
}

type tone string

func (t tone) gopageCase() string { return string(t) }

type card struct{ name string }

func (c card) Get(path []string) (Value, bool) {
	if len(path) == 1 && path[0] == "Name" {
		return String(c.name), true
	}
	return Nil(), false
}

func TestSequenceAdaptersReadInRange(t *testing.T) {
	cases := []struct {
		name string
		seq  Sequence
		want string
	}{
		{"objects", Objects[card]{{name: "a"}}, "a"},
		{"strings", Strings{"a"}, "a"},
		{"cases", Cases[tone]{"a"}, "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.seq.Len() != 1 {
				t.Fatalf("len = %d", c.seq.Len())
			}
			value := c.seq.At(0)
			if c.name == "objects" {
				inner := value.Object()
				if inner == nil {
					t.Fatalf("value = %+v", value)
				}
				got, _ := inner.Get([]string{"Name"})
				if got.Str != c.want {
					t.Errorf("name = %q", got.Str)
				}
				return
			}
			if value.Str != c.want {
				t.Errorf("value = %+v", value)
			}
		})
	}
}

func TestNumericSequenceAdapters(t *testing.T) {
	ints := Ints[int]{7}
	floats := Floats[float64]{1.5}
	bools := Bools{true}
	if ints.Len() != 1 || floats.Len() != 1 || bools.Len() != 1 {
		t.Fatal("every adapter reports one element")
	}
	if ints.At(0).Int() != 7 {
		t.Errorf("int = %+v", ints.At(0))
	}
	if floats.At(0).Float() != 1.5 {
		t.Errorf("float = %+v", floats.At(0))
	}
	if !bools.At(0).Truthy() {
		t.Errorf("bool = %+v", bools.At(0))
	}
}

func TestSequenceAdaptersClampOutOfRange(t *testing.T) {
	sequences := []Sequence{
		Objects[card]{{name: "a"}},
		Strings{"a"},
		Ints[int]{1},
		Floats[float64]{1},
		Bools{true},
		Cases[tone]{"a"},
	}
	for _, seq := range sequences {
		for _, index := range []int{-1, 9} {
			if got := seq.At(index); got.Kind != Nil().Kind {
				t.Errorf("%T.At(%d) = %+v, want nil", seq, index, got)
			}
		}
	}
}

func TestBundlesAreServedVerbatim(t *testing.T) {
	bundles := fstest.MapFS{"bundles/gopage.client.ABC.js": {Data: []byte("export{}")}}
	app, err := New(Options{Manifest: demo(t), Bundles: bundles})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/gopage.client.ABC.js", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "export{}" {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestStaticAndBundlesShareOneHandler(t *testing.T) {
	static := fstest.MapFS{"styles/app.css": {Data: []byte("body{}")}}
	bundles := fstest.MapFS{"bundles/gopage.client.ABC.js": {Data: []byte("export{}")}}
	app, err := New(Options{Manifest: demo(t), Static: static, Bundles: bundles})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	link := recorder.Header().Get("GOPAGE-Assets")
	if !strings.Contains(link, "rel=preload") || !strings.Contains(link, "rel=modulepreload") {
		t.Errorf("assets header = %q, want both stores listed", link)
	}
}

func TestUnreadableBundlesAreReported(t *testing.T) {
	broken := brokenFS{inner: fstest.MapFS{"bundles/a.js": {Data: []byte("export{}")}}}
	if _, err := New(Options{Manifest: demo(t), Bundles: broken}); err == nil {
		t.Error("an unreadable bundle store must be reported")
	}
}
