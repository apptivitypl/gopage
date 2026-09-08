package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/i18n"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/reply"
	"github.com/apptivitypl/gopage/internal/runtime"
	"github.com/apptivitypl/gopage/internal/seo"
	"github.com/apptivitypl/gopage/internal/vocab"
)

type PropsProvider func(*http.Request, Params) (runtime.Accessible, error)

type MetaProvider func(*http.Request, Params, runtime.Accessible) (runtime.Meta, error)

type SitemapProvider func(*http.Request) (seo.Seq, error)

var ErrNotFound = errors.New("gopage: not found")

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
	Sitemap    map[string]SitemapProvider
	Submit     map[string]SubmitProvider
	API        map[string]http.Handler
	Layouts    map[string]PropsProvider
	Middleware []Middleware
	Entropy    io.Reader
	Logger     *slog.Logger
	AccessLog  bool
	Preloads   map[string][]string
	Locals     any
	Images     ImageSupport
	Client     *http.Client
	Invalidate string
	OnRequest  Reporter
}

type localsKey struct{}

func WithLocals(ctx context.Context, locals any) context.Context {
	return context.WithValue(ctx, localsKey{}, locals)
}

func LocalsFrom(ctx context.Context) any {
	return ctx.Value(localsKey{})
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
	sitemaps   map[string]SitemapProvider
	submit     map[string]SubmitProvider
	api        map[string]http.Handler
	layouts    map[uint32]layoutHook
	entropy    io.Reader
	middleware []Middleware
	logger     *slog.Logger
	accessLog  bool
	messages   map[string]uint32
	chains     map[string][]*ir.Plan
	deferrals  map[string][]string
	locals     any
	vocab      vocab.Table
	zone       *time.Location
	images     ImageSupport
	client     *http.Client
	token      string
	onRequest  Reporter
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
		vocab:      vocab.New(settings),
		zone:       settings.I18n.Zone(),
		props:      opts.Props,
		meta:       opts.Meta,
		sitemaps:   opts.Sitemap,
		submit:     opts.Submit,
		api:        opts.API,
		layouts:    layoutPlans(opts.Manifest, opts.Layouts),
		locals:     opts.Locals,
		images:     opts.Images,
		client:     imageClient(opts.Client, opts.Config.Images.Serves),
		token:      opts.Invalidate,
		onRequest:  opts.OnRequest,
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
		mux.Handle(pattern, a.translating(handler))
	}
	if a.assets != nil {
		mux.Handle(assets.Prefix, a.assets)
		for _, file := range a.public {
			mux.Handle(file, a.assets)
		}
	}
	a.serveSEO(mux)
	mux.HandleFunc("/", a.renderPage)

	handler := a.local(mux)
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

func imageClient(client *http.Client, allowed func(host string) bool) *http.Client {
	held := &http.Client{Timeout: 10 * time.Second}
	if client != nil {
		copied := *client
		held = &copied
	}
	held.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= MaxImageHops {
			return errTooManyHops
		}
		if request.URL.Scheme != "https" || !allowed(request.URL.Hostname()) {
			return errNoSource
		}
		return nil
	}
	return held
}

func (a *App) serveSEO(mux *http.ServeMux) {
	if a.config.SEO.Sitemap.Enabled() {
		a.serveBuiltin(mux, seo.SitemapPath, a.sitemap)
		a.serveBuiltin(mux, seo.SitemapPrefix+"{shard}", a.sitemapShard)
	}
	if a.config.SEO.Robots.Enabled() {
		a.serveBuiltin(mux, seo.RobotsPath, a.robots)
	}
	if a.config.Images.Enabled() && a.images != nil {
		a.serveBuiltin(mux, ImagePath, a.image)
	}
	if a.token != "" {
		a.serveBuiltin(mux, InvalidatePath, a.invalidate)
	}
}

func (a *App) serveBuiltin(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	if a.claims(pattern) {
		a.logger.Warn("built-in endpoint yielded to a route of the project", "path", pattern)
		return
	}
	mux.HandleFunc(pattern, handler)
}

func (a *App) claims(pattern string) bool {
	if _, taken := a.api[pattern]; taken {
		return true
	}
	return a.assets != nil && slices.Contains(a.public, pattern)
}

func (a *App) local(next http.Handler) http.Handler {
	if a.locals == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithLocals(r.Context(), a.locals)))
	})
}

func (a *App) renderPage(w http.ResponseWriter, r *http.Request) {
	route, params, ok := a.router.Match(r.URL.Path)
	if !ok {
		a.fail(w, r, ir.FallbackNotFound, http.StatusNotFound)
		return
	}
	if held := bucketsOf(r, route); held != nil {
		r = r.WithContext(WithBuckets(r.Context(), held))
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
	layouts, err := a.layoutChain(r, route, params)
	if err != nil {
		return nil, err
	}
	return a.renderResolved(route, props, layouts, nil, a.config.I18n.DefaultLocale, nil)
}

func (a *App) options(hook runtime.Fragments, locale string) runtime.Options {
	opts := runtime.Options{
		Fragments: hook,
		Zone:      a.zone,
		Plural:    i18n.RuleFor(locale),
		Markers:   a.config.Nav.Differential(),
		Fetched:   a.config.Fragments.Fetches(),
	}
	if catalog, ok := a.manifest.Catalog(locale); ok {
		opts.Catalog = catalog
	}
	return opts
}

func (a *App) renderResolved(route ir.Route, props runtime.Accessible, layouts []runtime.Accessible,
	hook runtime.Fragments, locale string, deferred runtime.Deferred) (*runtime.Buffer, error) {
	chain := a.chain(route)
	out := runtime.Acquire(runtime.Capacity(chain))
	opts := a.options(hook, locale)
	opts.Layouts = layouts
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
	meta, err := a.metaFor(name, r, params, props)
	if err != nil {
		return nil, err
	}
	return runtime.WithLocale(runtime.WithMeta(props, a.seo(meta, r)), a.localeOf(r)), nil
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

func (a *App) metaFor(name string, r *http.Request, params Params, props runtime.Accessible) (runtime.Meta, error) {
	provider, ok := a.meta[name]
	if !ok {
		return runtime.Meta{}, nil
	}
	return provider(r, params, props)
}

func (a *App) Routes() []ir.Route {
	return a.router.Routes()
}

func (a *App) synthetic(ctx context.Context, route ir.Route, params Params) *http.Request {
	request := (&http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: patternPath(route.Pattern)},
		Header: http.Header{},
	}).WithContext(ctx)
	request = withLocale(request, a.config.I18n.DefaultLocale)
	if a.vocab.Localises() {
		request = request.WithContext(vocab.With(request.Context(), a.vocab))
	}
	if filled, err := Fill(route.Pattern, params); err == nil {
		request.URL = &url.URL{Path: filled}
	}
	return request
}

func (a *App) RenderFragment(ctx context.Context, route ir.Route, params Params, name string) ([]byte, error) {
	fragment, plan, ok := a.deferredIn(route, name)
	if !ok {
		return nil, fmt.Errorf("route %s has no deferred fragment named %q", route.Pattern, name)
	}
	provider, ok := a.deferred[name]
	if !ok {
		return nil, fmt.Errorf("fragment %q has no loader", name)
	}
	request := a.synthetic(ctx, route, params)
	request = request.WithContext(WithTranslator(request.Context(), a.translator(request)))
	props, err := a.providers(route, request, params)
	if err != nil {
		return nil, err
	}
	held, err := provider(request, params)
	if err != nil {
		return nil, err
	}
	page := a.options(a.fragmentHook(request), LocaleOf(request))
	out := runtime.Acquire(plan.Capacity)
	defer runtime.Release(out)
	body := runtime.Options{Fragments: page.Fragments, Catalog: page.Catalog, Plural: page.Plural}
	if err := runtime.RenderFragment(plan, fragment, runtime.WithRoot(props, fragment.Name, held), out, body); err != nil {
		return nil, err
	}
	return bytes.Clone(out.Bytes()), nil
}

func (a *App) RenderRoute(ctx context.Context, route ir.Route, params Params) ([]byte, error) {
	request := a.synthetic(ctx, route, params)
	body, err := a.Render(route, request, params)
	if err != nil {
		return nil, err
	}
	defer runtime.Release(body)
	out := make([]byte, body.Len())
	copy(out, body.Bytes())
	return out, nil
}

func (a *App) RenderStatic(route ir.Route) ([]byte, error) {
	answer := reply.NewRecorder()
	request := (&http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: patternPath(route.Pattern)},
		Header: http.Header{},
	}).WithContext(reply.WithRecorder(context.Background(), answer))
	body, err := a.Render(route, request, Params{})
	if err != nil {
		return nil, err
	}
	if answer.Touched() {
		runtime.Release(body)
		return nil, fmt.Errorf("route %s writes to the response, so it cannot be exported as a static page", route.Pattern)
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
