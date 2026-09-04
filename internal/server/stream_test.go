package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

type slow string

func (s slow) Get(path []string) (runtime.Value, bool) {
	if len(path) != 0 {
		return runtime.Nil(), false
	}
	return runtime.String(string(s)), true
}

func streamPlan() *ir.Plan {
	plan := &ir.Plan{
		Fragments: []ir.Fragment{{Name: "Reviews", Deferred: true}},
		Blob:      []byte("<p>head</p><b></b><p>tail</p>"),
	}
	plan.Ops = []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 11},
		{Kind: ir.OpFragment, A: 0, B: 5},
		{Kind: ir.OpStatic, A: 11, B: 3},
		{Kind: ir.OpText, A: 0},
		{Kind: ir.OpStatic, A: 14, B: 4},
		{Kind: ir.OpStatic, A: 18, B: 11},
	}
	plan.Exprs = []ir.ExprNode{{Kind: ir.ExprPath, A: 0}}
	plan.Paths = [][]string{{"Reviews"}}
	return plan
}

func streamConfig(mode string) config.Config {
	settings := config.Default()
	settings.Fragments.Deferred = mode
	return settings
}

func streamApp(t *testing.T, delay time.Duration, mode string) *App {
	t.Helper()
	manifest := &ir.Manifest{
		Plans:  []ir.Plan{*streamPlan()},
		Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
	}
	return New(Options{
		Manifest: manifest,
		Config:   streamConfig(mode),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				time.Sleep(delay)
				return slow("late"), nil
			},
		},
	})
}

type recordingWriter struct {
	*httptest.ResponseRecorder
	mu     sync.Mutex
	chunks []string
}

func (r *recordingWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	r.chunks = append(r.chunks, string(p))
	r.mu.Unlock()
	return r.ResponseRecorder.Write(p)
}

func (r *recordingWriter) Flush() {}

func TestAStreamedPageSendsTheHeadBeforeTheFragment(t *testing.T) {
	app := streamApp(t, 0, config.DeferredTail)
	recorder := &recordingWriter{ResponseRecorder: httptest.NewRecorder()}
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if len(recorder.chunks) < 2 {
		t.Fatalf("chunks = %q, want the head flushed before the fragment", recorder.chunks)
	}
	if !strings.Contains(recorder.chunks[0], "<p>head</p>") {
		t.Errorf("first chunk = %q", recorder.chunks[0])
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "<b>late</b>") || !strings.Contains(body, "<p>tail</p>") {
		t.Errorf("body = %q", body)
	}
	if recorder.Header().Get("Content-Length") != "" {
		t.Error("a streamed response must not carry a content length")
	}
	if got := recorder.Header().Get(CacheHeader); got != StreamStatus {
		t.Errorf("cache header = %q, want %q", got, StreamStatus)
	}
}

func TestAStreamedHeadRequestSendsNoBody(t *testing.T) {
	app := streamApp(t, 0, config.DeferredTail)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/", nil))
	if recorder.Body.Len() != 0 {
		t.Errorf("body = %q, want nothing for HEAD", recorder.Body.String())
	}
}

func TestOutOfOrderStreamingSendsTheTemplateAfterTheDocument(t *testing.T) {
	app := streamApp(t, 30*time.Millisecond, config.DeferredTail)
	recorder := &recordingWriter{ResponseRecorder: httptest.NewRecorder()}
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	body := recorder.Body.String()
	slot := strings.Index(body, `<gopage-slot name="Reviews">`)
	tail := strings.Index(body, "<p>tail</p>")
	template := strings.Index(body, `<template data-gopage-slot="Reviews">`)
	if slot < 0 || tail < 0 || template < 0 {
		t.Fatalf("body = %q", body)
	}
	if slot >= tail || tail >= template {
		t.Errorf("body = %q, want slot, then the rest of the page, then the template", body)
	}
	if !strings.Contains(body[template:], "late") {
		t.Errorf("template = %q, want the fragment body", body[template:])
	}
}

func TestALoaderFailureLeavesTheDocumentReadable(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredInline),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, errors.New("upstream is down")
			},
		},
	})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(recorder.Body.String(), "<p>head</p>") {
		t.Errorf("body = %q, want the part that was already sent", recorder.Body.String())
	}
}

func TestARouteWithoutDeferredFragmentsIsNotStreamed(t *testing.T) {
	plan := ir.Plan{Blob: []byte("<p>plain</p>"), Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: 12}}}
	app := New(Options{Manifest: &ir.Manifest{
		Plans:  []ir.Plan{plan},
		Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
	}})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Header().Get("Content-Length") == "" {
		t.Error("an ordinary page still carries a content length")
	}
	if got := recorder.Header().Get(CacheHeader); got == StreamStatus {
		t.Error("an ordinary page must not report itself as a stream")
	}
}

func TestAwaitAnsweredForAFragmentWithoutALoader(t *testing.T) {
	set := &deferredSet{results: map[string]*result{}}
	props, err := set.Await(ir.Fragment{Name: "Absent"})
	if err != nil || props == nil {
		t.Errorf("props = %v, err = %v, want an empty accessible", props, err)
	}
	if !set.Settle(ir.Fragment{Name: "Absent"}, runtime.NoWait) {
		t.Error("a fragment without a loader is always settled")
	}
	if names := set.slots(); len(names) != 0 {
		t.Errorf("slots = %v, want a fragment without a loader left alone", names)
	}
	set.Flush()
}

func TestALoaderErrorReachesTheRender(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, errors.New("upstream is down")
			},
		},
	})
	set := app.startDeferred([]string{"Reviews", "Unknown"}, httptest.NewRequest(http.MethodGet, "/", nil), nil, nil)
	if _, err := set.Await(ir.Fragment{Name: "Reviews"}); err == nil {
		t.Error("the loader failure must reach Await")
	}
	if names := set.slots(); len(names) != 0 {
		t.Errorf("slots = %v, want nothing slotted when nothing was skipped", names)
	}
}

func TestARouteWithoutDeferredFragmentsResolvesToNothing(t *testing.T) {
	app := New(Options{Manifest: &ir.Manifest{
		Plans:  []ir.Plan{{}},
		Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
	}})
	if got := app.resolved(httptest.NewRequest(http.MethodGet, "/", nil), nil, ir.Route{Name: "home"}); got != nil {
		t.Errorf("resolved = %v, want nothing to await", got)
	}
}

func TestASinkWithoutAFlusherStillWrites(t *testing.T) {
	recorder := httptest.NewRecorder()
	out := runtime.Acquire(16)
	defer runtime.Release(out)
	out.Write([]byte("bytes"))
	sink := &sink{writer: recorder, out: out}
	sink.flush()
	sink.flush()
	if recorder.Body.String() != "bytes" {
		t.Errorf("body = %q, want one write", recorder.Body.String())
	}
}

func TestSettleGivesUpWhileTheLoaderRuns(t *testing.T) {
	release := make(chan struct{})
	app := New(Options{
		Manifest: &ir.Manifest{Plans: []ir.Plan{*streamPlan()}, Routes: []ir.Route{{Pattern: "/", Name: "home"}}},
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				<-release
				return slow("late"), nil
			},
		},
	})
	set := app.startDeferred([]string{"Reviews"}, httptest.NewRequest(http.MethodGet, "/", nil), nil, nil)
	if set.Settle(ir.Fragment{Name: "Reviews"}, runtime.NoWait) {
		t.Error("a loader still running has not settled")
	}
	if names := set.slots(); len(names) != 1 || names[0] != "Reviews" {
		t.Errorf("slots = %v, want the skipped fragment recorded", names)
	}
	close(release)
	if _, err := set.Await(ir.Fragment{Name: "Reviews"}); err != nil {
		t.Fatalf("Await: %v", err)
	}
	if !set.Settle(ir.Fragment{Name: "Reviews"}, runtime.NoWait) {
		t.Error("a finished loader settles at once")
	}
	if names := set.slots(); len(names) != 1 {
		t.Errorf("slots = %v, want the record kept and not doubled", names)
	}
}

func TestFindFragmentWalksTheWholeChain(t *testing.T) {
	layout := ir.Plan{Fragments: []ir.Fragment{{Name: "Header"}}}
	page := streamPlan()
	fragment, plan, ok := findFragment([]*ir.Plan{&layout, page}, "Reviews")
	if !ok || plan != page || fragment.Name != "Reviews" {
		t.Errorf("fragment = %+v, ok = %v", fragment, ok)
	}
	if _, _, ok := findFragment([]*ir.Plan{&layout}, "Absent"); ok {
		t.Error("an unknown fragment must not be found")
	}
}

func TestTheStreamReportsAPropsFailure(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{Plans: []ir.Plan{*streamPlan()}, Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}}},
		Config:   streamConfig(config.DeferredTail),
		Props: map[string]PropsProvider{
			"home": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, errors.New("database is away")
			},
		},
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) { return slow("x"), nil },
		},
	})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the error page", recorder.Code)
	}
}

func TestOutOfOrderSkipsAFragmentWhoseLoaderFailed(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{Plans: []ir.Plan{*streamPlan()}, Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}}},
		Config:   streamConfig(config.DeferredTail),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				time.Sleep(20 * time.Millisecond)
				return nil, errors.New("upstream is down")
			},
		},
	})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if !strings.Contains(body, `<gopage-slot name="Reviews">`) {
		t.Errorf("body = %q, want the slot", body)
	}
	if strings.Contains(body, "<template") {
		t.Errorf("body = %q, want no template for a loader that failed", body)
	}
}

func TestTheTailIgnoresAFragmentThatLeftThePlan(t *testing.T) {
	app := New(Options{Manifest: &ir.Manifest{Plans: []ir.Plan{{}}, Routes: []ir.Route{{Name: "home"}}}})
	set := &deferredSet{results: map[string]*result{"Ghost": {done: make(chan struct{})}}}
	set.slot("Ghost")
	out := runtime.Acquire(16)
	defer runtime.Release(out)
	app.writeTail(set, &sink{writer: httptest.NewRecorder(), out: out}, ir.Route{Name: "home"}, runtime.Empty{}, runtime.Options{})
	if out.Len() != 0 {
		t.Errorf("tail = %q, want nothing for a fragment the plan does not hold", out.String())
	}
}

func TestResolvedStartsTheLoadersOffTheStream(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{Plans: []ir.Plan{*streamPlan()}, Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}}},
		Config:   streamConfig(config.DeferredInline),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) { return slow("resolved"), nil },
		},
	})
	deferred := app.resolved(httptest.NewRequest(http.MethodGet, "/", nil), nil, ir.Route{Name: "home", Plan: 0})
	if deferred == nil {
		t.Fatal("a route with a deferred fragment must resolve one")
	}
	props, err := deferred.Await(ir.Fragment{Name: "Reviews"})
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	value, ok := props.Get(nil)
	if !ok || value.Text() != "resolved" {
		t.Errorf("value = %q, ok = %v", value.Text(), ok)
	}
	deferred.Flush()
}

type brokenWriter struct{ http.ResponseWriter }

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("client hung up") }

func TestTheSinkKeepsItsBufferWhenTheWriteFails(t *testing.T) {
	out := runtime.Acquire(16)
	defer runtime.Release(out)
	out.Write([]byte("bytes"))
	sink := &sink{writer: brokenWriter{httptest.NewRecorder()}, out: out}
	sink.flush()
	if out.Len() == 0 {
		t.Error("a failed write must not drop the buffer")
	}
}

func TestTheTailSkipsAFragmentThatCannotRender(t *testing.T) {
	plan := streamPlan()
	plan.Exprs = []ir.ExprNode{{Kind: ir.ExprPath, A: 1}}
	plan.Paths = [][]string{{"Reviews"}, {"Missing"}}
	app := New(Options{Manifest: &ir.Manifest{Plans: []ir.Plan{*plan}, Routes: []ir.Route{{Name: "home", Plan: 0}}}})
	done := make(chan struct{})
	close(done)
	set := &deferredSet{results: map[string]*result{"Reviews": {done: done, props: slow("x")}}}
	set.slot("Reviews")
	out := runtime.Acquire(32)
	defer runtime.Release(out)
	app.writeTail(set, &sink{writer: httptest.NewRecorder(), out: out}, ir.Route{Name: "home", Plan: 0}, runtime.Empty{}, runtime.Options{})
}

func TestTheTailStopsAtAFragmentWhoseLoaderErrored(t *testing.T) {
	app := New(Options{Manifest: &ir.Manifest{Plans: []ir.Plan{*streamPlan()}, Routes: []ir.Route{{Name: "home", Plan: 0}}}})
	done := make(chan struct{})
	close(done)
	set := &deferredSet{results: map[string]*result{
		"Reviews": {done: done, err: errors.New("upstream is down")},
	}}
	set.slot("Reviews")
	out := runtime.Acquire(32)
	defer runtime.Release(out)
	app.writeTail(set, &sink{writer: httptest.NewRecorder(), out: out}, ir.Route{Name: "home", Plan: 0}, runtime.Empty{}, runtime.Options{})
	if strings.Contains(out.String(), "<template") {
		t.Errorf("tail = %q, want nothing for a loader that failed", out.String())
	}
}

func TestABudgetLeavesASlotAndSendsTheTemplateInTheTail(t *testing.T) {
	settings := streamConfig(config.DeferredTail)
	settings.Fragments.Budget = "1ms"
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: settings,
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				time.Sleep(30 * time.Millisecond)
				return slow("late"), nil
			},
		},
	})

	recorder := &recordingWriter{ResponseRecorder: httptest.NewRecorder()}
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	body := recorder.Body.String()
	slot := strings.Index(body, `<gopage-slot name="Reviews">`)
	tail := strings.Index(body, "<p>tail</p>")
	template := strings.Index(body, `<template data-gopage-slot="Reviews">`)
	if slot < 0 || tail < 0 || template < 0 {
		t.Fatalf("body = %q, want the budget to turn the fragment into a slot", body)
	}
	if slot >= tail || tail >= template {
		t.Errorf("body = %q, want slot, then the rest of the page, then the template", body)
	}
}

func TestALoaderInsideItsBudgetStaysInTheDocument(t *testing.T) {
	settings := streamConfig(config.DeferredTail)
	settings.Fragments.Budget = "5s"
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: settings,
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				return slow("late"), nil
			},
		},
	})

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if strings.Contains(body, "gopage-slot") {
		t.Errorf("body = %q, want a loader inside its budget rendered in place", body)
	}
	if !strings.Contains(body, "<b>late</b>") {
		t.Errorf("body = %q, want the fragment body", body)
	}
}

func TestTheTailFillsASlotWhoseLoaderAlreadyFinished(t *testing.T) {
	app := streamApp(t, 0, config.DeferredTail)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	set := app.startDeferred([]string{"Reviews"}, request, nil, nil)
	if _, err := set.Await(ir.Fragment{Name: "Reviews"}); err != nil {
		t.Fatalf("Await: %v", err)
	}
	set.slot("Reviews")

	out := runtime.Acquire(64)
	defer runtime.Release(out)
	recorder := httptest.NewRecorder()
	sink := &sink{writer: recorder, out: out}
	app.writeTail(set, sink, ir.Route{Name: "home", Plan: 0}, runtime.Empty{}, runtime.Options{})
	sink.flush()

	if !strings.Contains(recorder.Body.String(), `<template data-gopage-slot="Reviews">`) {
		t.Errorf("tail = %q, want a template for every slot the render left behind",
			recorder.Body.String())
	}
}

func TestOneFailingLoaderLeavesTheOthersAlone(t *testing.T) {
	failed := make(chan struct{})
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Deferred: map[string]DeferredProvider{
			"Broken": func(*http.Request, Params) (runtime.Accessible, error) {
				defer close(failed)
				return nil, errors.New("upstream is down")
			},
			"Reviews": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				<-failed
				select {
				case <-r.Context().Done():
					return nil, r.Context().Err()
				case <-time.After(50 * time.Millisecond):
					return slow("late"), nil
				}
			},
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	set := app.startDeferred([]string{"Broken", "Reviews"}, request, nil, nil)
	if _, err := set.Await(ir.Fragment{Name: "Broken"}); err == nil {
		t.Fatal("the broken loader must report its failure")
	}
	if _, err := set.Await(ir.Fragment{Name: "Reviews"}); err != nil {
		t.Errorf("err = %v, want a healthy loader to survive a failing sibling", err)
	}
}

func TestTheBufferedPathNeverLeavesASlot(t *testing.T) {
	settings := streamConfig(config.DeferredTail)
	settings.Fragments.Budget = "1ms"
	settings.Nav.Mode = config.NavPartial
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: settings,
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				time.Sleep(20 * time.Millisecond)
				return slow("late"), nil
			},
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(PartialHeader, "l0")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if strings.Contains(body, "gopage-slot") {
		t.Errorf("body = %q, want a buffered answer to wait for every loader", body)
	}
	if !strings.Contains(body, "late") {
		t.Errorf("body = %q, want the fragment body in the partial", body)
	}
}

func TestFetchModeNeverRunsALoaderWhileTheDocumentRenders(t *testing.T) {
	var calls int32
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				atomic.AddInt32(&calls, 1)
				return slow("late"), nil
			},
		},
	})

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	body := recorder.Body.String()
	if !strings.Contains(body, `<gopage-slot name="Reviews" fetch>`) {
		t.Errorf("body = %q, want a slot the client can fetch", body)
	}
	if strings.Contains(body, "late") {
		t.Error("the document must not carry the deferred body")
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("loader calls = %d, want the document rendered without running one", got)
	}
	if recorder.Header().Get("Content-Length") == "" {
		t.Error("a document that never waits carries a content length")
	}
	if got := recorder.Header().Get(CacheHeader); got == StreamStatus {
		t.Error("a document that never waits is not a stream")
	}
	if got := recorder.Header().Get("Vary"); got != varyHeader {
		t.Errorf("vary = %q, want %q", got, varyHeader)
	}
}

func deferredApp(t *testing.T, calls *int32) *App {
	t.Helper()
	return New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				atomic.AddInt32(calls, 1)
				return slow("late"), nil
			},
		},
	})
}

func fragmentRequest(app *App, name string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(FragmentHeader, name)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestAFragmentRequestReturnsOnlyTheFragmentBody(t *testing.T) {
	var calls int32
	recorder := fragmentRequest(deferredApp(t, &calls), "Reviews")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	if body != "<b>late</b>" {
		t.Errorf("body = %q, want the fragment body alone", body)
	}
	if got := recorder.Header().Get("Content-Type"); got != FragmentType {
		t.Errorf("content type = %q, want %q so the dev server leaves it alone", got, FragmentType)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("cache control = %q, want the fragment kept out of shared caches", got)
	}
	if got := recorder.Header().Get("Vary"); got != varyHeader {
		t.Errorf("vary = %q, want %q", got, varyHeader)
	}
	if got := recorder.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Errorf("content length = %q, want %d", got, len(body))
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("loader calls = %d, want exactly one", calls)
	}
}

func TestAFragmentRequestRefusesAnythingItDoesNotOwn(t *testing.T) {
	cases := map[string]string{
		"unknown name":     "Missing",
		"path traversal":   "../etc",
		"empty after trim": " ",
		"a digit first":    "1Reviews",
		"too long":         strings.Repeat("R", maxFragmentName+1),
	}
	for name, header := range cases {
		var calls int32
		recorder := fragmentRequest(deferredApp(t, &calls), header)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, recorder.Code)
		}
		if got := atomic.LoadInt32(&calls); got != 0 {
			t.Errorf("%s: loader calls = %d, want the loader left alone", name, got)
		}
	}
}

func TestAFragmentRequestRefusesAFragmentThatIsNotDeferred(t *testing.T) {
	plan := streamPlan()
	plan.Fragments[0].Deferred = false
	var calls int32
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*plan},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				atomic.AddInt32(&calls, 1)
				return slow("late"), nil
			},
		},
	})
	if got := fragmentRequest(app, "Reviews").Code; got != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a fragment nobody deferred", got)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("loader calls = %d, want none", got)
	}
}

func TestAFragmentRequestWithoutAProviderIsNotFound(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
	})
	if got := fragmentRequest(app, "Reviews").Code; got != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no loader is registered", got)
	}
}

func TestAFragmentRequestReportsALoaderFailure(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, errors.New("upstream is down")
			},
		},
	})
	if got := fragmentRequest(app, "Reviews").Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", got)
	}
}

func TestAFragmentHeadRequestSendsNoBody(t *testing.T) {
	var calls int32
	app := deferredApp(t, &calls)
	request := httptest.NewRequest(http.MethodHead, "/", nil)
	request.Header.Set(FragmentHeader, "Reviews")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Body.Len() != 0 {
		t.Errorf("body = %q, want nothing for HEAD", recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Length"); got == "" {
		t.Error("HEAD still reports the length")
	}
}

func TestAFragmentWithACacheWindowSaysHowLongItLives(t *testing.T) {
	plan := streamPlan()
	plan.Fragments[0].TTL = int64(90 * time.Second)
	var calls int32
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*plan},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				atomic.AddInt32(&calls, 1)
				return slow("late"), nil
			},
		},
	})
	if got := fragmentRequest(app, "Reviews").Header().Get("Cache-Control"); got != "private, max-age=90" {
		t.Errorf("cache control = %q, want the declared window", got)
	}
}

func TestASlotOnlyPortAnswersEmptyAndNeverSettles(t *testing.T) {
	var port runtime.Deferred = slotOnly{}
	props, err := port.Await(ir.Fragment{Name: "Reviews"})
	if err != nil || props == nil {
		t.Errorf("props = %v, err = %v, want an empty accessible", props, err)
	}
	if port.Settle(ir.Fragment{Name: "Reviews"}, 0) {
		t.Error("a fetched fragment never settles while the document renders")
	}
	port.Flush()
}

func TestFragmentNamesTheServerAccepts(t *testing.T) {
	accepted := []string{"Reviews", "_hidden", "Latest2", "a"}
	for _, name := range accepted {
		if !namedFragment(name) {
			t.Errorf("%q was refused, want it accepted", name)
		}
	}
	refused := []string{"", "2Reviews", "Rev-iews", "Rev iews", "../etc", strings.Repeat("R", maxFragmentName+1)}
	for _, name := range refused {
		if namedFragment(name) {
			t.Errorf("%q was accepted, want it refused", name)
		}
	}
}

func TestAFragmentRequestReportsAPropsFailure(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Props: map[string]PropsProvider{
			"home": func(*http.Request, Params) (runtime.Accessible, error) {
				return nil, errors.New("database is away")
			},
		},
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) { return slow("late"), nil },
		},
	})
	if got := fragmentRequest(app, "Reviews").Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want the error page", got)
	}
}

func TestAFragmentRequestReportsABodyThatCannotRender(t *testing.T) {
	plan := streamPlan()
	plan.Exprs = []ir.ExprNode{{Kind: ir.ExprPath, A: 1}}
	plan.Paths = [][]string{{"Reviews"}, {"Missing"}}
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*plan},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) { return slow("late"), nil },
		},
	})
	if got := fragmentRequest(app, "Reviews").Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want the error page", got)
	}
}

func TestAFragmentSurvivesAClientThatHangsUp(t *testing.T) {
	var calls int32
	app := deferredApp(t, &calls)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(FragmentHeader, "Reviews")
	recorder := brokenWriter{httptest.NewRecorder()}
	app.Handler().ServeHTTP(recorder, request)
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("loader calls = %d, want the loader still run", calls)
	}
}

func TestAPanickingDeferredLoaderDoesNotTakeTheProcessDown(t *testing.T) {
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*streamPlan()},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) { panic("upstream exploded") },
		},
	})
	if got := fragmentRequest(app, "Reviews").Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want the error page rather than a dead process", got)
	}

	set := app.startDeferred([]string{"Reviews"}, httptest.NewRequest(http.MethodGet, "/", nil), nil, nil)
	if _, err := set.Await(ir.Fragment{Name: "Reviews"}); err == nil ||
		!strings.Contains(err.Error(), "panicked") {
		t.Errorf("err = %v, want the panic turned into a loader error", err)
	}
}

func TestAPrivateDeferredLoaderKeepsItsFragmentOutOfEveryCache(t *testing.T) {
	plan := streamPlan()
	plan.Fragments[0].TTL = int64(5 * time.Minute)
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*plan},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				cache.From(r.Context()).Private()
				return slow("mine"), nil
			},
		},
	})
	recorder := fragmentRequest(app, "Reviews")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("cache control = %q, want a loader that said private to be believed", got)
	}
}

func TestACacheableDeferredLoaderStillAdvertisesItsAge(t *testing.T) {
	plan := streamPlan()
	plan.Fragments[0].TTL = int64(5 * time.Minute)
	var calls int32
	app := New(Options{
		Manifest: &ir.Manifest{
			Plans:  []ir.Plan{*plan},
			Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
		},
		Config: streamConfig(config.DeferredFetch),
		Deferred: map[string]DeferredProvider{
			"Reviews": func(*http.Request, Params) (runtime.Accessible, error) {
				atomic.AddInt32(&calls, 1)
				return slow("shared"), nil
			},
		},
	})
	if got := fragmentRequest(app, "Reviews").Header().Get("Cache-Control"); got != "private, max-age=300" {
		t.Errorf("cache control = %q", got)
	}
}
