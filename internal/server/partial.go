package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

const (
	PartialHeader = "RILL-Partial"
	LevelHeader   = "RILL-Level"
	TitleHeader   = "RILL-Title"
	PartialType   = "text/vnd.rill-partial"

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

func (a *App) writePartial(w http.ResponseWriter, r *http.Request, route ir.Route, params Params) {
	level := a.sharedLevel(r.Header.Get(PartialHeader), route)

	props, err := a.pageProps(w, r, route, params)
	if err != nil {
		a.logger.Error("render failed", "route", route.Name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	chain := a.chain(route)
	body := runtime.Acquire(runtime.Capacity(chain))
	defer runtime.Release(body)
	opts := a.options(a.fragmentHook(r), LocaleOf(r))
	opts.Deferred = a.resolved(r, params, route)
	if err := runtime.RenderOptions(chain[level:], props, body, opts); err != nil {
		a.logger.Error("render failed", "route", route.Name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	vary(w)
	keepPrivate(w)
	w.Header().Set("Content-Type", PartialType)
	w.Header().Set(LevelHeader, strconv.Itoa(level))
	w.Header().Set(TitleHeader, url.QueryEscape(titleOf(props)))
	w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body.Bytes()); err != nil {
		a.logger.Error("write failed", "route", route.Name, "error", err)
	}
}

func titleOf(props runtime.Accessible) string {
	value, _ := props.Get([]string{runtime.MetaRoot, "Title"})
	return value.Str
}
