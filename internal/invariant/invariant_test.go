package invariant

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/apptivitypl/rill/internal/cache"
	"github.com/apptivitypl/rill/internal/compile"
	"github.com/apptivitypl/rill/internal/config"
	"github.com/apptivitypl/rill/internal/diag"
	rt "github.com/apptivitypl/rill/internal/runtime"
	"github.com/apptivitypl/rill/internal/server"
)

const layout = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8">{% meta %}</head>
<body>
<nav>menu</nav>
<main>{% outlet %}</main>
</body>
</html>`

const docsLayout = `<aside>docs menu</aside><section>{% outlet %}</section>`

func project() fstest.MapFS {
	return fstest.MapFS{
		"rill.jsonc":               &fstest.MapFile{Data: []byte("{\"nav\": {\"mode\": \"partial\"}, \"i18n\": {\"locales\": [\"en\", \"pl\"]}}")},
		"app/layout.rill":          &fstest.MapFile{Data: []byte(layout)},
		"app/page.rill":            &fstest.MapFile{Data: []byte(`<h1>home</h1>`)},
		"app/docs/layout.rill":     &fstest.MapFile{Data: []byte(docsLayout)},
		"app/docs/page.rill":       &fstest.MapFile{Data: []byte(`<h1>docs</h1>`)},
		"app/docs/guide/page.rill": &fstest.MapFile{Data: []byte(`<h1>guide</h1><p>a page under two layouts</p>`)},
		"app/private/page.rill":    &fstest.MapFile{Data: []byte(privatePage)},
		"locales/en.json":          &fstest.MapFile{Data: []byte(`{"hello": "hello"}`)},
		"locales/pl.json":          &fstest.MapFile{Data: []byte(`{"hello": "czesc"}`)},
	}
}

const privatePage = "---\n" + `type Props struct {
	Viewer Viewer ` + "`rill:\"private\"`" + `
}

type Viewer struct {
	Email string
}
` + "---\n" + `<p>{{ Viewer.Email }}</p>`

func app(t *testing.T) *server.App {
	t.Helper()
	fsys := project()
	var bag diag.Bag
	result, err := compile.Compile(fsys, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	settings, err := config.Load(fsys)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return server.New(server.Options{
		Manifest: result.Manifest,
		Config:   settings,
		Cache:    cache.New(cache.Options{Limit: 4 << 20}),
		Props: map[string]server.PropsProvider{
			"private": func(*http.Request, server.Params) (rt.Accessible, error) {
				return rt.Map{"Viewer": rt.Object(viewer{})}, nil
			},
		},
	})
}

type viewer struct{}

func (viewer) Names() []string { return []string{"Email"} }

func (viewer) Get(path []string) (rt.Value, bool) {
	if len(path) == 1 && path[0] == "Email" {
		return rt.String("ada@example.com"), true
	}
	return rt.Nil(), false
}

func fetch(t *testing.T, handler http.Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestI3ARenderFromAnyRootRebuildsTheSameDocument(t *testing.T) {
	handler := app(t).Handler()
	whole := fetch(t, handler, "/docs/guide", nil)
	if whole.Code != http.StatusOK {
		t.Fatalf("status = %d", whole.Code)
	}
	full, err := Normalize(whole.Body.String())
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	for _, from := range []string{"/docs", "/"} {
		partial := fetch(t, handler, "/docs/guide", map[string]string{server.PartialHeader: from})
		if partial.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", from, partial.Code)
		}
		level := partial.Header().Get(server.LevelHeader)
		patched, err := Normalize(splice(t, whole.Body.String(), partial.Body.String(), level))
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if patched != full {
			t.Errorf("from %s the patched document differs\n patched: %s\n whole:   %s", from, patched, full)
		}
	}
}

func splice(t *testing.T, document, fragment, level string) string {
	t.Helper()
	if level == "0" {
		return fragment
	}
	open := rt.MarkerOpen + previous(level) + "-->"
	closing := rt.MarkerClose + previous(level) + "-->"
	start := strings.Index(document, open)
	end := strings.Index(document, closing)
	if start < 0 || end < 0 {
		t.Fatalf("the document carries no marker for level %s", level)
	}
	return document[:start+len(open)] + fragment + document[end:]
}

func previous(level string) string {
	switch level {
	case "2":
		return "1"
	default:
		return "0"
	}
}

func TestI4ARequestWithoutTheHeaderIsAWholeDocument(t *testing.T) {
	handler := app(t).Handler()
	for _, header := range []map[string]string{nil, {server.PartialHeader: ""}} {
		got := fetch(t, handler, "/docs/guide", header)
		if got.Code != http.StatusOK {
			t.Fatalf("status = %d", got.Code)
		}
		body := got.Body.String()
		if !strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, "<nav>menu</nav>") {
			t.Errorf("body = %q, want the whole document", body)
		}
	}
}

func TestI4ABrokenPartialHeaderStillAnswersAWholeChain(t *testing.T) {
	handler := app(t).Handler()
	got := fetch(t, handler, "/docs/guide", map[string]string{server.PartialHeader: "/nowhere"})
	if got.Code != http.StatusOK {
		t.Fatalf("status = %d", got.Code)
	}
	if got.Header().Get(server.LevelHeader) != "0" {
		t.Errorf("level = %q, want nothing kept", got.Header().Get(server.LevelHeader))
	}
	if !strings.Contains(got.Body.String(), "<nav>menu</nav>") {
		t.Errorf("body = %q, want the root layout included", got.Body.String())
	}
}

func TestI2APrivateValueCannotEnterACachedFragment(t *testing.T) {
	fsys := project()
	fsys["app/private/page.rill"] = &fstest.MapFile{Data: []byte(
		strings.Replace(privatePage, "<p>{{ Viewer.Email }}</p>",
			`{% fragment "leak" cache="5m" %}<p>{{ Viewer.Email }}</p>{% endfragment %}`, 1))}
	var bag diag.Bag
	if _, err := compile.Compile(fsys, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	found := false
	for _, item := range bag.Sorted() {
		if item.Code == diag.C503 {
			found = true
			if item.Span.Start == item.Span.End {
				t.Errorf("span = %+v, want the read pointed at", item.Span)
			}
		}
	}
	if !found {
		t.Fatalf("diagnostics = %v, want C503", bag.Sorted())
	}
}

func TestI1TheCacheStaysInsideItsBudget(t *testing.T) {
	limit := int64(64 << 10)
	store := cache.New(cache.Options{Limit: limit})
	body := make([]byte, 4<<10)
	for i := range 200 {
		key := "page/" + strings.Repeat("x", i%17) + string(rune('a'+i%26))
		_, _, err := store.Do(key, func(bool) (cache.Value, cache.Policy, error) {
			return cache.Value{Body: body}, cache.Policy{TTL: time.Hour}, nil
		})
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		if stats := store.Stats(); stats.Bytes > limit {
			t.Fatalf("the cache holds %d bytes, over its %d budget", stats.Bytes, limit)
		}
	}
	if store.Stats().Evicted == 0 {
		t.Error("the budget was never enforced")
	}
}

func TestI1AServedAppDoesNotGrowWithoutBound(t *testing.T) {
	handler := app(t).Handler()
	for range 200 {
		fetch(t, handler, "/docs/guide", nil)
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	for range 2000 {
		fetch(t, handler, "/docs/guide", nil)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if growth > 8<<20 {
		t.Errorf("the heap grew by %d bytes over 2000 requests", growth)
	}
}

func cachedFragment(t *testing.T, body string) *diag.Bag {
	t.Helper()
	fsys := project()
	fsys["app/private/page.rill"] = &fstest.MapFile{Data: []byte(
		strings.Replace(privatePage, "<p>{{ Viewer.Email }}</p>",
			`{% fragment "leak" cache="5m" %}`+body+`{% endfragment %}`, 1))}
	var bag diag.Bag
	if _, err := compile.Compile(fsys, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return &bag
}

func TestI2AVisitorTokenCannotEnterACachedFragment(t *testing.T) {
	for name, body := range map[string]string{
		"a form":    `<Form action="/apply"></Form>`,
		"the token": `{{ form.Token }}`,
		"a flash":   `<p>{{ flash }}</p>`,
	} {
		bag := cachedFragment(t, body)
		found := false
		for _, item := range bag.Sorted() {
			if item.Code == diag.C503 {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: diagnostics = %v, want C503", name, bag.Sorted())
		}
	}
}

const deferredInsideCache = "---\n" + `type Props struct {
	Heading string
}

type Review struct {
	Author string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}
` + "---\n" + `{% fragment "shell" cache="5m" %}<h1>{{ Heading }}</h1>{% fragment "Reviews" defer %}<b>deferred</b>{% endfragment %}<i>after</i>{% endfragment %}<p>tail</p>`

func TestI1ACachedFragmentSurvivesADeferredFlushInsideIt(t *testing.T) {
	fsys := fstest.MapFS{
		"rill.jsonc":    &fstest.MapFile{Data: []byte(`{"fragments": {"deferred": "tail"}}`)},
		"app/page.rill": &fstest.MapFile{Data: []byte(deferredInsideCache)},
	}
	var bag diag.Bag
	result, err := compile.Compile(fsys, &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	settings, err := config.Load(fsys)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	served := server.New(server.Options{
		Manifest: result.Manifest,
		Config:   settings,
		Cache:    cache.New(cache.Options{Limit: 4 << 20}),
		Props: map[string]server.PropsProvider{
			"index": func(*http.Request, server.Params) (rt.Accessible, error) {
				return rt.Map{"Heading": rt.String("hello")}, nil
			},
		},
		Deferred: map[string]server.DeferredProvider{
			"Reviews": func(*http.Request, server.Params) (rt.Accessible, error) {
				return rt.Map{}, nil
			},
		},
	})
	handler := served.Handler()

	var bodies []string
	for range 2 {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d", recorder.Code)
		}
		bodies = append(bodies, recorder.Body.String())
	}
	for i, body := range bodies {
		if !strings.Contains(body, "<h1>hello</h1>") {
			t.Errorf("request %d lost the head of the cached fragment: %q", i+1, body)
		}
		if !strings.Contains(body, "<i>after</i>") {
			t.Errorf("request %d lost the tail of the cached fragment: %q", i+1, body)
		}
	}
}
