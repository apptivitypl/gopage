package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/apptivitypl/rill/internal/cache"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

const (
	FragmentHeader  = "RILL-Fragment"
	FragmentType    = "text/vnd.rill-fragment"
	maxFragmentName = 64
)

var varyHeader = FragmentHeader + ", " + PartialHeader

type slotOnly struct{}

func (slotOnly) Await(ir.Fragment) (runtime.Accessible, error) {
	return runtime.Empty{}, nil
}

func (slotOnly) Settle(ir.Fragment, runtime.Budget) bool {
	return false
}

func (slotOnly) Flush() {}

func vary(w http.ResponseWriter) {
	w.Header().Set("Vary", varyHeader)
}

func keepPrivate(w http.ResponseWriter) {
	w.Header()["Cache-Control"] = privateDirective
}

func namedFragment(name string) bool {
	if name == "" || len(name) > maxFragmentName {
		return false
	}
	for index, letter := range name {
		switch {
		case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z':
		case letter == '_':
		case index > 0 && letter >= '0' && letter <= '9':
		default:
			return false
		}
	}
	return true
}

func (a *App) writeFragment(w http.ResponseWriter, r *http.Request, route ir.Route, params Params, name string) {
	fragment, plan, ok := a.deferredIn(route, name)
	if !ok {
		a.fail(w, r, ir.FallbackNotFound, http.StatusNotFound)
		return
	}
	provider, ok := a.deferred[name]
	if !ok {
		a.fail(w, r, ir.FallbackNotFound, http.StatusNotFound)
		return
	}

	recorder := cache.NewRecorder()
	r = r.WithContext(cache.WithRecorder(r.Context(), recorder))
	props, err := a.pageProps(w, r, route, params)
	if err != nil {
		a.failRender(w, r, route, err)
		return
	}
	request := r.WithContext(WithTranslator(r.Context(), a.translator(r)))
	held, err := provider(request, params)
	if err != nil {
		a.logger.Error("deferred loader failed", "fragment", fragment.Name, "error", err)
		a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
		return
	}

	page := a.options(a.fragmentHook(r), LocaleOf(r))
	body := runtime.Options{Fragments: page.Fragments, Catalog: page.Catalog, Plural: page.Plural}
	out := runtime.Acquire(plan.Capacity)
	defer runtime.Release(out)
	rooted := runtime.WithRoot(props, fragment.Name, held)
	if err := runtime.RenderFragment(plan, fragment, rooted, out, body); err != nil {
		a.logger.Error("deferred render failed", "fragment", fragment.Name, "error", err)
		a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
		return
	}

	vary(w)
	w.Header().Set("Content-Type", FragmentType)
	w.Header().Set("Cache-Control", freshness(fragment, recorder))
	w.Header().Set("Content-Length", strconv.Itoa(out.Len()))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(out.Bytes()); err != nil {
		a.logger.Error("write failed", "path", r.URL.Path, "error", err)
	}
}

func (a *App) deferredIn(route ir.Route, name string) (ir.Fragment, *ir.Plan, bool) {
	if !namedFragment(name) {
		return ir.Fragment{}, nil, false
	}
	fragment, plan, ok := findFragment(a.chain(route), name)
	if !ok || !fragment.Deferred {
		return ir.Fragment{}, nil, false
	}
	return fragment, plan, true
}

func freshness(fragment ir.Fragment, recorder *cache.Recorder) string {
	if !fragment.Cacheable() || !recorder.Shared() {
		return "private, no-store"
	}
	return "private, max-age=" + strconv.Itoa(int(time.Duration(fragment.TTL).Seconds()))
}
