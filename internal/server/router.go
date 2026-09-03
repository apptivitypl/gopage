package server

import (
	"net/http"
	"strings"

	"github.com/sonquer/rill/internal/ir"
)

type Params map[string]string

type Router struct {
	routes   []ir.Route
	patterns [][]string
}

func NewRouter(routes []ir.Route) *Router {
	ordered := make([]ir.Route, len(routes))
	copy(ordered, routes)
	sortBySpecificity(ordered)
	patterns := make([][]string, len(ordered))
	for i, route := range ordered {
		patterns[i] = splitPath(route.Pattern)
	}
	return &Router{routes: ordered, patterns: patterns}
}

func sortBySpecificity(routes []ir.Route) {
	for i := 1; i < len(routes); i++ {
		for j := i; j > 0 && lessSpecific(routes[j-1], routes[j]); j-- {
			routes[j-1], routes[j] = routes[j], routes[j-1]
		}
	}
}

func lessSpecific(a, b ir.Route) bool {
	return score(a.Pattern) > score(b.Pattern)
}

func score(pattern string) int {
	var value int
	for segment := range strings.SplitSeq(pattern, "/") {
		switch {
		case isCatchAll(segment):
			value += 100
		case isParam(segment):
			value += 10
		}
	}
	return value
}

func isParam(segment string) bool {
	return strings.HasPrefix(segment, "[") && strings.HasSuffix(segment, "]")
}

func isCatchAll(segment string) bool {
	return isParam(segment) && strings.Contains(segment, "...")
}

func isOptional(segment string) bool {
	return strings.HasPrefix(segment, "[[") && strings.HasSuffix(segment, "]]")
}

func paramName(segment string) string {
	return strings.Trim(segment, "[].")
}

func (r *Router) Match(path string) (ir.Route, Params, bool) {
	requested := splitPath(path)
	for i, route := range r.routes {
		if params, ok := matchPattern(r.patterns[i], requested); ok {
			return route, params, true
		}
	}
	return ir.Route{}, nil, false
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func matchPattern(pattern, requested []string) (Params, bool) {
	var params Params
	for i, segment := range pattern {
		switch {
		case isCatchAll(segment):
			rest := requested[min(i, len(requested)):]
			if len(rest) == 0 && !isOptional(segment) {
				return nil, false
			}
			if params == nil {
				params = Params{}
			}
			params[paramName(segment)] = strings.Join(rest, "/")
			return params, true
		case i >= len(requested):
			return nil, false
		case isParam(segment):
			if params == nil {
				params = Params{}
			}
			params[paramName(segment)] = requested[i]
		case segment != requested[i]:
			return nil, false
		}
	}
	if len(requested) != len(pattern) {
		return nil, false
	}
	if params == nil {
		params = Params{}
	}
	return params, true
}

func (r *Router) Routes() []ir.Route {
	return r.routes
}

func ParamsFrom(r *http.Request, names []string) Params {
	if len(names) == 0 {
		return Params{}
	}
	params := make(Params, len(names))
	for _, name := range names {
		params[name] = r.PathValue(name)
	}
	return params
}
