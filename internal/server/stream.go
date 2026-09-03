package server

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/sonquer/rill/internal/ir"
	"github.com/sonquer/rill/internal/runtime"
)

type DeferredProvider func(*http.Request, Params) (runtime.Accessible, error)

type result struct {
	props runtime.Accessible
	err   error
	done  chan struct{}
}

type deferredSet struct {
	results map[string]*result
	slotted []string
	flush   func()
	mu      sync.Mutex
}

func (d *deferredSet) Await(fragment ir.Fragment) (runtime.Accessible, error) {
	held, ok := d.results[fragment.Name]
	if !ok {
		return runtime.Empty{}, nil
	}
	<-held.done
	return held.props, held.err
}

func (d *deferredSet) Settle(fragment ir.Fragment, budget runtime.Budget) bool {
	held, ok := d.results[fragment.Name]
	if !ok {
		return true
	}
	if budget.Unlimited() {
		<-held.done
		return true
	}
	if budget > 0 {
		timer := time.NewTimer(time.Duration(budget))
		defer timer.Stop()
		select {
		case <-held.done:
			return true
		case <-timer.C:
		}
	} else {
		select {
		case <-held.done:
			return true
		default:
		}
	}
	d.slot(fragment.Name)
	return false
}

func (d *deferredSet) slot(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !slices.Contains(d.slotted, name) {
		d.slotted = append(d.slotted, name)
	}
}

func (d *deferredSet) slots() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.slotted)
}

func (d *deferredSet) Flush() {
	if d.flush == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.flush()
}

func (a *App) deferredFor(route ir.Route) []string {
	return a.deferrals[route.Name]
}

func (a *App) startDeferred(names []string, r *http.Request, params Params, flush func()) *deferredSet {
	set := &deferredSet{results: make(map[string]*result, len(names)), flush: flush}
	request := r.WithContext(WithTranslator(r.Context(), a.translator(r)))
	for _, name := range names {
		provider, ok := a.deferred[name]
		if !ok {
			continue
		}
		held := &result{done: make(chan struct{})}
		set.results[name] = held
		go func() {
			defer close(held.done)
			defer func() {
				if raised := recover(); raised != nil {
					held.err = fmt.Errorf("loader %s panicked: %v", name, raised)
					a.logger.Error("deferred loader panicked",
						"fragment", name, "error", held.err, "stack", string(debug.Stack()))
				}
			}()
			held.props, held.err = provider(request, params)
		}()
	}
	return set
}

const StreamStatus = "stream"

func (a *App) resolved(r *http.Request, params Params, route ir.Route) runtime.Deferred {
	names := a.deferredFor(route)
	if len(names) == 0 {
		return nil
	}
	if a.config.Fragments.Fetches() {
		return slotOnly{}
	}
	return a.startDeferred(names, r, params, nil)
}

func (a *App) streamPage(w http.ResponseWriter, r *http.Request, route ir.Route, params Params, names []string) {
	props, err := a.pageProps(w, r, route, params)
	if err != nil {
		a.failRender(w, r, route, err)
		return
	}
	vary(w)
	keepPrivate(w)
	w.Header().Set(CacheHeader, StreamStatus)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}

	out := runtime.Acquire(runtime.Capacity(a.chain(route)))
	defer runtime.Release(out)
	sink := &sink{writer: w, out: out}
	if flusher, ok := w.(http.Flusher); ok {
		sink.flusher = flusher
	}
	set := a.startDeferred(names, r, params, sink.flush)

	opts := a.options(a.fragmentHook(r), LocaleOf(r))
	opts.Deferred = set
	opts.Budget = runtime.Budget(a.config.Fragments.Wait())
	opts.Preload = a.preloads[route.Name].tags
	if err := runtime.RenderOptions(a.chain(route), props, out, opts); err != nil {
		a.logger.Error("stream failed", "route", route.Name, "error", err)
		return
	}
	a.writeTail(set, sink, route, props, opts)
	sink.flush()
}

func (a *App) writeTail(set *deferredSet, sink *sink, route ir.Route, props runtime.Accessible, opts runtime.Options) {
	chain := a.chain(route)
	for _, name := range set.slots() {
		fragment, plan, ok := findFragment(chain, name)
		if !ok {
			continue
		}
		held, err := set.Await(fragment)
		if err != nil {
			a.logger.Error("deferred loader failed", "fragment", name, "error", err)
			continue
		}
		sink.out.Write([]byte(runtime.TemplateOpen(name)))
		body := runtime.Options{Fragments: opts.Fragments, Catalog: opts.Catalog, Plural: opts.Plural}
		if err := runtime.RenderFragment(plan, fragment, runtime.WithRoot(props, name, held), sink.out, body); err != nil {
			a.logger.Error("deferred render failed", "fragment", name, "error", err)
		}
		sink.out.Write([]byte(runtime.TemplateClose()))
		sink.flush()
	}
}

func findFragment(chain []*ir.Plan, name string) (ir.Fragment, *ir.Plan, bool) {
	for _, plan := range chain {
		for _, fragment := range plan.Fragments {
			if fragment.Name == name {
				return fragment, plan, true
			}
		}
	}
	return ir.Fragment{}, nil, false
}

type sink struct {
	writer  http.ResponseWriter
	out     *runtime.Buffer
	flusher http.Flusher
}

func (s *sink) flush() {
	if s.out.Len() == 0 {
		return
	}
	if _, err := s.writer.Write(s.out.Bytes()); err != nil {
		return
	}
	s.out.Reset()
	if s.flusher != nil {
		s.flusher.Flush()
	}
}
