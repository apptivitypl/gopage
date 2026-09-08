package server

import (
	"net/http"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

type layoutHook struct {
	name string
	load PropsProvider
}

func layoutPlans(manifest *ir.Manifest, providers map[string]PropsProvider) map[uint32]layoutHook {
	if manifest == nil || len(providers) == 0 {
		return nil
	}
	held := make(map[uint32]layoutHook, len(manifest.Layouts))
	for _, layout := range manifest.Layouts {
		if provider, ok := providers[layout.Name]; ok {
			held[layout.Plan] = layoutHook{name: layout.Name, load: provider}
		}
	}
	return held
}

func (a *App) callLayout(r *http.Request, params Params, plan uint32) (runtime.Accessible, error) {
	hook, ok := a.layouts[plan]
	if !ok || hook.load == nil {
		return nil, nil
	}
	own := cache.NewRecorder()
	props, err := hook.load(r.WithContext(cache.WithRecorder(r.Context(), own)), params)
	if err != nil {
		return nil, err
	}
	cache.From(r.Context()).Merge(own)
	return props, nil
}

func (a *App) layoutOf(r *http.Request, route ir.Route, params Params, plan *ir.Plan) ([]runtime.Accessible, error) {
	index, held := chainIndex(a.chain(route), plan)
	if len(a.layouts) == 0 || !held || index >= len(route.LayoutChain) {
		return nil, nil
	}
	props, err := a.callLayout(r, params, route.LayoutChain[index])
	if err != nil || props == nil {
		return nil, err
	}
	return []runtime.Accessible{props}, nil
}

func chainIndex(chain []*ir.Plan, plan *ir.Plan) (int, bool) {
	for index, held := range chain {
		if held == plan {
			return index, true
		}
	}
	return 0, false
}
