package assets

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func sample() fstest.MapFS {
	return fstest.MapFS{
		"styles/app.css":      &fstest.MapFile{Data: []byte("body{margin:0}")},
		"styles/js/island.js": &fstest.MapFile{Data: []byte("export const mount = () => {}")},
		"styles/img/logo.svg": &fstest.MapFile{Data: []byte("<svg></svg>")},
		"app/page.rill":       &fstest.MapFile{Data: []byte("<h1>home</h1>")},
	}
}

func collect(t *testing.T, fsys fstest.MapFS) []Asset {
	t.Helper()
	list, err := Collect(fsys)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	return list
}

func TestCollectHashesEveryFile(t *testing.T) {
	list := collect(t, sample())
	if len(list) != 3 {
		t.Fatalf("assets = %+v", list)
	}
	if list[0].Source != "styles/app.css" || !strings.HasPrefix(list[0].Path, "/assets/app.") {
		t.Errorf("asset = %+v", list[0])
	}
	if !strings.HasSuffix(list[0].Path, ".css") || list[0].Kind != KindStyle {
		t.Errorf("asset = %+v", list[0])
	}
	if list[0].ETag == "" || list[0].Size != 14 {
		t.Errorf("asset = %+v", list[0])
	}
}

func TestNestedPathsAreKept(t *testing.T) {
	list := collect(t, sample())
	var script Asset
	for _, asset := range list {
		if asset.Kind == KindScript {
			script = asset
		}
	}
	if !strings.HasPrefix(script.Path, "/assets/js/island.") {
		t.Errorf("path = %q", script.Path)
	}
}

func TestContentIsPartOfTheHash(t *testing.T) {
	first := collect(t, sample())[0]
	changed := sample()
	changed["styles/app.css"] = &fstest.MapFile{Data: []byte("body{margin:1px}")}
	second := collect(t, changed)[0]
	if first.Path == second.Path {
		t.Errorf("changing the file must change the path: %q", first.Path)
	}
}

func TestMissingStaticDirectoryIsNotAnError(t *testing.T) {
	list, err := Collect(fstest.MapFS{"app/page.rill": &fstest.MapFile{Data: []byte("<h1>home</h1>")}})
	if err != nil || list != nil {
		t.Errorf("list = %v, err = %v", list, err)
	}
}

func TestUnknownExtensionsFallBack(t *testing.T) {
	list := collect(t, fstest.MapFS{"styles/data.unknown": &fstest.MapFile{Data: []byte("x")}})
	if list[0].Kind != KindOther || list[0].Type != "application/octet-stream" {
		t.Errorf("asset = %+v", list[0])
	}
}

func TestTagsCoverStylesAndScripts(t *testing.T) {
	tags := Tags(collect(t, sample()))
	if !strings.Contains(tags, `<link rel="stylesheet" href="/assets/app.`) {
		t.Errorf("tags = %q", tags)
	}
	if !strings.Contains(tags, `<script type="module" async src="/assets/js/island.`) {
		t.Errorf("tags = %q", tags)
	}
	if strings.Contains(tags, "logo") {
		t.Errorf("tags = %q, images are not linked", tags)
	}
}

func TestLinkHeaderPreloads(t *testing.T) {
	header := Link(collect(t, sample()))
	if !strings.Contains(header, "rel=preload; as=style") || !strings.Contains(header, "rel=modulepreload") {
		t.Errorf("link = %q", header)
	}
	if Link(nil) != "" {
		t.Errorf("link = %q, want empty", Link(nil))
	}
}

func handler(t *testing.T, fsys fstest.MapFS) (http.Handler, []Asset) {
	t.Helper()
	list := collect(t, fsys)
	served, err := Handler(fsys, list)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	return served, list
}

func TestHandlerServesWithAnEtag(t *testing.T) {
	served, list := handler(t, sample())
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, list[0].Path, nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "body{margin:0}" {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("ETag") != list[0].ETag {
		t.Errorf("etag = %q", recorder.Header().Get("ETag"))
	}
	if recorder.Header().Get("Cache-Control") != CacheControl {
		t.Errorf("cache control = %q", recorder.Header().Get("Cache-Control"))
	}
	if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/css") {
		t.Errorf("content type = %q", recorder.Header().Get("Content-Type"))
	}
}

func TestHandlerAnswersAMatchingEtagWithNotModified(t *testing.T) {
	served, list := handler(t, sample())
	request := httptest.NewRequest(http.MethodGet, list[0].Path, nil)
	request.Header.Set("If-None-Match", list[0].ETag)
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotModified || recorder.Body.Len() != 0 {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerRejectsUnknownPathsAndMethods(t *testing.T) {
	served, list := handler(t, sample())
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/missing.css", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	served.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, list[0].Path, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("status = %d, allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func TestHandlerReportsAnUnreadableAsset(t *testing.T) {
	if _, err := Handler(fstest.MapFS{}, []Asset{{Source: "styles/gone.css"}}); err == nil {
		t.Error("a missing asset must be reported")
	}
}

type failingFS struct {
	inner  fs.FS
	broken string
}

func (f failingFS) Open(name string) (fs.File, error) {
	if name == f.broken {
		return nil, fs.ErrPermission
	}
	return f.inner.Open(name)
}

func TestUnreadableAssetStopsCollection(t *testing.T) {
	if _, err := Collect(failingFS{inner: sample(), broken: "styles/app.css"}); err == nil {
		t.Error("an unreadable asset must be reported")
	}
}

func TestAnUnreadableDirectoryStopsCollection(t *testing.T) {
	if _, err := Collect(failingFS{inner: sample(), broken: "styles/js"}); err == nil {
		t.Error("an unreadable directory must be reported")
	}
}

func TestDescribeKeepsTheNameVerbatim(t *testing.T) {
	asset := Describe("rill.client.ABC.js", []byte("export{}"))
	if asset.Path != "/assets/rill.client.ABC.js" {
		t.Errorf("path = %q, want the bundler name kept", asset.Path)
	}
	if asset.Source != "bundles/rill.client.ABC.js" || asset.Kind != KindScript {
		t.Errorf("asset = %+v", asset)
	}
	if asset.ETag == "" || asset.Size != 8 {
		t.Errorf("asset = %+v", asset)
	}
}

func TestVerbatimListsTheBundleStore(t *testing.T) {
	fsys := fstest.MapFS{
		"bundles/rill.client.ABC.js": &fstest.MapFile{Data: []byte("export{}")},
		"bundles/island.XYZ.js":      &fstest.MapFile{Data: []byte("export{}")},
		"bundles/.keep":              &fstest.MapFile{Data: nil},
		"bundles/nested/x.js":        &fstest.MapFile{Data: []byte("export{}")},
	}
	list, err := Verbatim(fsys)
	if err != nil {
		t.Fatalf("Verbatim: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Path != "/assets/.keep" || list[1].Path != "/assets/island.XYZ.js" {
		t.Errorf("list = %+v", list)
	}
}

func TestVerbatimWithoutAStoreIsNotAnError(t *testing.T) {
	list, err := Verbatim(fstest.MapFS{})
	if err != nil || list != nil {
		t.Errorf("list = %v, err = %v", list, err)
	}
}

func TestVerbatimReportsAnUnreadableFile(t *testing.T) {
	fsys := failingFS{
		inner:  fstest.MapFS{"bundles/a.js": &fstest.MapFile{Data: []byte("export{}")}},
		broken: "bundles/a.js",
	}
	if _, err := Verbatim(fsys); err == nil {
		t.Error("an unreadable bundle must be reported")
	}
}

func TestServeMergesStores(t *testing.T) {
	static := fstest.MapFS{"styles/app.css": &fstest.MapFile{Data: []byte("body{}")}}
	bundles := fstest.MapFS{"bundles/rill.client.ABC.js": &fstest.MapFile{Data: []byte("export{}")}}
	staticList := collect(t, static)
	bundleList, err := Verbatim(bundles)
	if err != nil {
		t.Fatal(err)
	}
	handler, all, err := Serve([]Store{{FS: static, Files: staticList}, {FS: bundles, Files: bundleList}})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if len(all) != 2 || all[0].Path >= all[1].Path {
		t.Errorf("assets = %+v, want them sorted", all)
	}
	for _, asset := range all {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, asset.Path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s = %d", asset.Path, recorder.Code)
		}
	}
}

func TestServeWithoutAnythingToServe(t *testing.T) {
	handler, all, err := Serve(nil)
	if handler != nil || all != nil || err != nil {
		t.Errorf("handler = %v, all = %v, err = %v", handler, all, err)
	}
}

func TestServeReportsAnUnreadableAsset(t *testing.T) {
	if _, _, err := Serve([]Store{{FS: fstest.MapFS{}, Files: []Asset{{Source: "styles/gone.css"}}}}); err == nil {
		t.Error("a missing asset must be reported")
	}
}

func compressedFS() fstest.MapFS {
	return fstest.MapFS{
		"styles/app.css":    &fstest.MapFile{Data: []byte("body{margin:0}")},
		"styles/app.css.br": &fstest.MapFile{Data: []byte("brotli-bytes")},
		"styles/app.css.gz": &fstest.MapFile{Data: []byte("gzip-bytes")},
	}
}

func TestCollectSkipsCompressedVariants(t *testing.T) {
	list := collect(t, compressedFS())
	if len(list) != 1 || list[0].Source != "styles/app.css" {
		t.Errorf("list = %+v, want only the original", list)
	}
}

func TestVerbatimSkipsCompressedVariants(t *testing.T) {
	list, err := Verbatim(fstest.MapFS{
		"bundles/a.js":    &fstest.MapFile{Data: []byte("export{}")},
		"bundles/a.js.br": &fstest.MapFile{Data: []byte("x")},
		"bundles/a.js.gz": &fstest.MapFile{Data: []byte("y")},
	})
	if err != nil {
		t.Fatalf("Verbatim: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list = %+v", list)
	}
}

func request(t *testing.T, handler http.Handler, path, accept string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if accept != "" {
		req.Header.Set("Accept-Encoding", accept)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestThePrecompressedVariantIsPreferred(t *testing.T) {
	fsys := compressedFS()
	served, list := handler(t, fsys)
	path := list[0].Path

	cases := map[string]struct{ encoding, body string }{
		"br, gzip": {"br", "brotli-bytes"},
		"gzip":     {"gzip", "gzip-bytes"},
		"":         {"", "body{margin:0}"},
		"deflate":  {"", "body{margin:0}"},
	}
	for accept, want := range cases {
		recorder := request(t, served, path, accept)
		if got := recorder.Header().Get("Content-Encoding"); got != want.encoding {
			t.Errorf("accept %q: encoding = %q, want %q", accept, got, want.encoding)
		}
		if recorder.Body.String() != want.body {
			t.Errorf("accept %q: body = %q, want %q", accept, recorder.Body.String(), want.body)
		}
		if !strings.Contains(recorder.Header().Get("Vary"), "Accept-Encoding") {
			t.Errorf("accept %q: vary = %q", accept, recorder.Header().Get("Vary"))
		}
	}
}

func TestQualityValuesAreIgnored(t *testing.T) {
	served, list := handler(t, compressedFS())
	recorder := request(t, served, list[0].Path, "gzip;q=0.8, br;q=1.0")
	if recorder.Header().Get("Content-Encoding") != "br" {
		t.Errorf("encoding = %q", recorder.Header().Get("Content-Encoding"))
	}
}

func TestAnAssetWithoutVariantsIsServedPlain(t *testing.T) {
	served, list := handler(t, fstest.MapFS{"styles/app.css": &fstest.MapFile{Data: []byte("body{}")}})
	recorder := request(t, served, list[0].Path, "br, gzip")
	if recorder.Header().Get("Content-Encoding") != "" || recorder.Body.String() != "body{}" {
		t.Errorf("encoding = %q, body = %q", recorder.Header().Get("Content-Encoding"), recorder.Body.String())
	}
}

func TestServeReadsVariantsToo(t *testing.T) {
	fsys := compressedFS()
	list := collect(t, fsys)
	served, _, err := Serve([]Store{{FS: fsys, Files: list}})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	recorder := request(t, served, list[0].Path, "br")
	if recorder.Body.String() != "brotli-bytes" {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestATransformShapesTheAssetBeforeItIsHashed(t *testing.T) {
	shrink := func(_ Asset, data []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(data), " ", "")), nil
	}
	list, err := CollectWith(fstest.MapFS{"styles/app.css": &fstest.MapFile{Data: []byte("body { margin: 0 }")}}, shrink)
	if err != nil {
		t.Fatalf("CollectWith: %v", err)
	}
	plain := collect(t, fstest.MapFS{"styles/app.css": &fstest.MapFile{Data: []byte("body { margin: 0 }")}})
	if list[0].Path == plain[0].Path {
		t.Errorf("the transform must change the hash: %q", list[0].Path)
	}
	if list[0].Size != 14 {
		t.Errorf("size = %d, want the transformed length", list[0].Size)
	}
}

func TestAFailingTransformIsReported(t *testing.T) {
	failing := func(Asset, []byte) ([]byte, error) { return nil, fs.ErrInvalid }
	if _, err := CollectWith(fstest.MapFS{"styles/app.css": &fstest.MapFile{Data: []byte("x")}}, failing); err == nil {
		t.Error("a transform that fails must be reported")
	}
}

func TestPublicFilesKeepTheirPath(t *testing.T) {
	list, err := Public(fstest.MapFS{
		"public/favicon.ico":  &fstest.MapFile{Data: []byte("icon")},
		"public/icon.svg":     &fstest.MapFile{Data: []byte("<svg/>")},
		"public/img/hero.png": &fstest.MapFile{Data: []byte("png")},
		"styles/app.css":      &fstest.MapFile{Data: []byte("body{}")},
	})
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	paths := make([]string, 0, len(list))
	for _, asset := range list {
		paths = append(paths, asset.Path)
		if asset.Cache != PublicCache {
			t.Errorf("%s cache = %q, want the short public cache", asset.Path, asset.Cache)
		}
		if asset.ETag == "" {
			t.Errorf("%s has no etag", asset.Path)
		}
	}
	want := []string{"/favicon.ico", "/icon.svg", "/img/hero.png"}
	if !slices.Equal(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestAProjectWithoutAPublicDirectoryIsFine(t *testing.T) {
	list, err := Public(fstest.MapFS{"styles/app.css": &fstest.MapFile{Data: []byte("body{}")}})
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("list = %v, want none", list)
	}
}

func TestPublicFilesAreServedWithTheirOwnCacheHeader(t *testing.T) {
	fsys := fstest.MapFS{"public/favicon.ico": &fstest.MapFile{Data: []byte("icon")}}
	list, err := Public(fsys)
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	handler, _, err := Serve([]Store{{FS: fsys, Files: list}})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != PublicCache {
		t.Errorf("cache-control = %q, want %q", got, PublicCache)
	}
}

func TestPublicReportsAFileItCannotRead(t *testing.T) {
	if _, err := Public(brokenFS{}); err == nil {
		t.Error("a public file that cannot be read must be reported")
	}
}

type brokenFS struct{}

func (brokenFS) Open(name string) (fs.File, error) {
	if name == PublicDir {
		return fstest.MapFS{"favicon.ico": &fstest.MapFile{Data: []byte("icon")}}.Open(".")
	}
	return nil, errors.New("unreadable")
}

func TestPublicWalksNestedDirectories(t *testing.T) {
	list, err := Public(fstest.MapFS{
		"public/img/icons/small.svg": &fstest.MapFile{Data: []byte("<svg/>")},
	})
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	if len(list) != 1 || list[0].Path != "/img/icons/small.svg" {
		t.Errorf("list = %+v", list)
	}
}

type brokenSubtree struct{}

func (brokenSubtree) Open(name string) (fs.File, error) {
	switch name {
	case PublicDir:
		return fstest.MapFS{"img": &fstest.MapFile{Mode: fs.ModeDir}}.Open(".")
	default:
		return nil, errors.New("unreadable subtree")
	}
}

func TestPublicReportsASubtreeItCannotWalk(t *testing.T) {
	if _, err := Public(brokenSubtree{}); err == nil {
		t.Error("a public subtree that cannot be walked must be reported")
	}
}

func TestPublicFilesRevalidateWhileDeveloping(t *testing.T) {
	if got := publicCache(); got != PublicCache {
		t.Errorf("cache = %q, want a long window when serving for real", got)
	}
	t.Setenv(DevVar, "1")
	if got := publicCache(); got != PublicRefresh {
		t.Errorf("cache = %q, want an edited file to show up at once in dev", got)
	}
}

func TestVerbatimPreloadsOnlyWhatTheEntryNeedsEagerly(t *testing.T) {
	fsys := fstest.MapFS{
		"bundles/rill.client.AAA.js": &fstest.MapFile{Data: []byte("entry")},
		"bundles/island.EAGER.js":    &fstest.MapFile{Data: []byte("helper")},
		"bundles/island.LAZY.js":     &fstest.MapFile{Data: []byte("island")},
		"bundles/island.LAZY.js.br":  &fstest.MapFile{Data: []byte("x")},
		"bundles/" + PreloadFile:     &fstest.MapFile{Data: []byte("module island.EAGER.js\nisland Stars island.S.js island.EAGER.js\n")},
	}
	list, err := Verbatim(fsys)
	if err != nil {
		t.Fatalf("Verbatim: %v", err)
	}
	kinds := map[string]Kind{}
	for _, asset := range list {
		kinds[asset.Path] = asset.Kind
	}
	want := map[string]Kind{
		Prefix + "rill.client.AAA.js": KindScript,
		Prefix + "island.EAGER.js":    KindModule,
		Prefix + "island.LAZY.js":     KindOther,
	}
	for path, kind := range want {
		if kinds[path] != kind {
			t.Errorf("%s: kind = %v, want %v", path, kinds[path], kind)
		}
	}
	if _, served := kinds[Prefix+PreloadFile]; served {
		t.Error("the preload list must not be served as an asset")
	}
	link := Link(list)
	if strings.Contains(link, "island.LAZY.js") || !strings.Contains(link, "island.EAGER.js") {
		t.Errorf("link = %q, want lazy islands kept out of the early hints", link)
	}
	if tags := Tags(list); !strings.Contains(tags, `<link rel="modulepreload" href="/assets/island.EAGER.js">`) {
		t.Errorf("tags = %q", tags)
	}
}

func TestTheSidecarRoundTrips(t *testing.T) {
	sidecar := Sidecar{
		Links:   []string{"</assets/app.css>; rel=preload; as=style", "</fonts/mono.woff2>; rel=preload; as=font; crossorigin"},
		Modules: []string{"island.HELPER.js"},
		Islands: map[string][]string{"Stars": {"island.REACT.js", "island.STARS.js"}, "Ticker": {"island.T.js"}},
	}
	parsed := ParseSidecar(sidecar.Bytes())
	if parsed.Link() != sidecar.Link() || len(parsed.Modules) != 1 || len(parsed.Islands["Stars"]) != 2 {
		t.Errorf("parsed = %+v", parsed)
	}
	if got := ReadSidecar(fstest.MapFS{}); len(got.Links) != 0 || len(got.Islands) != 0 {
		t.Errorf("sidecar = %+v, want nothing without a file", got)
	}
	lazy := Sidecar{
		Modules: []string{"island.HELPER.js"},
		Islands: map[string][]string{"Stars": {"island.HELPER.js", "island.S.js"}, "Only": {"island.HELPER.js"}},
	}.IslandChunks()
	if len(lazy["Stars"]) != 1 || lazy["Stars"][0] != "island.S.js" {
		t.Errorf("lazy = %v, want the chunk the entry already preloads dropped", lazy)
	}
	if _, ok := lazy["Only"]; ok {
		t.Errorf("lazy = %v, want an island with nothing left dropped entirely", lazy)
	}
}

func TestAnInlinedStyleBecomesAStyleTagAndLeavesTheHints(t *testing.T) {
	list := []Asset{
		{Path: Prefix + "app.css", Kind: KindStyle, Inline: "body{margin:0}"},
		{Path: Prefix + "big.css", Kind: KindStyle},
		{Path: "/fonts/mono.woff2", Kind: KindFont},
	}
	tags := Tags(list)
	if !strings.Contains(tags, "<style>body{margin:0}</style>") || strings.Contains(tags, `href="/assets/app.css"`) {
		t.Errorf("tags = %q", tags)
	}
	if !strings.Contains(tags, `<link rel="stylesheet" href="/assets/big.css">`) {
		t.Errorf("tags = %q, want the large sheet linked", tags)
	}
	link := Link(list)
	if strings.Contains(link, "app.css") || !strings.Contains(link, "big.css") {
		t.Errorf("link = %q, want only the linked sheet preloaded", link)
	}
	if !strings.Contains(link, "</fonts/mono.woff2>; rel=preload; as=font; crossorigin") {
		t.Errorf("link = %q, want the font preloaded", link)
	}
}

func TestCollectOptionsInlinesSmallStyles(t *testing.T) {
	fsys := fstest.MapFS{
		"styles/app.css": &fstest.MapFile{Data: []byte("body{margin:0}")},
		"styles/big.css": &fstest.MapFile{Data: []byte("body{margin:0}")},
	}
	list, err := CollectOptions(fsys, Options{Inline: func(asset Asset, _ []byte) bool {
		return strings.Contains(asset.Source, "app")
	}})
	if err != nil {
		t.Fatalf("CollectOptions: %v", err)
	}
	inlined := map[string]bool{}
	for _, asset := range list {
		inlined[asset.Source] = asset.Inline != ""
	}
	if !inlined["styles/app.css"] || inlined["styles/big.css"] {
		t.Errorf("inlined = %v", inlined)
	}
	if got := kindOf(".woff2"); got != KindFont {
		t.Errorf("kind = %v", got)
	}
}

func TestAnUnreadableSidecarIsEmpty(t *testing.T) {
	if got := ReadSidecar(nil); len(got.Links) != 0 {
		t.Errorf("sidecar = %+v", got)
	}
	fsys := fstest.MapFS{"bundles/" + PreloadFile: &fstest.MapFile{Data: []byte("link </a.css>; rel=preload; as=style\nnoise line\n")}}
	if got := ReadSidecar(fsys); len(got.Links) != 1 || got.Link() != "</a.css>; rel=preload; as=style" {
		t.Errorf("sidecar = %+v", got)
	}
}

func ranged(t *testing.T, handler http.Handler, path, accept, span string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if accept != "" {
		req.Header.Set("Accept-Encoding", accept)
	}
	req.Header.Set("Range", span)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestARangeOverACompressedBodyIsRefused(t *testing.T) {
	served, list := handler(t, compressedFS())
	recorder := ranged(t, served, list[0].Path, "br", "bytes=0-3")

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want the whole representation, not a slice of the compressed stream", recorder.Code)
	}
	if got := recorder.Header().Get("Accept-Ranges"); got != "none" {
		t.Errorf("accept-ranges = %q, want none while an encoding is applied", got)
	}
	if got := recorder.Header().Get("Content-Range"); got != "" {
		t.Errorf("content-range = %q, want none", got)
	}
	if recorder.Body.String() != "brotli-bytes" {
		t.Errorf("body = %q, want the whole compressed stream", recorder.Body.String())
	}
}

func TestARangeOverAnIdentityBodyStillWorks(t *testing.T) {
	served, list := handler(t, compressedFS())
	recorder := ranged(t, served, list[0].Path, "", "bytes=0-3")

	if recorder.Code != http.StatusPartialContent {
		t.Errorf("status = %d, want 206 when nothing is encoded", recorder.Code)
	}
	if recorder.Body.String() != "body" {
		t.Errorf("body = %q, want the requested slice", recorder.Body.String())
	}
}

func TestEachEncodingCarriesItsOwnETag(t *testing.T) {
	served, list := handler(t, compressedFS())
	plain := request(t, served, list[0].Path, "")
	packed := request(t, served, list[0].Path, "br")

	left, right := plain.Header().Get("ETag"), packed.Header().Get("ETag")
	if left == "" || right == "" {
		t.Fatalf("etags = %q, %q", left, right)
	}
	if left == right {
		t.Errorf("etag = %q for both representations, want one per encoding", left)
	}
	if got := EncodedETag(`"abc"`, ""); got != `"abc"` {
		t.Errorf("EncodedETag with no coding = %q", got)
	}
	if got := EncodedETag("abc", "br"); got != "abc" {
		t.Errorf("EncodedETag on an unquoted tag = %q, want it left alone", got)
	}
}

func TestACompressedVariantStillAnswersConditionally(t *testing.T) {
	served, list := handler(t, compressedFS())
	first := request(t, served, list[0].Path, "br")
	etag := first.Header().Get("ETag")

	req := httptest.NewRequest(http.MethodGet, list[0].Path, nil)
	req.Header.Set("Accept-Encoding", "br")
	req.Header.Set("If-None-Match", etag)
	again := httptest.NewRecorder()
	served.ServeHTTP(again, req)

	if again.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304 for a tag the client already holds", again.Code)
	}
	if again.Body.Len() != 0 {
		t.Errorf("body = %q, want nothing on a 304", again.Body.String())
	}
}

func TestAConditionalRequestNamingAnotherTagIsServed(t *testing.T) {
	served, list := handler(t, compressedFS())
	req := httptest.NewRequest(http.MethodGet, list[0].Path, nil)
	req.Header.Set("Accept-Encoding", "br")
	req.Header.Set("If-None-Match", `"someone-else", W/"and-another"`)
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "brotli-bytes" {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestAHeadOfACompressedVariantCarriesNoBody(t *testing.T) {
	served, list := handler(t, compressedFS())
	req := httptest.NewRequest(http.MethodHead, list[0].Path, nil)
	req.Header.Set("Accept-Encoding", "br")
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Length"); got != "12" {
		t.Errorf("content-length = %q, want the length of the compressed body", got)
	}
}

func TestAWildcardConditionalMatches(t *testing.T) {
	served, list := handler(t, compressedFS())
	req := httptest.NewRequest(http.MethodGet, list[0].Path, nil)
	req.Header.Set("Accept-Encoding", "br")
	req.Header.Set("If-None-Match", "*")
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", recorder.Code)
	}
}

func TestAnInlinedSheetIsWrittenBeforeALinkedOne(t *testing.T) {
	list := []Asset{
		{Kind: KindStyle, Path: "/assets/app.css"},
		{Kind: KindStyle, Path: "/assets/critical.css", Inline: "body{margin:0}"},
		{Kind: KindScript, Path: "/assets/client.js"},
		{Kind: KindModule, Path: "/assets/island.js"},
	}
	tags := Tags(list)

	inline := strings.Index(tags, "<style>")
	linked := strings.Index(tags, `<link rel="stylesheet"`)
	if inline < 0 || linked < 0 {
		t.Fatalf("tags = %q", tags)
	}
	if inline > linked {
		t.Errorf("tags = %q, want the inlined sheet first so the full one can still override it", tags)
	}
	if script := strings.Index(tags, "<script"); script < linked {
		t.Errorf("tags = %q, want the stylesheets before the scripts", tags)
	}
	if !strings.Contains(tags, `<link rel="modulepreload" href="/assets/island.js">`) {
		t.Errorf("tags = %q, want the module preload kept", tags)
	}
}

func TestTagsKeepTheOrderWithinEachKind(t *testing.T) {
	tags := Tags([]Asset{
		{Kind: KindStyle, Path: "/a.css"},
		{Kind: KindStyle, Path: "/b.css"},
		{Kind: KindOther, Path: "/logo.svg"},
	})
	if strings.Index(tags, "/a.css") > strings.Index(tags, "/b.css") {
		t.Errorf("tags = %q, want the declared order kept", tags)
	}
	if strings.Contains(tags, "logo.svg") {
		t.Errorf("tags = %q, want other assets left out", tags)
	}
}
