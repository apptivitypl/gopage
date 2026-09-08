package server

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"golang.org/x/sync/errgroup"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/reply"
	"github.com/apptivitypl/gopage/internal/runtime"
)

type recorded struct {
	policy *cache.Recorder
	answer *reply.Recorder
	props  runtime.Accessible
}

func apart(r *http.Request) (*http.Request, *recorded) {
	own := &recorded{policy: cache.NewRecorder(), answer: reply.NewRecorder()}
	return r.WithContext(reply.WithRecorder(cache.WithRecorder(r.Context(), own.policy), own.answer)), own
}

func (a *App) recovering(name string, run func() error) func() error {
	return func() (err error) {
		defer func() {
			if raised := recover(); raised != nil {
				err = fmt.Errorf("loader %s panicked: %v", name, raised)
				a.logger.Error("loader panicked", "loader", name, "error", err, "stack", string(debug.Stack()))
			}
		}()
		return run()
	}
}

func (a *App) loadingFrom(route ir.Route, from int) []int {
	if len(a.layouts) == 0 {
		return nil
	}
	var found []int
	for index := max(from, 0); index < len(route.LayoutChain); index++ {
		if _, ok := a.layouts[route.LayoutChain[index]]; ok {
			found = append(found, index)
		}
	}
	return found
}

func (a *App) loadChain(w http.ResponseWriter, r *http.Request, route ir.Route,
	params Params, from int) (runtime.Accessible, []runtime.Accessible, error) {
	if len(a.loadingFrom(route, from)) == 0 {
		props, err := a.pageProps(w, r, route, params)
		return props, nil, err
	}
	var page *recorded
	layouts, err := a.fanOut(r, route, params, from, func(group *errgroup.Group, shared *http.Request) {
		request, own := apart(shared)
		page = own
		group.Go(a.recovering(route.Name, func() error {
			props, err := a.pageProps(w, request, route, params)
			own.props = props
			return err
		}))
	})
	if err != nil {
		return nil, nil, err
	}
	a.adopt(r, page)
	return page.props, layouts, nil
}

func (a *App) layoutChain(r *http.Request, route ir.Route, params Params) ([]runtime.Accessible, error) {
	return a.fanOut(r, route, params, 0, nil)
}

func (a *App) fanOut(r *http.Request, route ir.Route, params Params, from int,
	page func(*errgroup.Group, *http.Request)) ([]runtime.Accessible, error) {
	loading := a.loadingFrom(route, from)
	if len(loading) == 0 && page == nil {
		return nil, nil
	}
	group, ctx := errgroup.WithContext(r.Context())
	shared := r.WithContext(ctx)
	if page != nil {
		page(group, shared)
	}
	held := make([]*recorded, len(loading))
	for slot, index := range loading {
		hook := a.layouts[route.LayoutChain[index]]
		request, own := apart(shared)
		held[slot] = own
		group.Go(a.recovering(hook.name, func() error {
			props, err := hook.load(request, params)
			own.props = props
			return err
		}))
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	layouts := make([]runtime.Accessible, len(route.LayoutChain)-from)
	for slot, index := range loading {
		a.adopt(r, held[slot])
		layouts[index-from] = held[slot].props
	}
	return layouts, nil
}

func (a *App) adopt(r *http.Request, held *recorded) {
	cache.From(r.Context()).Merge(held.policy)
	reply.From(r.Context()).Merge(held.answer)
}
