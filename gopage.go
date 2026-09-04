package gopage

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/apptivitypl/gopage/internal/action"
	"github.com/apptivitypl/gopage/internal/api"
	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/form"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/logs"
	"github.com/apptivitypl/gopage/internal/redirect"
	"github.com/apptivitypl/gopage/internal/runtime"
	"github.com/apptivitypl/gopage/internal/server"
)

type (
	Params           = server.Params
	Accessible       = runtime.Accessible
	PropsProvider    = server.PropsProvider
	DeferredProvider = server.DeferredProvider
	MetaProvider     = server.MetaProvider
	SubmitProvider   = server.SubmitProvider
	Value            = runtime.Value
)

var (
	String = runtime.String
	Int    = runtime.Int
	Bool   = runtime.Bool
)

type Props = runtime.Map

type Meta = runtime.Meta

func NewMeta(title string) Meta {
	return Meta{Title: title}
}

type Route struct {
	Pattern string
	Name    string
	Static  bool
}

type Middleware = server.Middleware

type Options struct {
	Manifest   []byte
	Config     []byte
	Static     fs.FS
	Bundles    fs.FS
	Preload    []byte
	Public     fs.FS
	CacheBytes int64
	Props      map[string]PropsProvider
	Deferred   map[string]DeferredProvider
	Meta       map[string]MetaProvider
	Submit     map[string]SubmitProvider
	API        map[string]http.Handler
	Middleware []Middleware
	Logger     *slog.Logger
}

type App struct {
	inner    *server.App
	manifest *ir.Manifest
	logger   *slog.Logger
}

func (a *App) MaxConnections() int {
	return a.inner.MaxConnections()
}

func (a *App) Log() *slog.Logger {
	return a.logger
}

func New(opts Options) (*App, error) {
	manifest, err := ir.Decode(opts.Manifest)
	if err != nil {
		return nil, fmt.Errorf("gopage: %w", err)
	}
	settings, err := config.Parse(string(opts.Config))
	if err != nil {
		return nil, fmt.Errorf("gopage: %w", err)
	}
	served, link, public, err := staticAssets(opts.Static, opts.Bundles, opts.Public)
	if err != nil {
		return nil, fmt.Errorf("gopage: %w", err)
	}
	sidecar := assets.ParseSidecar(opts.Preload)
	if len(sidecar.Links) == 0 && len(sidecar.Islands) == 0 {
		sidecar = assets.ReadSidecar(opts.Bundles)
	}
	if len(sidecar.Links) > 0 {
		link = sidecar.Link()
	}
	logger := opts.Logger
	if logger == nil {
		logger = logs.New()
	}
	return &App{
		logger: logger,
		inner: server.New(server.Options{
			Manifest:   manifest,
			Config:     settings,
			Cache:      cache.New(cache.Options{Limit: opts.CacheBytes}),
			Assets:     served,
			AssetLink:  link,
			Public:     public,
			Props:      opts.Props,
			Deferred:   opts.Deferred,
			Meta:       opts.Meta,
			Submit:     opts.Submit,
			API:        opts.API,
			Middleware: opts.Middleware,
			Logger:     logger,
			AccessLog:  logs.Access(),
			Preloads:   sidecar.IslandChunks(),
		}),
		manifest: manifest,
	}, nil
}

func staticAssets(static, bundles, public fs.FS) (http.Handler, string, []string, error) {
	var stores []assets.Store
	var files []string
	if static != nil {
		list, err := assets.Collect(static)
		if err != nil {
			return nil, "", nil, err
		}
		stores = append(stores, assets.Store{FS: static, Files: list})
	}
	if bundles != nil {
		list, err := assets.Verbatim(bundles)
		if err != nil {
			return nil, "", nil, err
		}
		stores = append(stores, assets.Store{FS: bundles, Files: list})
	}
	if public != nil {
		list, err := assets.Public(public)
		if err != nil {
			return nil, "", nil, err
		}
		for _, asset := range list {
			files = append(files, asset.Path)
		}
		stores = append(stores, assets.Store{FS: public, Files: list})
	}
	served, all, err := assets.Serve(stores)
	if err != nil || served == nil {
		return nil, "", nil, err
	}
	return served, assets.Link(linkable(all)), files, nil
}

func linkable(list []assets.Asset) []assets.Asset {
	kept := make([]assets.Asset, 0, len(list))
	for _, asset := range list {
		if asset.Cache == assets.PublicCache {
			continue
		}
		kept = append(kept, asset)
	}
	return kept
}

func (a *App) Handler() http.Handler {
	return a.inner.Handler()
}

type (
	CacheRecorder = cache.Recorder
	CacheStats    = cache.Stats
)

func (a *App) Invalidate(tags ...string) int {
	return a.inner.Invalidate(tags...)
}

func (a *App) CacheStats() CacheStats {
	return a.inner.CacheStats()
}

func LocaleOf(r *http.Request) string {
	return server.LocaleOf(r)
}

var ErrNotFound = server.ErrNotFound

type (
	Response   = api.Response
	APIHandler = api.Handler
	Stream     = api.Stream
)

var (
	JSON      = api.JSON
	Text      = api.Text
	Content   = api.Content
	NoContent = api.NoContent
	Events    = api.Events
)

type (
	Action     = action.Action
	FormResult = form.Result
)

func Redirect(route string) *action.Redirect {
	return action.Route(route)
}

func RedirectTo(location string) *action.Redirect {
	return action.To(location)
}

func SafeRedirect(target, fallback string, allowed ...string) string {
	if location, ok := redirect.Safe(target, allowed); ok {
		return location
	}
	return fallback
}

func DecodeForm(r *http.Request, target any) (FormResult, error) {
	return form.Decode(r, target)
}

func API(handlers map[string]APIHandler) http.Handler {
	return api.Mux(handlers)
}

func ParamsFrom(r *http.Request, names []string) Params {
	return server.ParamsFrom(r, names)
}

func (a *App) Routes() []Route {
	routes := make([]Route, 0, len(a.manifest.Routes))
	for _, route := range a.manifest.Routes {
		routes = append(routes, Route{
			Pattern: route.Pattern,
			Name:    route.Name,
			Static:  route.Class == ir.ClassStatic,
		})
	}
	return routes
}

func (a *App) RenderStatic(name string) ([]byte, error) {
	for _, route := range a.manifest.Routes {
		if route.Name == name {
			return a.inner.RenderStatic(route)
		}
	}
	return nil, fmt.Errorf("gopage: no route named %q", name)
}

type Sequence = runtime.Sequence

var (
	Nil    = runtime.Nil
	Float  = runtime.Float
	Seq    = runtime.Seq
	Object = runtime.Object
)

type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type Real interface {
	~float32 | ~float64
}

type Objects[T Accessible] []T

func (s Objects[T]) Len() int { return len(s) }

func (s Objects[T]) At(index int) Value {
	if index < 0 || index >= len(s) {
		return Nil()
	}
	return Object(s[index])
}

type Strings []string

func (s Strings) Len() int { return len(s) }

func (s Strings) At(index int) Value {
	if index < 0 || index >= len(s) {
		return Nil()
	}
	return String(s[index])
}

type Ints[T Integer] []T

func (s Ints[T]) Len() int { return len(s) }

func (s Ints[T]) At(index int) Value {
	if index < 0 || index >= len(s) {
		return Nil()
	}
	return Int(int64(s[index]))
}

type Floats[T Real] []T

func (s Floats[T]) Len() int { return len(s) }

func (s Floats[T]) At(index int) Value {
	if index < 0 || index >= len(s) {
		return Nil()
	}
	return Float(float64(s[index]))
}

type Bools []bool

func (s Bools) Len() int { return len(s) }

func (s Bools) At(index int) Value {
	if index < 0 || index >= len(s) {
		return Nil()
	}
	return Bool(s[index])
}

type Ctx struct {
	request *http.Request
	params  Params
}

func NewCtx(request *http.Request, params Params) *Ctx {
	return &Ctx{request: request, params: params}
}

func (c *Ctx) Context() context.Context {
	if c.request == nil {
		return context.Background()
	}
	return c.request.Context()
}

func (c *Ctx) Request() *http.Request {
	return c.request
}

func (c *Ctx) Params() Params {
	return c.params
}

func (c *Ctx) Param(name string) string {
	return c.params[name]
}

func (c *Ctx) Cache() *cache.Recorder {
	return cache.From(c.Context())
}

func (c *Ctx) Locale() string {
	if c.request == nil {
		return ""
	}
	return server.LocaleOf(c.request)
}

func (c *Ctx) T(key string) string {
	return server.TranslatorFrom(c.Context())(key, 0, false)
}

func (c *Ctx) Count(key string, count int) string {
	return server.TranslatorFrom(c.Context())(key, count, true)
}

func (c *Ctx) Query(name string) string {
	if c.request == nil {
		return ""
	}
	return c.request.URL.Query().Get(name)
}

type Case interface {
	gopageCase() string
}

type Cases[T Case] []T

func (s Cases[T]) Len() int { return len(s) }

func (s Cases[T]) At(index int) Value {
	if index < 0 || index >= len(s) {
		return Nil()
	}
	return String(s[index].gopageCase())
}
