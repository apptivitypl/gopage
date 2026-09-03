package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sonquer/rill/internal/ir"
)

const (
	benchRoutes   = 50
	benchMessages = 300
)

func benchManifest() *ir.Manifest {
	manifest := &ir.Manifest{
		Version: ir.Version,
		Plans: []ir.Plan{
			{
				Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 6}, {Kind: ir.OpOutlet}, {Kind: ir.OpStatic, A: 6, B: 7}},
				Blob:     []byte("<main></main>"),
				Capacity: 32,
			},
			{
				Ops:      []ir.Op{{Kind: ir.OpStatic, A: 0, B: 5}},
				Blob:     []byte("<home"),
				Capacity: 16,
			},
		},
		Routes: []ir.Route{
			{Pattern: "/", Name: "index", Plan: 1, LayoutChain: []uint32{0}, Class: ir.ClassStatic},
		},
	}
	for i := range benchRoutes {
		manifest.Routes = append(manifest.Routes,
			ir.Route{
				Pattern: fmt.Sprintf("/section%d/page", i),
				Name:    fmt.Sprintf("section%d.page", i),
				Plan:    1,
				Class:   ir.ClassStatic,
			},
			ir.Route{
				Pattern: fmt.Sprintf("/section%d/[id]", i),
				Name:    fmt.Sprintf("section%d.id", i),
				Plan:    1,
				Class:   ir.ClassDynamic,
			})
	}
	texts := make([][ir.PluralForms]string, benchMessages)
	for i := range benchMessages {
		manifest.Messages = append(manifest.Messages, fmt.Sprintf("message.key.%d", i))
		texts[i] = [ir.PluralForms]string{fmt.Sprintf("text number %d", i)}
	}
	manifest.Catalogs = []ir.Catalog{{Locale: "en", Texts: texts}}
	return manifest
}

func BenchmarkRouterMatch(b *testing.B) {
	router := NewRouter(benchManifest().Routes)
	target := fmt.Sprintf("/section%d/1234", benchRoutes-1)
	if _, _, ok := router.Match(target); !ok {
		b.Fatalf("%s does not match", target)
	}
	b.ReportAllocs()
	for b.Loop() {
		router.Match(target)
	}
}

func BenchmarkRouterMiss(b *testing.B) {
	router := NewRouter(benchManifest().Routes)
	b.ReportAllocs()
	for b.Loop() {
		router.Match("/no/such/route")
	}
}

func BenchmarkTranslator(b *testing.B) {
	app := New(Options{Manifest: benchManifest()})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = withLocale(request, "en")
	if got := app.translator(request)("message.key.7", 0, false); got != "text number 7" {
		b.Fatalf("translator returned %q", got)
	}
	b.ReportAllocs()
	for b.Loop() {
		app.translator(request)
	}
}

func BenchmarkServeRequest(b *testing.B) {
	app := New(Options{Manifest: benchManifest()})
	handler := app.Handler()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		b.Fatalf("status = %d", recorder.Code)
	}
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
}
