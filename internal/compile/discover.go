package compile

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/paths"
)

const (
	AppDir       = paths.AppDir
	PageFile     = "page.gopage"
	LayoutFile   = "layout.gopage"
	RouteFile    = "route.go"
	NotFoundFile = "not-found.gopage"
	ErrorFile    = "error.gopage"
	APIPrefix    = "/api"
)

type RouteKind uint8

const (
	RoutePage RouteKind = iota
	RouteAPI
)

type Route struct {
	Pattern string
	Name    string
	Kind    RouteKind
	File    string
	Layouts []string
}

func (r Route) Localized() bool {
	return r.Kind == RoutePage && !strings.HasPrefix(r.Pattern, APIPrefix)
}

func Discover(fsys fs.FS, bag *diag.Bag) []Route {
	layouts := collectLayouts(fsys)
	var routes []Route

	_ = fs.WalkDir(fsys, AppDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		dir := path.Dir(filePath)
		switch entry.Name() {
		case PageFile:
			routes = append(routes, page(dir, filePath, layouts, bag))
		case RouteFile:
			routes = append(routes, api(dir, filePath))
		}
		return nil
	})

	sort.Slice(routes, func(i, j int) bool { return routes[i].Pattern < routes[j].Pattern })
	return reportConflicts(routes, bag)
}

func page(dir, filePath string, layouts []string, bag *diag.Bag) Route {
	pattern := patternOf(dir)
	if strings.HasPrefix(pattern, APIPrefix) {
		bag.Add(diag.New(diag.C102, filePath, diag.Span{},
			fmt.Sprintf("%s renders a page under %s", filePath, APIPrefix)).
			WithHelp("the api namespace serves handlers only; move this page outside app/api"))
	}
	return Route{
		Pattern: pattern,
		Name:    nameOf(pattern),
		Kind:    RoutePage,
		File:    filePath,
		Layouts: layoutsFor(dir, layouts),
	}
}

func api(dir, filePath string) Route {
	pattern := patternOf(dir)
	return Route{Pattern: pattern, Name: nameOf(pattern), Kind: RouteAPI, File: filePath}
}

func collectLayouts(fsys fs.FS) []string {
	var layouts []string
	_ = fs.WalkDir(fsys, AppDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() == LayoutFile {
			layouts = append(layouts, filePath)
		}
		return nil
	})
	sort.Strings(layouts)
	return layouts
}

func layoutsFor(dir string, layouts []string) []string {
	var chain []string
	for _, layout := range layouts {
		layoutDir := path.Dir(layout)
		if dir == layoutDir || strings.HasPrefix(dir, layoutDir+"/") {
			chain = append(chain, layout)
		}
	}
	sort.Slice(chain, func(i, j int) bool { return len(chain[i]) < len(chain[j]) })
	return chain
}

func patternOf(dir string) string {
	rest := strings.TrimPrefix(strings.TrimPrefix(dir, AppDir), "/")
	if rest == "" {
		return "/"
	}
	var segments []string
	for segment := range strings.SplitSeq(rest, "/") {
		if isGroup(segment) {
			continue
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return "/"
	}
	return "/" + strings.Join(segments, "/")
}

func isGroup(segment string) bool {
	return strings.HasPrefix(segment, "(") && strings.HasSuffix(segment, ")")
}

func nameOf(pattern string) string {
	if pattern == "/" {
		return "index"
	}
	var parts []string
	for segment := range strings.SplitSeq(strings.TrimPrefix(pattern, "/"), "/") {
		parts = append(parts, strings.Trim(segment, "[].,"))
	}
	return strings.Join(parts, ".")
}

func reportConflicts(routes []Route, bag *diag.Bag) []Route {
	seen := map[string]Route{}
	unique := routes[:0]
	for _, route := range routes {
		previous, clash := seen[route.Pattern]
		if clash {
			bag.Add(diag.New(diag.C101, route.File, diag.Span{},
				fmt.Sprintf("%s and %s both answer %s", previous.File, route.File, route.Pattern)).
				WithHelp("a segment holds either a page or a route handler, never both"))
			continue
		}
		seen[route.Pattern] = route
		unique = append(unique, route)
	}
	return unique
}

func ParamsOf(pattern string) []string {
	var params []string
	for segment := range strings.SplitSeq(pattern, "/") {
		if !strings.HasPrefix(segment, "[") || !strings.HasSuffix(segment, "]") {
			continue
		}
		params = append(params, strings.Trim(segment, "[].,"))
	}
	return params
}

type FallbackFile struct {
	Prefix  string
	Name    string
	File    string
	Kind    string
	Layouts []string
}

func DiscoverFallbacks(fsys fs.FS) []FallbackFile {
	layouts := collectLayouts(fsys)
	var found []FallbackFile
	_ = fs.WalkDir(fsys, AppDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		var kind string
		switch entry.Name() {
		case NotFoundFile:
			kind = "not-found"
		case ErrorFile:
			kind = "error"
		default:
			return nil
		}
		dir := path.Dir(filePath)
		found = append(found, FallbackFile{
			Prefix:  patternOf(dir),
			Name:    fallbackName(patternOf(dir), kind),
			File:    filePath,
			Kind:    kind,
			Layouts: layoutsFor(dir, layouts),
		})
		return nil
	})
	sort.Slice(found, func(i, j int) bool {
		if found[i].Prefix != found[j].Prefix {
			return found[i].Prefix < found[j].Prefix
		}
		return found[i].Kind < found[j].Kind
	})
	return found
}

func DiscoverComponents(fsys fs.FS) map[string]string {
	found := map[string]string{}
	_ = fs.WalkDir(fsys, ComponentsDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if entry.Name() == TemplateFile {
			found[path.Base(path.Dir(filePath))] = filePath
			return nil
		}
		if path.Dir(filePath) == ComponentsDir && strings.HasSuffix(entry.Name(), TemplateSuffix) {
			found[strings.TrimSuffix(entry.Name(), TemplateSuffix)] = filePath
		}
		return nil
	})
	return found
}

func fallbackName(prefix, kind string) string {
	if prefix == "/" {
		return kind
	}
	return nameOf(prefix) + "." + kind
}
