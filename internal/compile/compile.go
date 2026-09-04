package compile

import (
	"io/fs"
	"slices"

	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/i18n"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/schema"
	"github.com/apptivitypl/gopage/internal/seo"
)

type Phase struct {
	Name string
	Run  func(*state) error
}

type state struct {
	fsys       fs.FS
	bag        *diag.Bag
	routes     []Route
	fallbacks  []FallbackFile
	handlers   []Handler
	assets     []assets.Asset
	assetTags  string
	messages   *messageTable
	inventory  *inventory
	catalogs   map[string]i18n.Catalog
	islands    map[string]Island
	extra      []assets.Asset
	transform  assets.Transform
	inline     assets.Inliner
	config     config.Config
	plans      []ir.Plan
	planOf     map[string]uint32
	components map[string]Component
	schemas    map[string]*schema.Schema
	templates  map[string]Template
	manifest   *ir.Manifest
}

var phases = []Phase{
	{Name: "discover routes", Run: discoverPhase},
	{Name: "collect assets", Run: assetsPhase},
	{Name: "load catalogs", Run: catalogsPhase},
	{Name: "load handlers", Run: handlersPhase},
	{Name: "load components", Run: componentsPhase},
	{Name: "compile templates", Run: templatesPhase},
	{Name: "build manifest", Run: manifestPhase},
}

func Phases() []string {
	names := make([]string, len(phases))
	for i, phase := range phases {
		names[i] = phase.Name
	}
	return names
}

type Result struct {
	Components map[string]Component
	Fallbacks  []FallbackFile
	Handlers   []Handler
	Assets     []assets.Asset
	Catalogs   map[string]i18n.Catalog
	Islands    map[string]Island
	Messages   []string
	Classes    []string
	Manifest   *ir.Manifest
	Routes     []Route
	Schemas    map[string]*schema.Schema
	Templates  map[string]Template
}

type Options struct {
	Extra     []assets.Asset
	Inline    assets.Inliner
	Transform assets.Transform
}

func Compile(fsys fs.FS, bag *diag.Bag) (Result, error) {
	return CompileWith(fsys, bag, Options{})
}

func CompileWith(fsys fs.FS, bag *diag.Bag, opts Options) (Result, error) {
	s := &state{
		extra:      opts.Extra,
		inline:     opts.Inline,
		transform:  opts.Transform,
		fsys:       fsys,
		bag:        bag,
		messages:   newMessageTable(),
		inventory:  newInventory(),
		planOf:     map[string]uint32{},
		components: map[string]Component{},
		schemas:    map[string]*schema.Schema{},
		templates:  map[string]Template{},
	}
	for _, phase := range phases {
		if err := phase.Run(s); err != nil {
			return Result{}, err
		}
	}
	return Result{
		Components: s.components,
		Fallbacks:  s.fallbacks,
		Handlers:   s.handlers,
		Assets:     s.assets,
		Catalogs:   s.catalogs,
		Islands:    s.islands,
		Messages:   s.messages.Keys(),
		Classes:    s.inventory.Names(),
		Manifest:   s.manifest,
		Routes:     s.routes,
		Schemas:    s.schemas,
		Templates:  s.templates,
	}, nil
}

func catalogsPhase(s *state) error {
	settings, err := config.Load(s.fsys)
	if err != nil {
		return err
	}
	catalogs, err := i18n.Load(s.fsys)
	if err != nil {
		return err
	}
	for _, name := range i18n.Legacies(s.fsys) {
		s.bag.Add(diag.New(diag.C603, name, diag.At(0),
			"catalogs are json files now").
			WithHelp("rewrite " + name + " as json, keeping the same keys and nesting"))
	}
	s.config = settings
	s.catalogs = catalogs
	return nil
}

func assetsPhase(s *state) error {
	list, err := assets.CollectOptions(s.fsys, assets.Options{Transform: s.transform, Inline: s.inline})
	if err != nil {
		return err
	}
	s.assets = append(slices.Clone(list), s.extra...)
	s.assetTags = assets.Tags(s.assets)
	return nil
}

func discoverPhase(s *state) error {
	s.routes = Discover(s.fsys, s.bag)
	s.fallbacks = DiscoverFallbacks(s.fsys)
	return nil
}

func handlersPhase(s *state) error {
	for _, route := range s.routes {
		if route.Kind != RouteAPI {
			continue
		}
		if handler, ok := LoadHandler(s.fsys, route, s.bag); ok {
			s.handlers = append(s.handlers, handler)
		}
	}
	return nil
}

func (s *state) islandNames() map[string]bool {
	names := make(map[string]bool, len(s.islands))
	for name := range s.islands {
		names[name] = true
	}
	return names
}

func (s *state) linker() *Linker {
	return NewLinker(s.routes).Serving(s.served()...)
}

func (s *state) served() []string {
	files := append([]string(nil), s.config.Routing.Reserved...)
	public, err := assets.Public(s.fsys)
	if err == nil {
		for _, asset := range public {
			files = append(files, asset.Path)
		}
	}
	return append(files, seo.SitemapPath, seo.RobotsPath)
}

func componentsPhase(s *state) error {
	s.islands = DiscoverIslands(s.fsys)
	for name, file := range DiscoverComponents(s.fsys) {
		component, ok := LoadComponent(s.fsys, name, file, s.bag)
		if !ok {
			continue
		}
		Check(component.Document, component.File, component.Schema, s.bag)
		s.linker().Check(component.Document, component.File, s.bag)
		s.components[name] = component
	}
	return nil
}

func templatesPhase(s *state) error {
	for _, route := range s.routes {
		for _, layout := range route.Layouts {
			s.compileTemplate(layout)
		}
		if route.Kind == RoutePage {
			s.compileTemplate(route.File)
		}
	}
	for _, fallback := range s.fallbacks {
		for _, layout := range fallback.Layouts {
			s.compileTemplate(layout)
		}
		s.compileTemplate(fallback.File)
	}
	return nil
}

func (s *state) compileTemplate(file string) {
	if _, done := s.planOf[file]; done {
		return
	}
	template, ok := ReadTemplate(s.fsys, file, s.bag)
	if !ok {
		return
	}
	model := s.model(template)
	Check(template.Document, file, model, s.bag)
	CheckContexts(template.Document, file, s.bag)
	CheckFragments(template.Document, file, model, s.bag)
	CheckIslands(template.Document, file, model, s.components, s.islandNames(), s.bag)
	s.linker().Check(template.Document, file, s.bag)
	s.goMessages(template)
	s.templates[file] = template
	s.planOf[file] = uint32(len(s.plans))
	s.plans = append(s.plans, Lower(template.Document, file, LowerOptions{
		IsLayout:   template.IsLayout,
		Components: s.components,
		Assets:     s.assetTags,
		Messages:   s.messages,
		Islands:    s.islandNames(),
		Classes:    s.inventory,
		Deferred:   deferredNames(model),
		Fetches:    s.config.Fragments.Fetches(),
	}, s.bag))
}

func deferredNames(model *schema.Schema) map[string]bool {
	names := map[string]bool{}
	for _, name := range schema.Deferred(model) {
		names[name] = true
	}
	return names
}

func (s *state) model(template Template) *schema.Schema {
	sources := template.Sources()
	if len(sources) == 0 {
		return nil
	}
	model := schema.Parse(sources, s.bag)
	s.schemas[template.File] = model
	return model
}

func manifestPhase(s *state) error {
	manifest := &ir.Manifest{Version: ir.Version, Plans: s.plans}
	manifest.Messages = s.messages.Keys()
	manifest.Catalogs = BuildCatalogs(s.messages, s.catalogs, s.config.I18n.Locales,
		s.config.I18n.DefaultLocale, len(s.catalogs) > 0, s.bag)
	CheckPlurals(s.messages, s.catalogs, s.config.I18n.Locales, s.bag)
	for _, route := range s.routes {
		if route.Kind != RoutePage {
			continue
		}
		planIndex, ok := s.planOf[route.File]
		if !ok {
			continue
		}
		manifest.Routes = append(manifest.Routes, ir.Route{
			Pattern:     route.Pattern,
			Name:        route.Name,
			Plan:        planIndex,
			LayoutChain: s.chainOf(route.Layouts),
			Class:       s.classOf(route),
		})
	}
	for _, fallback := range s.fallbacks {
		planIndex, ok := s.planOf[fallback.File]
		if !ok {
			continue
		}
		kind := ir.FallbackNotFound
		if fallback.Kind == "error" {
			kind = ir.FallbackError
		}
		manifest.Fallbacks = append(manifest.Fallbacks, ir.Fallback{
			Prefix:      fallback.Prefix,
			Name:        fallback.Name,
			Kind:        kind,
			Plan:        planIndex,
			LayoutChain: s.chainOf(fallback.Layouts),
		})
	}
	s.manifest = manifest
	return nil
}

func (s *state) chainOf(layouts []string) []uint32 {
	chain := make([]uint32, 0, len(layouts))
	for _, layout := range layouts {
		if index, ok := s.planOf[layout]; ok {
			chain = append(chain, index)
		}
	}
	return chain
}

func (s *state) classOf(route Route) ir.RouteClass {
	if len(ParamsOf(route.Pattern)) > 0 {
		return ir.ClassDynamic
	}
	if s.needsRequest(route) {
		return ir.ClassDynamic
	}
	return ir.ClassStatic
}

func (s *state) needsRequest(route Route) bool {
	for _, file := range append([]string{route.File}, route.Layouts...) {
		template, ok := s.templates[file]
		if ok && (template.HasLoader() || template.HasSubmit()) {
			return true
		}
	}
	return false
}
