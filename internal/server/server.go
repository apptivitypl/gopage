package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/sonquer/rill/internal/assets"
	"github.com/sonquer/rill/internal/cache"
	"github.com/sonquer/rill/internal/config"
	"github.com/sonquer/rill/internal/i18n"
	"github.com/sonquer/rill/internal/ir"
	"github.com/sonquer/rill/internal/runtime"
	"github.com/sonquer/rill/internal/seo"
)

type PropsProvider func(*http.Request, Params) (runtime.Accessible, error)

type MetaProvider func(*http.Request, Params) (runtime.Meta, error)

var ErrNotFound = errors.New("rill: not found")

type Middleware func(http.Handler) http.Handler

type Options struct {
	Manifest   *ir.Manifest
	Config     config.Config
	Assets     http.Handler
	AssetLink  string
	Public     []string
	Cache      *cache.Cache
	Props      map[string]PropsProvider
	Deferred   map[string]DeferredProvider
	Meta       map[string]MetaProvider
	Submit     map[string]SubmitProvider
	API        map[string]http.Handler
	Middleware []Middleware
	Entropy    io.Reader
	Logger     *slog.Logger
	AccessLog  bool
	Preloads   map[string][]string
}

type routePreload struct {
	tags string
	link string
}

type App struct {
	manifest   *ir.Manifest
	config     config.Config
	assets     http.Handler
	assetLink  string
	preloads   map[string]routePreload
	public     []string
	cache      *cache.Cache
	router     *Router
	props      map[string]PropsProvider
	deferred   map[string]DeferredProvider
	meta       map[string]MetaProvider
	submit     map[string]SubmitProvider
	api        map[string]http.Handler
	entropy    io.Reader
	middleware []Middleware
	logger     *slog.Logger
	accessLog  bool
	messages   map[string]uint32
	chains     map[string][]*ir.Plan
	deferrals  map[string][]string
}

func New(opts Options) *App {
	manifest := opts.Manifest
	if manifest == nil {
		manifest = &ir.Manifest{Version: ir.Version}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	settings := opts.Config
	if settings.I18n.DefaultLocale == "" {
		settings = config.Default()
	}
	app := &App{
		manifest:   manifest,
		config:     settings,
		assets:     opts.Assets,
		assetLink:  opts.AssetLink,
		preloads:   preloadsFor(opts.Manifest, opts.Preloads),
		public:     opts.Public,
		deferred:   opts.Deferred,
		cache:      opts.Cache,
		router:     NewRouter(manifest.Routes),
		props:      opts.Props,
		meta:       opts.Meta,
		submit:     opts.Submit,
		api:        opts.API,
		entropy:    opts.Entropy,
		middleware: opts.Middleware,
		logger:     logger,
		accessLog:  opts.AccessLog,
		messages:   messageIndex(manifest),
	}
	app.chains, app.deferrals = routePlans(manifest)
	return app
}

func messageIndex(manifest *ir.Manifest) map[string]uint32 {
	index := make(map[string]uint32, len(manifest.Messages))
	for position, key := range manifest.Messages {
		index[key] = uint32(position)
	}
	return index
}

func routePlans(manifest *ir.Manifest) (map[string][]*ir.Plan, map[string][]string) {
	chains := make(map[string][]*ir.Plan, len(manifest.Routes))
	deferrals := make(map[string][]string, len(manifest.Routes))
	for _, route := range manifest.Routes {
		chain := manifest.Chain(route)
		chains[route.Name] = chain
		var names []string
		for _, plan := range chain {
			for _, fragment := range plan.Fragments {
				if fragment.Deferred {
					names = append(names, fragment.Name)
				}
			}
		}
		deferrals[route.Name] = names
	}
	return chains, deferrals
}

func (a *App) chain(route ir.Route) []*ir.Plan {
	if chain, ok := a.chains[route.Name]; ok {
		return chain
	}
	return a.manifest.Chain(route)
}

func (a *App) MaxConnections() int {
	return a.config.Security.MaxConnections
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	for pattern, handler := range a.api {
		mux.Handle(pattern, handler)
	}
	if a.assets != nil {
		mux.Handle(assets.Prefix, a.assets)
		for _, file := range a.public {
			mux.Handle(file, a.assets)
		}
	}
	mux.HandleFunc(seo.SitemapPath, a.sitemap)
	mux.HandleFunc(seo.RobotsPath, a.robots)
	mux.HandleFunc("/", a.renderPage)

	var handler http.Handler = mux
	for i := len(a.middleware) - 1; i >= 0; i-- {
		handler = a.middleware[i](handler)
	}
	if a.config.I18n.Mode == config.ModeSubdomain {
		handler = a.hostLocale(handler)
	} else {
		handler = a.locale(handler)
	}
	return a.observe(a.compressed(a.guard(a.secure(a.crossOrigin(a.limited(a.reroute(handler)))))))
}

func (a *App) renderPage(w http.ResponseWriter, r *http.Request) {
	route, params, ok := a.router.Match(r.URL.Path)
	if !ok {
		a.fail(w, r, ir.FallbackNotFound, http.StatusNotFound)
		return
	}
	if r.Method == http.MethodPost {
		a.submitPage(w, r, route, params)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		allow := allowFor(a.submit, route.Name)
		a.logger.Warn("method not allowed", "route", route.Name, "method", r.Method, "allow", allow)
		w.Header().Set("Allow", allow)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if name := r.Header.Get(FragmentHeader); name != "" {
		a.writeFragment(w, r, route, params, name)
		return
	}
	if a.partial(r) {
		a.writePartial(w, r, route, params)
		return
	}
	a.hint(w, route)
	if names := a.deferredFor(route); len(names) > 0 && !a.config.Fragments.Fetches() {
		a.streamPage(w, r, route, params, names)
		return
	}
	a.cachedPage(w, r, route, params)
}

func (a *App) hint(w http.ResponseWriter, route ir.Route) {
	link := a.assetLink
	if extra := a.preloads[route.Name].link; extra != "" {
		if link != "" {
			link += ", "
		}
		link += extra
	}
	if link == "" {
		return
	}
	w.Header().Set(AssetsHeader, link)
	w.Header().Set("Link", link)
	w.WriteHeader(http.StatusEarlyHints)
	w.Header().Del("Link")
}

func preloadsFor(manifest *ir.Manifest, chunks map[string][]string) map[string]routePreload {
	preloads := map[string]routePreload{}
	if manifest == nil || len(chunks) == 0 {
		return preloads
	}
	for _, route := range manifest.Routes {
		names := eagerChunks(manifest.Chain(route), chunks)
		if len(names) == 0 {
			continue
		}
		var tags, link strings.Builder
		for index, name := range names {
			tags.WriteString(`<link rel="modulepreload" href="` + assets.Prefix + name + `" fetchpriority="low">`)
			if index > 0 {
				link.WriteString(", ")
			}
			link.WriteString("<" + assets.Prefix + name + ">; rel=modulepreload")
		}
		preloads[route.Name] = routePreload{tags: tags.String(), link: link.String()}
	}
	return preloads
}

func eagerChunks(chain []*ir.Plan, chunks map[string][]string) []string {
	seen := map[string]bool{}
	var names []string
	for _, plan := range chain {
		for _, island := range plan.Islands {
			if island.Strategy != "load" && island.Strategy != "idle" {
				continue
			}
			for _, chunk := range chunks[island.Name] {
				if !seen[chunk] {
					seen[chunk] = true
					names = append(names, chunk)
				}
			}
		}
	}
	sort.Strings(names)
	return names
}

func (a *App) write(w http.ResponseWriter, r *http.Request, body *runtime.Buffer, status int) {
	vary(w)
	keepPrivate(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body.Bytes()); err != nil {
		a.logger.Error("write failed", "path", r.URL.Path, "error", err)
	}
}

func (a *App) fail(w http.ResponseWriter, r *http.Request, kind ir.FallbackKind, status int) {
	body, ok := a.renderFallback(kind, r)
	if !ok {
		keepPrivate(w)
		http.Error(w, http.StatusText(status), status)
		return
	}
	defer runtime.Release(body)
	a.write(w, r, body, status)
}

func (a *App) renderFallback(kind ir.FallbackKind, r *http.Request) (*runtime.Buffer, bool) {
	fallback, ok := a.manifest.Fallback(kind, r.URL.Path)
	if !ok {
		return nil, false
	}
	props, err := a.providers(ir.Route{Name: fallback.Name, Pattern: fallback.Prefix}, r, Params{})
	if err != nil {
		a.logger.Error("fallback props failed", "fallback", fallback.Name, "error", err)
		props = runtime.WithLocale(runtime.WithMeta(runtime.Empty{}, runtime.Meta{}), a.localeOf(r))
	}
	chain := a.manifest.Chain(ir.Route{Plan: fallback.Plan, LayoutChain: fallback.LayoutChain})
	out := runtime.Acquire(runtime.Capacity(chain))
	if err := runtime.RenderOptions(chain, props, out, a.options(nil, LocaleOf(r))); err != nil {
		runtime.Release(out)
		a.logger.Error("fallback render failed", "fallback", fallback.Name, "error", err)
		return nil, false
	}
	return out, true
}

func allowFor(submit map[string]SubmitProvider, name string) string {
	if _, ok := submit[name]; ok {
		return "GET, HEAD, POST"
	}
	return "GET, HEAD"
}

func (a *App) Render(route ir.Route, r *http.Request, params Params) (*runtime.Buffer, error) {
	props, err := a.propsFor(route, r, params)
	if err != nil {
		return nil, err
	}
	return a.renderWith(route, props)
}

func (a *App) renderWith(route ir.Route, props runtime.Accessible) (*runtime.Buffer, error) {
	return a.renderHooked(route, props, nil)
}

func (a *App) renderHooked(route ir.Route, props runtime.Accessible, hook runtime.Fragments) (*runtime.Buffer, error) {
	return a.renderLocalised(route, props, hook, a.config.I18n.DefaultLocale)
}

func (a *App) options(hook runtime.Fragments, locale string) runtime.Options {
	opts := runtime.Options{
		Fragments: hook,
		Plural:    i18n.RuleFor(locale),
		Markers:   a.config.Nav.Differential(),
		Fetched:   a.config.Fragments.Fetches(),
	}
	if catalog, ok := a.manifest.Catalog(locale); ok {
		opts.Catalog = catalog
	}
	return opts
}

func (a *App) renderLocalised(route ir.Route, props runtime.Accessible, hook runtime.Fragments,
	locale string) (*runtime.Buffer, error) {
	return a.renderResolved(route, props, hook, locale, nil)
}

func (a *App) renderResolved(route ir.Route, props runtime.Accessible, hook runtime.Fragments,
	locale string, deferred runtime.Deferred) (*runtime.Buffer, error) {
	chain := a.chain(route)
	out := runtime.Acquire(runtime.Capacity(chain))
	opts := a.options(hook, locale)
	opts.Deferred = deferred
	opts.Preload = a.preloads[route.Name].tags
	if err := runtime.RenderOptions(chain, props, out, opts); err != nil {
		runtime.Release(out)
		return nil, fmt.Errorf("route %s: %w", route.Name, err)
	}
	return out, nil
}

func (a *App) propsFor(route ir.Route, r *http.Request, params Params) (runtime.Accessible, error) {
	return a.providers(route, r, params)
}

func (a *App) providers(route ir.Route, r *http.Request, params Params) (runtime.Accessible, error) {
	name := route.Name
	r = r.WithContext(WithTranslator(r.Context(), a.translator(r)))
	var props runtime.Accessible = runtime.Empty{}
	if provider, ok := a.props[name]; ok {
		resolved, err := provider(r, params)
		if err != nil {
			return nil, err
		}
		props = resolved
	}
	meta, err := a.metaFor(name, r, params)
	if err != nil {
		return nil, err
	}
	return runtime.WithLocale(runtime.WithMeta(props, a.seo(meta, r, route)), a.localeOf(r)), nil
}

func (a *App) localeOf(r *http.Request) runtime.Locale {
	tag := LocaleOf(r)
	if tag == "" {
		tag = a.config.I18n.DefaultLocale
	}
	locale := runtime.Locale{Tag: tag, Default: tag == a.config.I18n.DefaultLocale}
	if !locale.Default || a.config.I18n.PrefixDefault {
		locale.Prefix = "/" + tag
	}
	return locale
}

func (a *App) metaFor(name string, r *http.Request, params Params) (runtime.Meta, error) {
	provider, ok := a.meta[name]
	if !ok {
		return runtime.Meta{}, nil
	}
	return provider(r, params)
}

func (a *App) Routes() []ir.Route {
	return a.router.Routes()
}

func (a *App) RenderStatic(route ir.Route) ([]byte, error) {
	request := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: patternPath(route.Pattern)},
		Header: http.Header{},
	}
	body, err := a.Render(route, request, Params{})
	if err != nil {
		return nil, err
	}
	defer runtime.Release(body)
	out := make([]byte, body.Len())
	copy(out, body.Bytes())
	return out, nil
}

func patternPath(pattern string) string {
	if pattern == "" {
		return "/"
	}
	return pattern
}
