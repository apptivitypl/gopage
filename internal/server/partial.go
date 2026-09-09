package server

import (
	"net/http"
	"strings"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/runtime"
)

const (
	PartialHeader = "GOPAGE-Partial"
	LevelHeader   = "GOPAGE-Level"
	TitleHeader   = "GOPAGE-Title"
	PartialType   = "text/vnd.gopage-partial"

	MaxPartialPath = 512
)

func (a *App) partial(r *http.Request) bool {
	return a.config.Nav.Differential() && r.Header.Get(PartialHeader) != ""
}

func (a *App) sharedLevel(from string, target ir.Route) int {
	if len(from) > MaxPartialPath {
		return 0
	}
	held, _, ok := a.router.Match(strings.TrimSpace(from))
	if !ok {
		return 0
	}
	level := 0
	for level < len(held.LayoutChain) && level < len(target.LayoutChain) &&
		held.LayoutChain[level] == target.LayoutChain[level] {
		level++
	}
	return level
}

func (a *App) partialPage(w http.ResponseWriter, r *http.Request, route ir.Route, params Params, level int) {
	if names := a.deferredFor(route); len(names) > 0 && !a.config.Fragments.Fetches() {
		a.renderFresh(w, r, route, params, level, cache.StatusBypass)
		return
	}
	a.cachedRender(w, r, route, params, level)
}

func titleOf(props runtime.Accessible) string {
	value, _ := props.Get([]string{runtime.MetaRoot, "Title"})
	return value.Str
}
