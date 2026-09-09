package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	goruntime "runtime"

	"github.com/apptivitypl/gopage/internal/action"
	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/redirect"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func nested() *ir.Manifest {
	m := &ir.Manifest{Version: ir.Version}
	m.Plans = []ir.Plan{
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 6}, {Kind: ir.OpOutlet}, {Kind: ir.OpStatic, A: 6, B: 7}},
			Blob: []byte("<main></main>"), Capacity: 64},
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 7}, {Kind: ir.OpOutlet}, {Kind: ir.OpStatic, A: 7, B: 8}},
			Blob: []byte("<aside></aside>"), Capacity: 64},
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 4}}, Blob: []byte("home"), Capacity: 16},
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 4}}, Blob: []byte("docs"), Capacity: 16},
		{Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 5}}, Blob: []byte("guide"), Capacity: 16},
	}
	m.Routes = []ir.Route{
		{Pattern: "/", Name: "index", Plan: 2, LayoutChain: []uint32{0}},
		{Pattern: "/docs", Name: "docs", Plan: 3, LayoutChain: []uint32{0, 1}},
		{Pattern: "/docs/guide", Name: "docs.guide", Plan: 4, LayoutChain: []uint32{0, 1}},
	}
	return m
}

func navApp(t *testing.T, mode string) *App {
	t.Helper()
	return New(Options{Manifest: nested(), Config: settings(t, `{"nav": {"mode": "`+mode+`"}}`)})
}

func fetchPartial(t *testing.T, app *App, from, to string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, to, nil)
	request.Header.Set(PartialHeader, from)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestOutletMarkersAppearOnlyWithPartialNavigation(t *testing.T) {
	on := get(t, navApp(t, "partial").Handler(), "/")
	if !strings.Contains(on.Body.String(), runtime.MarkerOpen+"0-->") {
		t.Errorf("body = %q, want the outlet marked", on.Body.String())
	}
	off := get(t, navApp(t, "off").Handler(), "/")
	if strings.Contains(off.Body.String(), runtime.MarkerOpen) {
		t.Errorf("body = %q, want no markers", off.Body.String())
	}
}

func TestASharedLayoutIsNotResent(t *testing.T) {
	recorder := fetchPartial(t, navApp(t, "partial"), "/docs", "/docs/guide")
	if recorder.Header().Get(LevelHeader) != "2" {
		t.Errorf("level = %q, want both layouts kept", recorder.Header().Get(LevelHeader))
	}
	body := recorder.Body.String()
	if body != "guide" {
		t.Errorf("body = %q, want only the page", body)
	}
	if recorder.Header().Get("Content-Type") != PartialType {
		t.Errorf("content type = %q", recorder.Header().Get("Content-Type"))
	}
}

func TestOnlyTheSharedPrefixIsKept(t *testing.T) {
	recorder := fetchPartial(t, navApp(t, "partial"), "/", "/docs")
	if recorder.Header().Get(LevelHeader) != "1" {
		t.Errorf("level = %q, want the root layout kept", recorder.Header().Get(LevelHeader))
	}
	if body := recorder.Body.String(); !strings.Contains(body, "<aside>") || !strings.Contains(body, "docs") {
		t.Errorf("body = %q, want the inner layout resent", body)
	}
}

func TestAnUnknownOriginFallsBackToTheWholeChain(t *testing.T) {
	recorder := fetchPartial(t, navApp(t, "partial"), "/gone", "/docs")
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type = %q, want the whole document rather than a tail", got)
	}
	if recorder.Header().Get(LevelHeader) != "" {
		t.Errorf("level = %q, want no level on a document", recorder.Header().Get(LevelHeader))
	}
	if !strings.Contains(recorder.Body.String(), "<main>") {
		t.Errorf("body = %q, want the root layout included", recorder.Body.String())
	}
}

func TestTheTitleTravelsEscaped(t *testing.T) {
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, "{\"nav\": {\"mode\": \"partial\"}}"),
		Meta: map[string]MetaProvider{
			"docs": func(*http.Request, Params, runtime.Accessible) (runtime.Meta, error) {
				return runtime.Meta{Title: "żółw & co"}, nil
			},
		},
	})
	recorder := fetchPartial(t, app, "/", "/docs")
	got := recorder.Header().Get(TitleHeader)
	if got == "" || strings.Contains(got, "&") || strings.Contains(got, "ż") {
		t.Errorf("title = %q, want it escaped for a header", got)
	}
}

func TestAPageWithoutATitleStillSendsTheHeader(t *testing.T) {
	recorder := fetchPartial(t, navApp(t, "partial"), "/docs", "/")
	if _, ok := recorder.Header()[http.CanonicalHeaderKey(TitleHeader)]; !ok {
		t.Error("the header must always travel so the client can clear a stale title")
	}
}

func TestPartialRequestsAreIgnoredWhenNavigationIsOff(t *testing.T) {
	recorder := fetchPartial(t, navApp(t, "off"), "/", "/docs")
	if recorder.Header().Get("Content-Type") == PartialType {
		t.Errorf("content type = %q, want a whole document", recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Body.String(), "<main>") {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestARequestWithoutTheHeaderGetsTheWholeDocument(t *testing.T) {
	recorder := get(t, navApp(t, "partial").Handler(), "/docs")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "<main>") {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestAFailingRenderIsReportedToThePartialClient(t *testing.T) {
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, "{\"nav\": {\"mode\": \"partial\"}}"),
		Props: map[string]PropsProvider{
			"docs": func(*http.Request, Params) (runtime.Accessible, error) { return nil, http.ErrBodyNotAllowed },
		},
	})
	if recorder := fetchPartial(t, app, "/", "/docs"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestAFailingMetaFailsThePartialTheSameWayItFailsADocument(t *testing.T) {
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, "{\"nav\": {\"mode\": \"partial\"}}"),
		Meta: map[string]MetaProvider{
			"docs": func(*http.Request, Params, runtime.Accessible) (runtime.Meta, error) {
				return runtime.Meta{}, http.ErrBodyNotAllowed
			},
		},
	})
	if recorder := fetchPartial(t, app, "/", "/docs"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestTheTitleIsReadFromThePropsThatWereAlreadyBuilt(t *testing.T) {
	var calls int
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, "{\"nav\": {\"mode\": \"partial\"}}"),
		Meta: map[string]MetaProvider{
			"docs": func(*http.Request, Params, runtime.Accessible) (runtime.Meta, error) {
				calls++
				return runtime.Meta{Title: "docs"}, nil
			},
		},
	})
	fetchPartial(t, app, "/", "/docs")
	if calls != 1 {
		t.Errorf("meta provider ran %d times, want once", calls)
	}
}

type refusingWriter struct{ header http.Header }

func (w *refusingWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (*refusingWriter) Write([]byte) (int, error) { return 0, http.ErrBodyNotAllowed }

func (*refusingWriter) WriteHeader(int) {}

func TestAPartialThatCannotRenderIsReported(t *testing.T) {
	broken := nested()
	broken.Plans[4] = ir.Plan{
		Ops:      []ir.Op{{Kind: ir.OpText, A: 0}},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{"Missing"}},
		Capacity: 8,
	}
	app := New(Options{Manifest: broken, Config: settings(t, "{\"nav\": {\"mode\": \"partial\"}}")})
	if recorder := fetchPartial(t, app, "/docs", "/docs/guide"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestAConnectionThatGoesAwayIsLogged(t *testing.T) {
	app := navApp(t, "partial")
	request := httptest.NewRequest(http.MethodGet, "/docs", nil)
	request.Header.Set(PartialHeader, "/")
	app.Handler().ServeHTTP(&refusingWriter{}, request)

	document := httptest.NewRequest(http.MethodGet, "/docs", nil)
	app.Handler().ServeHTTP(&refusingWriter{}, document)
}

func TestHeadOnAFallbackPageSendsNoBody(t *testing.T) {
	app := New(Options{Manifest: withFallbacks(manifest())})
	request := httptest.NewRequest(http.MethodHead, "/missing", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound || recorder.Body.Len() != 0 {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Length") == "" {
		t.Error("a head request still reports the length")
	}
}

func TestAFallbackWriteFailureIsLogged(t *testing.T) {
	app := New(Options{Manifest: withFallbacks(manifest())})
	app.Handler().ServeHTTP(&refusingWriter{}, httptest.NewRequest(http.MethodGet, "/missing", nil))
}

func TestAPageWithoutASubmitProviderAllowsOnlyReads(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	request := httptest.NewRequest(http.MethodPut, "/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("allow = %q", recorder.Header().Get("Allow"))
	}
}

func TestRenderStaticUsesThePatternAsThePath(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	body, err := app.RenderStatic(manifest().Routes[0])
	if err != nil || !strings.Contains(string(body), "<home") {
		t.Errorf("body = %q, err = %v", body, err)
	}
	empty := ir.Route{Pattern: "", Name: "index", Plan: 1, LayoutChain: []uint32{0}}
	if _, err := app.RenderStatic(empty); err != nil {
		t.Errorf("an empty pattern renders at the root: %v", err)
	}
}

func TestAFailingProviderStopsRenderStatic(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) { return nil, http.ErrBodyNotAllowed },
		},
	})
	if _, err := app.RenderStatic(manifest().Routes[0]); err == nil {
		t.Error("a failing provider must stop a static render")
	}
}

func TestTheCatalogForTheRequestLocaleIsUsed(t *testing.T) {
	m := manifest()
	m.Messages = []string{"hello"}
	m.Catalogs = []ir.Catalog{{Locale: "en", Texts: [][ir.PluralForms]string{{"hi"}}}}
	m.Plans[1] = ir.Plan{
		Messages: []string{"hello"},
		Ops:      []ir.Op{{Kind: ir.OpText, A: 0}},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprMessage, A: 0, B: runtime.NoArgument}},
		Capacity: 16,
	}
	app := New(Options{Manifest: m})
	if body := get(t, app.Handler(), "/").Body.String(); !strings.Contains(body, ">hi<") {
		t.Errorf("body = %q, want the catalog text", body)
	}
}

func TestAnOversizedPartialHeaderIsNotSplit(t *testing.T) {
	const runs = 20
	app := navApp(t, "partial")
	target := app.manifest.Routes[2]
	huge := "/docs/" + strings.Repeat("a/", 40_000)

	var before, after goruntime.MemStats
	goruntime.GC()
	goruntime.ReadMemStats(&before)
	for range runs {
		app.sharedLevel(huge, target)
	}
	goruntime.ReadMemStats(&after)

	if grown := (after.TotalAlloc - before.TotalAlloc) / runs; grown > 4<<10 {
		t.Errorf("%d bytes per call, want the header refused before it is split into segments", grown)
	}
	if level := app.sharedLevel(huge, target); level != 0 {
		t.Errorf("level = %d, want a whole chain for a header nobody can match", level)
	}
}

func TestAPartialHeaderAtTheCapStillMatches(t *testing.T) {
	recorder := fetchPartial(t, navApp(t, "partial"), "/docs", "/docs/guide")
	if got := recorder.Header().Get(LevelHeader); got != "2" {
		t.Errorf("level = %q, want a header inside the cap matched", got)
	}
}

func countingNav(t *testing.T, policy func(*cache.Recorder)) (*App, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, `{"nav": {"mode": "partial"}}`),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"docs.guide": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				policy(cache.From(r.Context()))
				return runtime.Empty{}, nil
			},
		},
	})
	return app, &calls
}

func TestASecondPartialForTheSameLevelIsServedFromTheCache(t *testing.T) {
	app, calls := countingNav(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	first := fetchPartial(t, app, "/docs", "/docs/guide")
	if got := first.Header().Get(CacheHeader); got != "miss" {
		t.Errorf("first = %q", got)
	}
	second := fetchPartial(t, app, "/docs", "/docs/guide")
	if got := second.Header().Get(CacheHeader); got != "hit" {
		t.Errorf("second = %q", got)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want the loader spared on the second click", calls.Load())
	}
	if second.Body.String() != first.Body.String() || second.Body.String() != "guide" {
		t.Errorf("bodies = %q and %q", first.Body.String(), second.Body.String())
	}
	if got := second.Header().Get("Content-Type"); got != PartialType {
		t.Errorf("content type on a hit = %q", got)
	}
	if got := second.Header().Get(LevelHeader); got != "2" {
		t.Errorf("level on a hit = %q", got)
	}
}

func TestADocumentAndAPartialForTheSameUrlDoNotShareAnEntry(t *testing.T) {
	app, calls := countingNav(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	document := get(t, app.Handler(), "/docs/guide")
	tail := fetchPartial(t, app, "/docs", "/docs/guide")
	if !strings.Contains(document.Body.String(), "<main>") {
		t.Errorf("document = %q, want the whole chain", document.Body.String())
	}
	if tail.Body.String() != "guide" {
		t.Errorf("partial = %q, want the page alone", tail.Body.String())
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want one render each", calls.Load())
	}
	if entries := app.CacheStats().Entries; entries != 2 {
		t.Errorf("entries = %d, want the document and the tail kept apart", entries)
	}
}

func TestEachSharedLevelGetsItsOwnEntry(t *testing.T) {
	app, calls := countingNav(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	levels := map[string]string{"/": "1", "/docs": "2", "/docs/guide": "2"}
	for from, want := range levels {
		if got := fetchPartial(t, app, from, "/docs/guide").Header().Get(LevelHeader); got != want {
			t.Errorf("from %s: level = %q, want %q", from, got, want)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want one render per level", calls.Load())
	}
	if entries := app.CacheStats().Entries; entries != 2 {
		t.Errorf("entries = %d, want the source path out of the key", entries)
	}
}

func TestACachedPartialStillCarriesItsTitle(t *testing.T) {
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, `{"nav": {"mode": "partial"}}`),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"docs": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				cache.From(r.Context()).TTL(time.Minute)
				return runtime.Empty{}, nil
			},
		},
		Meta: map[string]MetaProvider{
			"docs": func(*http.Request, Params, runtime.Accessible) (runtime.Meta, error) {
				return runtime.Meta{Title: "żółw & co"}, nil
			},
		},
	})
	first := fetchPartial(t, app, "/", "/docs")
	second := fetchPartial(t, app, "/", "/docs")
	if got := second.Header().Get(CacheHeader); got != "hit" {
		t.Errorf("second = %q", got)
	}
	if got := second.Header().Get(TitleHeader); got == "" || got != first.Header().Get(TitleHeader) {
		t.Errorf("title on a hit = %q, want the one the loader produced", got)
	}
}

func TestAPartialIsNeverAdvertisedAsShareable(t *testing.T) {
	app, _ := countingNav(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	for _, recorder := range []*httptest.ResponseRecorder{
		fetchPartial(t, app, "/docs", "/docs/guide"),
		fetchPartial(t, app, "/docs", "/docs/guide"),
	} {
		if got := recorder.Header().Get("Cache-Control"); got != "private, max-age=60" {
			t.Errorf("cache-control = %q, want a partial kept out of shared caches", got)
		}
	}
	plain, _ := countingNav(t, func(*cache.Recorder) {})
	if got := fetchPartial(t, plain, "/docs", "/docs/guide").Header().Get("Cache-Control"); got != PrivateFreshness {
		t.Errorf("cache-control without a policy = %q", got)
	}
}

func TestAPartialFromAnUnknownOriginWarmsTheDocument(t *testing.T) {
	app, calls := countingNav(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	if got := fetchPartial(t, app, "/gone", "/docs/guide").Header().Get(CacheHeader); got != "miss" {
		t.Errorf("first = %q", got)
	}
	if got := get(t, app.Handler(), "/docs/guide").Header().Get(CacheHeader); got != "hit" {
		t.Errorf("the document after it = %q, want the entry the partial filled", got)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want the whole chain rendered once", calls.Load())
	}
}

func TestAPartialWithAFlashIsRenderedFresh(t *testing.T) {
	app, calls := countingNav(t, func(r *cache.Recorder) { r.TTL(time.Minute) })
	fetchPartial(t, app, "/docs", "/docs/guide")

	request := httptest.NewRequest(http.MethodGet, "/docs/guide", nil)
	request.Header.Set(PartialHeader, "/docs")
	request.AddCookie(&http.Cookie{Name: hosted(action.FlashCookie), Value: "sent"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	if got := recorder.Header().Get(CacheHeader); got != "bypass" {
		t.Errorf("header = %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != PartialType {
		t.Errorf("content type = %q, want the tail even when the cache is skipped", got)
	}
	if recorder.Body.String() != "guide" {
		t.Errorf("body = %q, want the page alone", recorder.Body.String())
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestAPartialForAPrivateVisitorIsRenderedFresh(t *testing.T) {
	var calls atomic.Int64
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, `{"nav": {"mode": "partial"}, "security": {"privateCookies": ["session"]}}`),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Props: map[string]PropsProvider{
			"docs.guide": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				cache.From(r.Context()).TTL(time.Minute)
				return runtime.Empty{}, nil
			},
		},
	})
	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/docs/guide", nil)
		request.Header.Set(PartialHeader, "/docs")
		request.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if got := recorder.Header().Get(CacheHeader); got != "bypass" {
			t.Errorf("header = %q", got)
		}
		if recorder.Body.String() != "guide" {
			t.Errorf("body = %q", recorder.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want a signed-in visitor served fresh", calls.Load())
	}
}

func TestInvalidatingATagClearsTheDocumentAndThePartial(t *testing.T) {
	app, calls := countingNav(t, func(r *cache.Recorder) { r.TTL(time.Minute).Tag("guide") })
	get(t, app.Handler(), "/docs/guide")
	fetchPartial(t, app, "/docs", "/docs/guide")
	if removed := app.Invalidate("guide"); removed != 2 {
		t.Errorf("removed = %d, want both variants", removed)
	}
	get(t, app.Handler(), "/docs/guide")
	fetchPartial(t, app, "/docs", "/docs/guide")
	if calls.Load() != 4 {
		t.Errorf("calls = %d, want both variants rendered again", calls.Load())
	}
}

func TestALoaderRedirectReachesThePartialClient(t *testing.T) {
	app := New(Options{
		Manifest: nested(),
		Config:   settings(t, `{"nav": {"mode": "partial"}}`),
		Props: map[string]PropsProvider{
			"docs.guide": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, redirect.Fail(http.StatusSeeOther, "/docs")
			},
		},
	})
	recorder := fetchPartial(t, app, "/docs", "/docs/guide")
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want the redirect the loader asked for", recorder.Code)
	}
	if got := recorder.Header().Get("Location"); got != "/docs" {
		t.Errorf("location = %q", got)
	}
}

func TestAPartialKeepsTheOutletNumberingOfTheDocument(t *testing.T) {
	app := navApp(t, "partial")
	document := get(t, app.Handler(), "/docs/guide").Body.String()
	if !strings.Contains(document, runtime.MarkerOpen+"1-->") {
		t.Fatalf("document = %q, want the nested outlet marked", document)
	}
	tail := fetchPartial(t, app, "/", "/docs/guide").Body.String()
	if !strings.Contains(tail, runtime.MarkerOpen+"1-->") {
		t.Errorf("partial = %q, want the outlet numbered as the document numbers it", tail)
	}
	if strings.Contains(tail, runtime.MarkerOpen+"0-->") {
		t.Errorf("partial = %q, want no marker of a layout it does not carry", tail)
	}
}

func TestALayoutOutsideThePartialDoesNotDecideItsFreshness(t *testing.T) {
	held := nested()
	held.Layouts = []ir.Layout{{Name: "root", Plan: 0}}
	var calls atomic.Int64
	app := New(Options{
		Manifest: held,
		Config:   settings(t, `{"nav": {"mode": "partial"}}`),
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Layouts: map[string]PropsProvider{
			"root": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				cache.From(r.Context()).Private()
				return runtime.Empty{}, nil
			},
		},
		Props: map[string]PropsProvider{
			"docs.guide": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				calls.Add(1)
				cache.From(r.Context()).TTL(time.Minute)
				return runtime.Empty{}, nil
			},
		},
	})
	if got := get(t, app.Handler(), "/docs/guide").Header().Get(CacheHeader); got != "bypass" {
		t.Errorf("document = %q, want the private layout to keep it out of the cache", got)
	}
	fetchPartial(t, app, "/docs", "/docs/guide")
	if got := fetchPartial(t, app, "/docs", "/docs/guide").Header().Get(CacheHeader); got != "hit" {
		t.Errorf("partial = %q, want a tail the layout is not part of to be cached", got)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want the document and the first partial only", calls.Load())
	}
}
