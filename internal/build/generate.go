package build

import (
	"fmt"
	"go/format"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/apptivitypl/gopage/internal/assets"
	"github.com/apptivitypl/gopage/internal/codegen"
	"github.com/apptivitypl/gopage/internal/compile"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/paths"
	"github.com/apptivitypl/gopage/internal/schema"
)

const (
	RegistryGo  = paths.GenRoot + "/registry.go"
	AppGo       = paths.GenRoot + "/app.go"
	EmbedGo     = paths.GenRoot + "/embed.go"
	ServedGo    = paths.GenRoot + "/embed_js.go"
	packageName = "gen"
)

type generated struct {
	Package  string
	Route    string
	Meta     bool
	Submit   bool
	Loader   bool
	Params   bool
	Form     string
	Deferred []string
}

var reserved = map[string]bool{"props": true, "styles": true, "public": true, "bundles": true}

func isPackage(name string) bool {
	for _, letter := range name {
		if letter != '_' && !unicode.IsLetter(letter) && !unicode.IsDigit(letter) {
			return false
		}
	}
	return name != ""
}

func writeGo(path string, source []byte) error {
	formatted, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("%s: generated code does not parse: %w", path, err)
	}
	return writeFile(path, formatted)
}

func cleanPackages(dir string) error {
	entries, err := os.ReadDir(filepath.Join(dir, paths.GenRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || reserved[entry.Name()] || !isPackage(entry.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, paths.GenRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func writeGenerated(dir, module string, result compile.Result) ([]generated, error) {
	if err := cleanPackages(dir); err != nil {
		return nil, err
	}
	targets := loaderRoutes(result)
	if len(targets)+len(result.Handlers) > 0 && module == "" {
		resolved, err := moduleOf(dir)
		if err != nil {
			return nil, err
		}
		module = resolved
	}
	var packages []generated
	for _, route := range targets {
		template := result.Templates[route.File]
		pkg := codegen.PackageName(route.Name)
		source, err := codegen.Render(codegen.File{
			Package:    pkg,
			SourceFile: route.File,
			SourceLine: template.FirstLine,
			Source:     template.Frontmatter,
			Schema:     result.Schemas[route.File],
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", route.File, err)
		}
		target := filepath.Join(dir, paths.GenRoot, pkg, "page.go")
		if err := writeFile(target, source); err != nil {
			return nil, err
		}
		var bag diag.Bag
		entry := generated{
			Package:  pkg,
			Route:    route.Name,
			Meta:     template.HasMeta(),
			Loader:   template.HasLoader(),
			Params:   template.LoaderTakesParams(),
			Form:     template.FormType(&bag),
			Deferred: schema.Deferred(result.Schemas[route.File]),
		}
		entry.Submit = entry.Form != ""
		if bag.HasErrors() {
			return nil, &Error{Diagnostics: bag.Sorted(), Sources: map[string]string{route.File: template.Source}}
		}
		if err := writeGo(filepath.Join(dir, paths.GenRoot, pkg, "provider.go"), provider(entry)); err != nil {
			return nil, err
		}
		packages = append(packages, entry)
	}
	var handlers []generated
	for _, handler := range result.Handlers {
		pkg := codegen.PackageName(handler.Route)
		if err := writeGo(filepath.Join(dir, paths.GenRoot, pkg, "handler.go"), adapter(pkg, handler, module)); err != nil {
			return nil, err
		}
		handlers = append(handlers, generated{Package: pkg, Route: handler.Route})
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Package < packages[j].Package })
	sort.Slice(handlers, func(i, j int) bool { return handlers[i].Package < handlers[j].Package })
	if err := writeGo(filepath.Join(dir, filepath.FromSlash(RegistryGo)), registry(module, packages, handlers)); err != nil {
		return nil, err
	}
	if err := writeGo(filepath.Join(dir, filepath.FromSlash(EmbedGo)), embedded()); err != nil {
		return nil, err
	}
	if err := writeGo(filepath.Join(dir, filepath.FromSlash(ServedGo)), served()); err != nil {
		return nil, err
	}
	return packages, writeGo(filepath.Join(dir, filepath.FromSlash(AppGo)), app())
}

func Bootstrap(dir string) error {
	module, err := moduleOf(dir)
	if err != nil {
		return err
	}
	sources := map[string][]byte{
		RegistryGo: registry(module, nil, nil),
		AppGo:      app(),
		EmbedGo:    embedded(),
		ServedGo:   served(),
	}
	for name, data := range sources {
		if err := writeGo(filepath.Join(dir, filepath.FromSlash(name)), data); err != nil {
			return err
		}
	}
	files := map[string][]byte{
		paths.Manifest:                              nil,
		paths.GenConfig:                             nil,
		paths.GenStyles + "/.keep":                  nil,
		paths.GenBundles + "/.keep":                 nil,
		paths.GenBundles + "/" + assets.PreloadFile: nil,
		paths.GenPublic + "/.keep":                  nil,
	}
	for name, data := range files {
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(name)), data); err != nil {
			return err
		}
	}
	return nil
}

func app() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gopage. DO NOT EDIT.\n\npackage %s\n\n", packageName)
	fmt.Fprintf(&b, "import (\n\t_ \"embed\"\n\n\t%q\n)\n\n", codegen.GopageImport)
	fmt.Fprintf(&b, "//go:embed %s\nvar Manifest []byte\n\n", path.Base(paths.Manifest))
	fmt.Fprintf(&b, "//go:embed %s\nvar Config []byte\n\n", path.Base(paths.GenConfig))
	fmt.Fprintf(&b, "//go:embed %s/%s\nvar Preload []byte\n\n", path.Base(paths.GenBundles), assets.PreloadFile)
	b.WriteString("func Options() gopage.Options {\n")
	b.WriteString("\treturn gopage.Options{\n")
	b.WriteString("\t\tManifest: Manifest,\n\t\tConfig:   Config,\n\t\tPreload:  Preload,\n\t\tStatic:   Static,\n\t\tBundles:  Bundles,\n\t\tPublic:   Public,\n")
	b.WriteString("\t\tProps:    Props(),\n\t\tMeta:     Meta(),\n\t\tSubmit:   Submit(),\n\t\tAPI:      API(),\n\t\tDeferred: Deferred(),\n")
	b.WriteString("\t}\n}\n")
	return []byte(b.String())
}

func embedded() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gopage. DO NOT EDIT.\n\n//go:build !js\n\npackage %s\n\n", packageName)
	b.WriteString("import \"embed\"\n\n")
	fmt.Fprintf(&b, "//go:embed all:%s\nvar Static embed.FS\n\n", path.Base(paths.GenStyles))
	fmt.Fprintf(&b, "//go:embed all:%s\nvar Bundles embed.FS\n\n", path.Base(paths.GenBundles))
	fmt.Fprintf(&b, "//go:embed all:%s\nvar Public embed.FS\n", path.Base(paths.GenPublic))
	return []byte(b.String())
}

func served() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gopage. DO NOT EDIT.\n\n//go:build js\n\npackage %s\n\n", packageName)
	b.WriteString("import \"embed\"\n\n")
	b.WriteString("var (\n\tStatic  embed.FS\n\tBundles embed.FS\n\tPublic  embed.FS\n)\n")
	return []byte(b.String())
}

func loaderRoutes(result compile.Result) []compile.Route {
	var routes []compile.Route
	for _, route := range result.Routes {
		if route.Kind != compile.RoutePage {
			continue
		}
		if template, ok := result.Templates[route.File]; ok && (template.HasLoader() || template.HasSubmit()) {
			routes = append(routes, route)
		}
	}
	for _, fallback := range result.Fallbacks {
		if template, ok := result.Templates[fallback.File]; ok && template.HasLoader() {
			routes = append(routes, compile.Route{
				Pattern: fallback.Prefix,
				Name:    fallback.Name,
				Kind:    compile.RoutePage,
				File:    fallback.File,
				Layouts: fallback.Layouts,
			})
		}
	}
	return routes
}

func provider(entry generated) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gopage. DO NOT EDIT.\n\npackage %s\n\n", entry.Package)
	fmt.Fprintf(&b, "import (\n\t\"net/http\"\n\n\t%q\n)\n\n", codegen.GopageImport)
	fmt.Fprintf(&b, "const Route = %q\n\n", entry.Route)
	if entry.Loader {
		b.WriteString("func Provider(request *http.Request, params gopage.Params) (gopage.Accessible, error) {\n")
		fmt.Fprintf(&b, "\tprops, err := Load(%s)\n", loaderArgs(entry))
		b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		b.WriteString("\treturn props, nil\n}\n")
	}
	if entry.Meta {
		b.WriteString("\nfunc MetaProvider(request *http.Request, params gopage.Params) (gopage.Meta, error) {\n")
		b.WriteString("\tctx := gopage.NewCtx(request, params)\n")
		fmt.Fprintf(&b, "\tprops, err := Load(%s)\n", metaLoaderArgs(entry))
		b.WriteString("\tif err != nil {\n\t\treturn gopage.Meta{}, nil\n\t}\n")
		b.WriteString("\treturn Meta(ctx, props), nil\n}\n")
	}
	for _, name := range entry.Deferred {
		fmt.Fprintf(&b, "\nfunc %sProvider(request *http.Request, params gopage.Params) (gopage.Accessible, error) {\n", name)
		fmt.Fprintf(&b, "\tvalue, err := %s(gopage.NewCtx(request, params))\n", name)
		b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		fmt.Fprintf(&b, "\treturn %s%s{value: value}, nil\n}\n", codegen.DeferredPrefix, name)
	}
	if entry.Submit {
		b.WriteString("\nfunc SubmitProvider(request *http.Request, params gopage.Params) (gopage.Action, gopage.FormResult, error) {\n")
		fmt.Fprintf(&b, "\tvar submitted %s\n", entry.Form)
		b.WriteString("\tresult, err := gopage.DecodeForm(request, &submitted)\n")
		b.WriteString("\tif err != nil {\n\t\treturn nil, result, err\n\t}\n")
		b.WriteString("\tif !result.Valid() {\n\t\treturn nil, result, nil\n\t}\n")
		b.WriteString("\taction, err := Submit(gopage.NewCtx(request, params), params, submitted)\n")
		b.WriteString("\treturn action, result, err\n}\n")
	}
	return []byte(b.String())
}

func adapter(pkg string, handler compile.Handler, module string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gopage. DO NOT EDIT.\n\npackage %s\n\n", pkg)
	b.WriteString("import (\n\t\"net/http\"\n\n")
	fmt.Fprintf(&b, "\t%q\n", codegen.GopageImport)
	fmt.Fprintf(&b, "\troute %q\n)\n\n", module+"/"+handler.Dir)
	fmt.Fprintf(&b, "const Pattern = %q\n\n", handler.Pattern)
	fmt.Fprintf(&b, "var params = %s\n\n", paramsLiteral(handler.Pattern))
	b.WriteString("func Handler() http.Handler {\n")
	b.WriteString("\treturn gopage.API(map[string]gopage.APIHandler{\n")
	for _, method := range handler.Methods {
		fmt.Fprintf(&b, "\t\t%q: func(request *http.Request) (gopage.Response, error) {\n", method)
		b.WriteString("\t\t\tbound := gopage.ParamsFrom(request, params)\n")
		fmt.Fprintf(&b, "\t\t\treturn route.%s(gopage.NewCtx(request, bound), bound)\n", method)
		b.WriteString("\t\t},\n")
	}
	b.WriteString("\t})\n}\n")
	return []byte(b.String())
}

func paramsLiteral(pattern string) string {
	names := compile.MuxParams(pattern)
	if len(names) == 0 {
		return "[]string(nil)"
	}
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

func loaderArgs(entry generated) string {
	if entry.Params {
		return "gopage.NewCtx(request, params), params"
	}
	return "gopage.NewCtx(request, params)"
}

func metaLoaderArgs(entry generated) string {
	if entry.Params {
		return "ctx, params"
	}
	return "ctx"
}

func registry(module string, packages, handlers []generated) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gopage. DO NOT EDIT.\n\npackage %s\n\n", packageName)
	b.WriteString("import (\n")
	if len(handlers) > 0 {
		b.WriteString("\t\"net/http\"\n\n")
	}
	fmt.Fprintf(&b, "\t%q\n", codegen.GopageImport)
	for _, pkg := range append(append([]generated{}, packages...), handlers...) {
		fmt.Fprintf(&b, "\t%q\n", module+"/"+paths.GenRoot+"/"+pkg.Package)
	}
	b.WriteString(")\n\n")
	b.WriteString("func Props() map[string]gopage.PropsProvider {\n")
	b.WriteString("\treturn map[string]gopage.PropsProvider{\n")
	for _, pkg := range packages {
		if pkg.Loader {
			fmt.Fprintf(&b, "\t\t%s.Route: %s.Provider,\n", pkg.Package, pkg.Package)
		}
	}
	b.WriteString("\t}\n}\n\n")
	b.WriteString("func Meta() map[string]gopage.MetaProvider {\n")
	b.WriteString("\treturn map[string]gopage.MetaProvider{\n")
	for _, pkg := range packages {
		if pkg.Meta {
			fmt.Fprintf(&b, "\t\t%s.Route: %s.MetaProvider,\n", pkg.Package, pkg.Package)
		}
	}
	b.WriteString("\t}\n}\n\n")
	b.WriteString("func Submit() map[string]gopage.SubmitProvider {\n")
	b.WriteString("\treturn map[string]gopage.SubmitProvider{\n")
	for _, pkg := range packages {
		if pkg.Submit {
			fmt.Fprintf(&b, "\t\t%s.Route: %s.SubmitProvider,\n", pkg.Package, pkg.Package)
		}
	}
	b.WriteString("\t}\n}\n\n")
	b.WriteString("func Deferred() map[string]gopage.DeferredProvider {\n")
	b.WriteString("\treturn map[string]gopage.DeferredProvider{\n")
	for _, pkg := range packages {
		for _, name := range pkg.Deferred {
			fmt.Fprintf(&b, "\t\t%q: %s.%sProvider,\n", name, pkg.Package, name)
		}
	}
	b.WriteString("\t}\n}\n\n")
	b.WriteString("func API() map[string]http.Handler {\n")
	b.WriteString("\treturn map[string]http.Handler{\n")
	for _, pkg := range handlers {
		fmt.Fprintf(&b, "\t\t%s.Pattern: %s.Handler(),\n", pkg.Package, pkg.Package)
	}
	b.WriteString("\t}\n}\n")
	return []byte(b.String())
}
