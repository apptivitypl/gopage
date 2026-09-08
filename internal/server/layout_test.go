package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apptivitypl/gopage/internal/action"
	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/form"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/reply"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func chromeManifest() *ir.Manifest {
	held := manifest()
	held.Plans[0] = ir.Plan{
		Ops: []ir.Op{
			{Kind: ir.OpStatic, A: 0, B: 6},
			{Kind: ir.OpText, A: 0},
			{Kind: ir.OpStatic, A: 6, B: 7},
			{Kind: ir.OpOutlet},
		},
		Exprs:    []ir.ExprNode{{Kind: ir.ExprPath, A: 0}},
		Paths:    [][]string{{runtime.LayoutRoot, "Home"}},
		Blob:     []byte("<main></main>"),
		Capacity: 64,
	}
	held.Routes[1].LayoutChain = []uint32{0}
	held.Layouts = []ir.Layout{{Name: "layout", Plan: 0}}
	return held
}

func chromeApp(t *testing.T, load PropsProvider) *App {
	t.Helper()
	settings := config.Default()
	settings.Nav.Mode = config.NavPartial
	return New(Options{
		Manifest: chromeManifest(),
		Config:   settings,
		Cache:    cache.New(cache.Options{Limit: 1 << 20}),
		Layouts:  map[string]PropsProvider{"layout": load},
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				cache.From(r.Context()).TTL(time.Hour)
				return runtime.Empty{}, nil
			},
		},
	})
}

func TestALayoutLoaderFillsTheChrome(t *testing.T) {
	app := chromeApp(t, func(*http.Request, Params) (runtime.Accessible, error) {
		return runtime.Map{"Home": runtime.String("/start")}, nil
	})
	body := get(t, app.Handler(), "/").Body.String()
	if !strings.Contains(body, "<main>/start") {
		t.Errorf("body = %q, want the layout to render its own props", body)
	}
}

func TestALayoutLoaderShortensTheFreshness(t *testing.T) {
	app := chromeApp(t, func(r *http.Request, _ Params) (runtime.Accessible, error) {
		cache.From(r.Context()).TTL(time.Minute).Tag("nav")
		return runtime.Map{"Home": runtime.String("/")}, nil
	})
	answer := get(t, app.Handler(), "/")
	if got := answer.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=60") {
		t.Errorf("cache-control = %q, want the layout to win with the shorter freshness", got)
	}
	if app.Invalidate("nav") != 1 {
		t.Error("the layout's tag must reach the page entry")
	}
}

func TestAPrivateLayoutKeepsThePageOutOfTheCache(t *testing.T) {
	app := chromeApp(t, func(r *http.Request, _ Params) (runtime.Accessible, error) {
		cache.From(r.Context()).Private()
		return runtime.Map{"Home": runtime.String("/")}, nil
	})
	handler := app.Handler()
	get(t, handler, "/")
	if got := get(t, handler, "/").Header().Get(CacheHeader); got == "hit" {
		t.Error("a private layout must keep the page out of the store")
	}
}

func TestAFailingLayoutLoaderFailsThePage(t *testing.T) {
	app := chromeApp(t, func(*http.Request, Params) (runtime.Accessible, error) {
		return nil, errors.New("no nav")
	})
	if got := get(t, app.Handler(), "/").Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d", got)
	}
}

func TestAHeldLayoutDoesNotLoadAgain(t *testing.T) {
	calls := 0
	app := chromeApp(t, func(*http.Request, Params) (runtime.Accessible, error) {
		calls++
		return runtime.Map{"Home": runtime.String("/")}, nil
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(PartialHeader, "/listings/7")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if calls != 0 {
		t.Errorf("the layout loaded %d times, want none when the browser already holds it", calls)
	}
}

func TestARouteWithoutLayoutProvidersCostsNothing(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	held, err := app.layoutChain(httptest.NewRequest(http.MethodGet, "/", nil),
		app.manifest.Routes[0], Params{})
	if err != nil || held != nil {
		t.Errorf("layouts = %v, err = %v", held, err)
	}
	if got := layoutPlans(nil, nil); got != nil {
		t.Errorf("plans = %v", got)
	}
	if got := layoutPlans(chromeManifest(), map[string]PropsProvider{"absent": nil}); len(got) != 0 {
		t.Errorf("plans = %v, want nothing for a layout the registry does not name", got)
	}
}

func TestAFragmentInALayoutSeesItsLayoutProps(t *testing.T) {
	held := chromeManifest()
	held.Plans[0].Fragments = []ir.Fragment{{Name: "Nav", Deferred: true}}
	held.Plans[0].Ops = []ir.Op{
		{Kind: ir.OpStatic, A: 0, B: 6},
		{Kind: ir.OpFragment, A: 0, B: 3},
		{Kind: ir.OpText, A: 0},
		{Kind: ir.OpStatic, A: 6, B: 7},
		{Kind: ir.OpOutlet},
	}
	settings := config.Default()
	settings.Fragments.Deferred = config.DeferredFetch
	app := New(Options{
		Manifest: held,
		Config:   settings,
		Layouts: map[string]PropsProvider{"layout": func(*http.Request, Params) (runtime.Accessible, error) {
			return runtime.Map{"Home": runtime.String("/start")}, nil
		}},
		Deferred: map[string]DeferredProvider{
			"Nav": func(*http.Request, Params) (runtime.Accessible, error) { return runtime.Empty{}, nil },
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(FragmentHeader, "Nav")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Body.String(); got != "/start" {
		t.Errorf("fragment = %q, want the layout props", got)
	}
}

func TestAFailingLayoutStopsTheFragment(t *testing.T) {
	held := chromeManifest()
	held.Plans[0].Fragments = []ir.Fragment{{Name: "Nav", Deferred: true}}
	held.Plans[0].Ops = []ir.Op{
		{Kind: ir.OpFragment, A: 0, B: 2},
		{Kind: ir.OpText, A: 0},
		{Kind: ir.OpOutlet},
	}
	settings := config.Default()
	settings.Fragments.Deferred = config.DeferredFetch
	app := New(Options{
		Manifest: held,
		Config:   settings,
		Layouts: map[string]PropsProvider{"layout": func(*http.Request, Params) (runtime.Accessible, error) {
			return nil, errors.New("no nav")
		}},
		Deferred: map[string]DeferredProvider{
			"Nav": func(*http.Request, Params) (runtime.Accessible, error) { return runtime.Empty{}, nil },
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(FragmentHeader, "Nav")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the failure to surface", recorder.Code)
	}
}

func brokenChrome(t *testing.T, mode config.NavMode) *App {
	t.Helper()
	settings := config.Default()
	settings.Nav.Mode = mode
	held := chromeManifest()
	held.Plans[1].Fragments = []ir.Fragment{{Name: "Late", Deferred: true}}
	return New(Options{
		Manifest: held,
		Config:   settings,
		Layouts: map[string]PropsProvider{"layout": func(*http.Request, Params) (runtime.Accessible, error) {
			return nil, errors.New("no nav")
		}},
	})
}

func TestAFailingLayoutStopsAPartial(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(PartialHeader, "/docs/guide")
	recorder := httptest.NewRecorder()
	brokenChrome(t, config.NavPartial).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestALayoutTheRegistryDoesNotNameIsSkipped(t *testing.T) {
	held := chromeManifest()
	held.Layouts = []ir.Layout{{Name: "other", Plan: 0}}
	app := New(Options{
		Manifest: held,
		Layouts: map[string]PropsProvider{"other": func(*http.Request, Params) (runtime.Accessible, error) {
			return runtime.Map{"Home": runtime.String("/x")}, nil
		}},
	})
	route := app.manifest.Routes[0]
	route.LayoutChain = []uint32{0, 1}
	held2, err := app.layoutChain(httptest.NewRequest(http.MethodGet, "/", nil), route, Params{})
	if err != nil {
		t.Fatalf("layoutChain: %v", err)
	}
	if len(held2) != 2 || held2[0] == nil || held2[1] != nil {
		t.Errorf("layouts = %v, want only the layout the registry names", held2)
	}
}

func TestAFailingLayoutStopsARerender(t *testing.T) {
	held := formManifest()
	held.Routes[len(held.Routes)-1].LayoutChain = []uint32{0}
	held.Layouts = []ir.Layout{{Name: "layout", Plan: 0}}
	app := New(Options{
		Manifest: held,
		Layouts: map[string]PropsProvider{"layout": func(*http.Request, Params) (runtime.Accessible, error) {
			return nil, errors.New("no nav")
		}},
		Submit: map[string]SubmitProvider{"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
			return nil, form.Result{Submitted: true, Errors: map[string][]string{"Name": {"x"}}}, nil
		}},
	})
	token, masked := validToken(t)
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("__csrf="+masked))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(token)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestAFailingLayoutStopsAStream(t *testing.T) {
	settings := config.Default()
	settings.Fragments.Deferred = config.DeferredTail
	held := chromeManifest()
	held.Plans[1].Fragments = []ir.Fragment{{Name: "Late", Deferred: true}}
	app := New(Options{
		Manifest: held,
		Config:   settings,
		Layouts: map[string]PropsProvider{"layout": func(*http.Request, Params) (runtime.Accessible, error) {
			return nil, errors.New("no nav")
		}},
		Deferred: map[string]DeferredProvider{
			"Late": func(*http.Request, Params) (runtime.Accessible, error) { return runtime.Empty{}, nil },
		},
	})
	if got := get(t, app.Handler(), "/").Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d", got)
	}
}

func TestLayoutHelpersAnswerWhenNothingIsDeclared(t *testing.T) {
	if got := WithBuckets(t.Context(), nil); got != t.Context() {
		t.Error("no buckets leaves the context alone")
	}
	app := New(Options{Manifest: chromeManifest()})
	route := app.manifest.Routes[0]
	held, err := app.layoutOf(httptest.NewRequest(http.MethodGet, "/", nil), route, Params{}, &app.manifest.Plans[1])
	if err != nil || held != nil {
		t.Errorf("layouts = %v, err = %v, want nothing for a plan that is no layout", held, err)
	}
}

func TestChainIndexFindsThePlanOrSaysSo(t *testing.T) {
	held := chromeManifest()
	chain := []*ir.Plan{&held.Plans[0], &held.Plans[1]}
	if index, ok := chainIndex(chain, &held.Plans[1]); !ok || index != 1 {
		t.Errorf("index = %d, ok = %v", index, ok)
	}
	if _, ok := chainIndex(chain, &held.Plans[2]); ok {
		t.Error("a plan outside the chain is not in it")
	}
}

func TestTheChainLoadsAtOnce(t *testing.T) {
	const wait = 60 * time.Millisecond
	held := chromeManifest()
	held.Plans = append(held.Plans, ir.Plan{
		Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 3}, {Kind: ir.OpOutlet}},
		Blob:     []byte("<i>"),
		Capacity: 32,
	})
	inner := uint32(len(held.Plans) - 1)
	held.Routes[0].LayoutChain = []uint32{0, inner}
	held.Layouts = []ir.Layout{{Name: "layout", Plan: 0}, {Name: "layout.inner", Plan: inner}}
	slow := func(*http.Request, Params) (runtime.Accessible, error) {
		time.Sleep(wait)
		return runtime.Map{"Home": runtime.String("/")}, nil
	}
	app := New(Options{
		Manifest: held,
		Layouts:  map[string]PropsProvider{"layout": slow, "layout.inner": slow},
		Props: map[string]PropsProvider{
			"index": func(*http.Request, Params) (runtime.Accessible, error) {
				time.Sleep(wait)
				return runtime.Empty{}, nil
			},
		},
	})
	at := time.Now()
	if got := get(t, app.Handler(), "/").Code; got != http.StatusOK {
		t.Fatalf("status = %d", got)
	}
	if took := time.Since(at); took > 2*wait {
		t.Errorf("the chain took %v for three loaders of %v each, want them to overlap", took, wait)
	}
}

func TestAPanickingLayoutIsAnError(t *testing.T) {
	app := chromeApp(t, func(*http.Request, Params) (runtime.Accessible, error) {
		panic("no nav")
	})
	if got := get(t, app.Handler(), "/").Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want a panic in a loader to answer rather than end the process", got)
	}
}

func TestTheChainMergesInOrder(t *testing.T) {
	app := chromeApp(t, func(r *http.Request, _ Params) (runtime.Accessible, error) {
		reply.From(r.Context()).Header().Set("X-Owner", "layout")
		reply.From(r.Context()).Header().Set("X-Layout", "yes")
		return runtime.Map{"Home": runtime.String("/")}, nil
	})
	app.props["index"] = func(r *http.Request, _ Params) (runtime.Accessible, error) {
		reply.From(r.Context()).Header().Set("X-Owner", "page")
		return runtime.Empty{}, nil
	}
	for range 20 {
		answer := get(t, app.Handler(), "/")
		if got := answer.Header().Get("X-Owner"); got != "page" {
			t.Fatalf("x-owner = %q, want the page to win over the layout every time", got)
		}
		if got := answer.Header().Get("X-Layout"); got != "yes" {
			t.Fatalf("x-layout = %q, want what only the layout set", got)
		}
	}
}

func TestALayoutFragmentWithoutALoaderRendersNothingExtra(t *testing.T) {
	held := chromeManifest()
	held.Layouts = nil
	app := New(Options{
		Manifest: held,
		Layouts:  map[string]PropsProvider{"absent": nil},
	})
	app.layouts = map[uint32]layoutHook{0: {name: "layout"}}
	props, err := app.callLayout(httptest.NewRequest(http.MethodGet, "/", nil), Params{}, 0)
	if err != nil || props != nil {
		t.Errorf("props = %v, err = %v, want nothing for a plan with no provider", props, err)
	}
}

func TestRenderPassesOnAFailingLayout(t *testing.T) {
	app := chromeApp(t, func(*http.Request, Params) (runtime.Accessible, error) {
		return nil, errors.New("no nav")
	})
	if _, err := app.RenderRoute(t.Context(), app.manifest.Routes[0], Params{}); err == nil {
		t.Error("a failing layout must reach the caller of Render")
	}
}
