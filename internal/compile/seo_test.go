package compile

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
)

func seoBag(t *testing.T, files map[string]string) *diag.Bag {
	t.Helper()
	var bag diag.Bag
	if _, err := Compile(app(files), &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return &bag
}

func found(bag *diag.Bag, code diag.Code) (diag.Diagnostic, bool) {
	for _, item := range bag.Items() {
		if item.Code == code {
			return item, true
		}
	}
	return diag.Diagnostic{}, false
}

func TestARouteOnTheBuiltinSitemapIsRejected(t *testing.T) {
	bag := seoBag(t, map[string]string{
		"app/page.gopage": "<p>home</p>",
		"app/sitemap.xml/route.go": "package sitemap\n\nimport \"github.com/apptivitypl/gopage\"\n\n" +
			"func GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) { return nil, nil }\n",
	})
	item, ok := found(bag, diag.C112)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Help, `"sitemap": {"mode": "off"}`) {
		t.Errorf("help = %q", item.Help)
	}
}

func TestAPublicRobotsFileIsRejected(t *testing.T) {
	files := app(map[string]string{"app/page.gopage": "<p>home</p>"})
	files["public/robots.txt"] = &fstest.MapFile{Data: []byte("User-agent: *\n")}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	item, ok := found(&bag, diag.C112)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Help, `"robots": {"mode": "off"}`) {
		t.Errorf("help = %q", item.Help)
	}
}

func TestATurnedOffGeneratorLeavesThePathToTheProject(t *testing.T) {
	files := app(map[string]string{
		"app/page.gopage": "<p>home</p>",
		"app/sitemap.xml/route.go": "package sitemap\n\nimport \"github.com/apptivitypl/gopage\"\n\n" +
			"func GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) { return nil, nil }\n",
	})
	files["gopage.jsonc"] = &fstest.MapFile{Data: []byte(`{"seo": {"sitemap": {"mode": "off"}}}`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, ok := found(&bag, diag.C112); ok {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}

func TestASeoRuleThatNamesNoRouteWarns(t *testing.T) {
	files := app(map[string]string{"app/page.gopage": "<p>home</p>", "app/about/page.gopage": "<p>about</p>"})
	files["gopage.jsonc"] = &fstest.MapFile{Data: []byte(`{"seo": {
		"sitemap": {"exclude": ["/ghost"]},
		"robots": {"groups": [{"userAgent": ["*"], "allow": ["/"], "disallow": ["/about", "/nowhere"]}]}}}`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var named []string
	for _, item := range bag.Items() {
		if item.Code == diag.W704 {
			named = append(named, item.Message)
		}
	}
	if len(named) != 2 {
		t.Fatalf("warnings = %v, want the two unserved rules", named)
	}
	if bag.HasErrors() {
		t.Errorf("a warning is not an error: %+v", bag.Items())
	}
}

func TestSeoRulesAcceptLocalePrefixesReservedPathsAndWildcards(t *testing.T) {
	files := app(map[string]string{"app/page.gopage": "<p>home</p>", "app/about/page.gopage": "<p>about</p>"})
	files["gopage.jsonc"] = &fstest.MapFile{Data: []byte(`{"i18n": {"locales": ["en", "pl"]}, "seo": {
		"sitemap": {"exclude": ["/pl/about", "/pl"]},
		"robots": {"groups": [{"userAgent": ["*"], "disallow": ["/api", "/assets/x", "/*.pdf"]}]}}}`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, ok := found(&bag, diag.W704); ok {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}

func TestASitemapHookIsRecognised(t *testing.T) {
	source := "---\ntype Props struct{}\n\n" +
		"func Load(ctx *gopage.Ctx) (Props, error) { return Props{}, nil }\n\n" +
		"func Sitemap(ctx *gopage.Ctx) (gopage.SitemapSeq, error) { return nil, nil }\n---\n<p>home</p>"
	var bag diag.Bag
	result, err := Compile(app(map[string]string{"app/page.gopage": source}), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	template := result.Templates["app/page.gopage"]
	if !template.HasSitemap() || !template.Sitemap(&bag) {
		t.Errorf("template = %+v", template)
	}
	if bag.HasErrors() {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}

func TestASitemapHookWithTheWrongSignatureIsRejected(t *testing.T) {
	source := "---\ntype Props struct{}\n\n" +
		"func Sitemap(ctx *gopage.Ctx, params gopage.Params) (gopage.SitemapSeq, error) { return nil, nil }\n---\n<p>home</p>"
	var bag diag.Bag
	result, err := Compile(app(map[string]string{"app/page.gopage": source}), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.Templates["app/page.gopage"].Sitemap(&bag) {
		t.Error("the hook was accepted")
	}
	item, ok := found(&bag, diag.C325)
	if !ok {
		t.Fatalf("diagnostics = %+v", bag.Items())
	}
	if !strings.Contains(item.Help, "gopage.SitemapSeq") {
		t.Errorf("help = %q", item.Help)
	}
}

func TestATemplateWithoutASitemapHookReportsNothing(t *testing.T) {
	var bag diag.Bag
	result, err := Compile(app(map[string]string{"app/page.gopage": "<p>home</p>"}), &bag)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.Templates["app/page.gopage"].Sitemap(&bag) || bag.HasErrors() {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
	if SitemapSignature("func Sitemap(", "app/page.gopage", &bag) {
		t.Error("unparsable frontmatter declares nothing")
	}
}

func TestLinksMaySpeakTheLocalisedVocabulary(t *testing.T) {
	files := app(map[string]string{
		"app/page.gopage":          `<a href="/pl/oferty">oferty</a><a href="/pl">home</a><a href="/listings">canonical</a>`,
		"app/listings/page.gopage": "<p>list</p>",
	})
	files["gopage.jsonc"] = &fstest.MapFile{Data: []byte(`{
		"i18n": {"locales": ["en", "pl"]},
		"routing": {"aliases": {"pl": {"listings": "oferty"}}}
	}`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, ok := found(&bag, diag.C111); ok {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}

func TestALinkThatNoLocaleAnswersIsStillReported(t *testing.T) {
	files := app(map[string]string{"app/page.gopage": `<a href="/pl/ghost">ghost</a>`})
	files["gopage.jsonc"] = &fstest.MapFile{Data: []byte(`{"i18n": {"locales": ["en", "pl"]}}`)}
	var bag diag.Bag
	if _, err := Compile(files, &bag); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, ok := found(&bag, diag.C111); !ok {
		t.Errorf("diagnostics = %+v", bag.Items())
	}
}
