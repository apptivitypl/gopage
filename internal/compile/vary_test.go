package compile

import (
	"testing"
	"testing/fstest"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/ir"
)

func varyOfRoute(t *testing.T, result Result, pattern string) []ir.Vary {
	t.Helper()
	for _, route := range result.Manifest.Routes {
		if route.Pattern == pattern {
			return route.Vary
		}
	}
	t.Fatalf("no route answers %s", pattern)
	return nil
}

func TestVaryReachesTheRouteFromThePage(t *testing.T) {
	result, bag := compiled(t, app(map[string]string{
		"app/page.gopage": `{% vary cookie="theme" values="light, dark" %}<p>home</p>`,
	}))
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	dimensions := varyOfRoute(t, result, "/")
	if len(dimensions) != 1 || dimensions[0].Name != "theme" || dimensions[0].Kind != ir.VaryCookie {
		t.Fatalf("vary = %+v", dimensions)
	}
	if got := dimensions[0].Bucket("dark"); got != "dark" {
		t.Errorf("bucket = %q", got)
	}
	if got := dimensions[0].Bucket("chartreuse"); got != "" {
		t.Errorf("bucket = %q, want the value outside the list to share one entry", got)
	}
}

func TestALayoutVariesEveryPageBelowIt(t *testing.T) {
	tree := app(map[string]string{
		"app/page.gopage":       "<p>home</p>",
		"app/about/page.gopage": `{% vary header="CF-IPCountry" values="PL, DE" %}<p>about</p>`,
	})
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte(
		`{% vary cookie="theme" values="light, dark" %}<main>{% outlet %}</main>`)}
	result, bag := compiled(t, tree)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	if got := varyOfRoute(t, result, "/"); len(got) != 1 || got[0].Name != "theme" {
		t.Errorf("home vary = %+v", got)
	}
	about := varyOfRoute(t, result, "/about")
	if len(about) != 2 || about[0].Name != "theme" || about[1].Name != "CF-IPCountry" {
		t.Errorf("about vary = %+v", about)
	}
}

func TestThePageWinsOverTheLayoutOnTheSameName(t *testing.T) {
	tree := app(map[string]string{
		"app/page.gopage": `{% vary cookie="theme" values="light, dark, sepia" %}<p>home</p>`,
	})
	tree["app/layout.gopage"] = &fstest.MapFile{Data: []byte(
		`{% vary cookie="theme" values="light" %}<main>{% outlet %}</main>`)}
	result, bag := compiled(t, tree)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	dimensions := varyOfRoute(t, result, "/")
	if len(dimensions) != 1 || len(dimensions[0].Values) != 3 {
		t.Errorf("vary = %+v, want the page to win", dimensions)
	}
}

func TestVaryInAComponentIsRejected(t *testing.T) {
	_, bag := compiled(t, app(map[string]string{
		"app/page.gopage":        "<Card />",
		"components/Card.gopage": `{% vary cookie="theme" values="light" %}<p>card</p>`,
	}))
	if _, ok := found(bag, diag.C114); !ok {
		t.Errorf("diagnostics = %+v, want C114", bag.Items())
	}
}

func TestVaryOnAPrivateCookieIsRejected(t *testing.T) {
	tree := app(map[string]string{
		"app/page.gopage": `{% vary cookie="session" values="in, out" %}<p>home</p>`,
	})
	tree["gopage.jsonc"] = &fstest.MapFile{Data: []byte(`{"security": {"privateCookies": ["session"]}}`)}
	_, bag := compiled(t, tree)
	if _, ok := found(bag, diag.C330); !ok {
		t.Errorf("diagnostics = %+v, want C330", bag.Items())
	}
}

func TestTooManyVariantsWarns(t *testing.T) {
	tree := app(map[string]string{
		"app/page.gopage": `{% vary cookie="theme" values="a, b, c" %}` +
			`{% vary header="X-Market" values="d, e, f" %}<p>home</p>`,
	})
	tree["gopage.jsonc"] = &fstest.MapFile{Data: []byte(`{"cache": {"variants": 8}}`)}
	_, bag := compiled(t, tree)
	item, ok := found(bag, diag.W706)
	if !ok {
		t.Fatalf("diagnostics = %+v, want W706", bag.Items())
	}
	if item.Severity != diag.Warning {
		t.Errorf("severity = %v, want a warning", item.Severity)
	}
	if bag.HasErrors() {
		t.Errorf("diagnostics = %+v, want the build to carry on", bag.Items())
	}
}
