package scaffold

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/config"
)

func create(t *testing.T, cfg Config) string {
	t.Helper()
	if cfg.Dir == "" {
		cfg.Dir = t.TempDir()
	}
	if cfg.Template == "" {
		cfg.Template = "hello-world"
	}
	if cfg.Module == "" {
		cfg.Module = "example.com/demo"
	}
	if err := Create(cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return cfg.Dir
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestNamesListsBundledTemplates(t *testing.T) {
	if !slices.Contains(Names(), "hello-world") {
		t.Errorf("Names() = %v, want hello-world", Names())
	}
}

func TestHasChecksTemplates(t *testing.T) {
	if !Has("hello-world") || Has("nope") {
		t.Error("Has reports the wrong templates")
	}
}

func TestDefaultNameUsesTheDirectory(t *testing.T) {
	cases := map[string]string{
		"demo":          "demo",
		"/tmp/my-site":  "my-site",
		"/tmp/my-site/": "my-site",
	}
	for dir, want := range cases {
		if got := DefaultName(dir); got != want {
			t.Errorf("DefaultName(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestCreateWritesTheWholeTree(t *testing.T) {
	dir := create(t, Config{Name: "demo"})
	for _, name := range []string{
		"go.mod", ".gitignore", "gopage.jsonc",
		"app/layout.gopage", "app/page.gopage", "app/api/health/route.go",
		"cmd/server/main.go", "cmd/worker/main.go", "public/favicon.ico",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestTemplateSuffixIsStripped(t *testing.T) {
	dir := create(t, Config{})
	if _, err := os.Stat(filepath.Join(dir, "cmd", "server", "main.go.tmpl")); err == nil {
		t.Error("a .tmpl file leaked into the generated project")
	}
}

func TestModulePathIsSubstituted(t *testing.T) {
	dir := create(t, Config{Module: "example.com/mysite"})
	if !strings.Contains(read(t, dir, "go.mod"), "module example.com/mysite") {
		t.Errorf("go.mod = %q", read(t, dir, "go.mod"))
	}
	if !strings.Contains(read(t, dir, "cmd/server/main.go"), `"example.com/mysite/internal/gen"`) {
		t.Errorf("main.go = %q", read(t, dir, "cmd/server/main.go"))
	}
}

func TestNameLandsInTheConfig(t *testing.T) {
	dir := create(t, Config{Name: "my-site"})
	if !strings.Contains(read(t, dir, "gopage.jsonc"), `"name": "my-site"`) {
		t.Errorf("gopage.toml = %q", read(t, dir, "gopage.jsonc"))
	}
}

func TestReplaceDirectiveIsOptional(t *testing.T) {
	withReplace := create(t, Config{GopagePath: "/src/gopage"})
	if !strings.Contains(read(t, withReplace, "go.mod"), "replace github.com/apptivitypl/gopage => /src/gopage") {
		t.Error("the replace directive is missing")
	}

	without := create(t, Config{})
	if strings.Contains(read(t, without, "go.mod"), "replace") {
		t.Error("a replace directive appeared without a path")
	}
}

func TestTheRequiredGopageVersionCanBePinned(t *testing.T) {
	pinned := create(t, Config{GopageVersion: "v0.4.2"})
	if !strings.Contains(read(t, pinned, "go.mod"), "github.com/apptivitypl/gopage v0.4.2") {
		t.Errorf("go.mod = %q", read(t, pinned, "go.mod"))
	}

	unpinned := create(t, Config{})
	if !strings.Contains(read(t, unpinned, "go.mod"), "github.com/apptivitypl/gopage "+DefaultGopageVersion) {
		t.Errorf("go.mod = %q, want the placeholder version", read(t, unpinned, "go.mod"))
	}
}

func TestTemplateSyntaxIsNotAppliedToGopageFiles(t *testing.T) {
	dir := create(t, Config{})
	layout := read(t, dir, "app/layout.gopage")
	if !strings.Contains(layout, "{% outlet %}") {
		t.Errorf("layout lost its directive: %q", layout)
	}
}

func TestCreateRefusesAnUnknownTemplate(t *testing.T) {
	err := Create(Config{Dir: t.TempDir(), Module: "example.com/x", Template: "nope"})
	if err == nil || !strings.Contains(err.Error(), "hello-world") {
		t.Errorf("err = %v, want it to list the available templates", err)
	}
}

func TestCreateRequiresAModulePath(t *testing.T) {
	err := Create(Config{Dir: t.TempDir(), Template: "hello-world"})
	if err == nil || !strings.Contains(err.Error(), "module path") {
		t.Errorf("err = %v", err)
	}
}

func TestCreateRefusesANonEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Create(Config{Dir: dir, Module: "example.com/x", Template: "hello-world"})
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("err = %v", err)
	}
}

func TestCreateMakesMissingDirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep")
	create(t, Config{Dir: dir})
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Errorf("nested directory was not created: %v", err)
	}
}

func TestTheAnswersReachTheConfigFile(t *testing.T) {
	dir := create(t, Config{
		Module:        "example.com/demo",
		Locales:       []string{"pl", "en"},
		DefaultLocale: "pl",
		Nav:           NavOff,
		CSS:           CSSTailwind,
	})
	settings := settingsOf(t, dir)
	if !slices.Equal(settings.I18n.Locales, []string{"pl", "en"}) || settings.I18n.DefaultLocale != "pl" {
		t.Errorf("i18n = %+v", settings.I18n)
	}
	if settings.CSS.Engine != config.EngineTailwind || settings.Nav.Mode != config.NavOff {
		t.Errorf("css = %q, nav = %q", settings.CSS.Engine, settings.Nav.Mode)
	}
}

func TestDefaultsFillTheGaps(t *testing.T) {
	dir := create(t, Config{Module: "example.com/demo"})
	settings := settingsOf(t, dir)
	if !slices.Equal(settings.I18n.Locales, []string{"en"}) {
		t.Errorf("locales = %v", settings.I18n.Locales)
	}
	if settings.CSS.Engine != config.EngineTailwind || settings.Nav.Mode != config.NavPartial {
		t.Errorf("css = %q, nav = %q", settings.CSS.Engine, settings.Nav.Mode)
	}
}

func settingsOf(t *testing.T, dir string) config.Config {
	t.Helper()
	settings, err := config.Parse(read(t, dir, "gopage.jsonc"))
	if err != nil {
		t.Fatalf("the generated config does not parse: %v", err)
	}
	return settings
}

func TestEveryTemplateWritesAConfigThatParses(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			dir := create(t, Config{Module: "example.com/demo", Template: name, Locales: []string{"en", "pl"}})
			settings := settingsOf(t, dir)
			if settings.Schema == "" {
				t.Error("the config must point at its schema")
			}
			if settings.App.Name == "" {
				t.Error("the config must name the application")
			}
		})
	}
}

func TestSplitLocales(t *testing.T) {
	cases := map[string][]string{
		"en, pl , de": {"en", "pl", "de"},
		"en":          {"en"},
		"":            nil,
		" , ":         nil,
	}
	for text, want := range cases {
		if got := SplitLocales(text); !slices.Equal(got, want) {
			t.Errorf("SplitLocales(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestAnEmptyTemplateFallsBackToTheDefault(t *testing.T) {
	dir := t.TempDir()
	if err := Create(Config{Dir: dir, Module: "example.com/demo"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "page.gopage")); err != nil {
		t.Errorf("stat = %v, want the default template written", err)
	}
	if !strings.Contains(read(t, dir, "gopage.jsonc"), `"name": "`+filepath.Base(dir)+`"`) {
		t.Errorf("gopage.toml = %q, want the directory as the name", read(t, dir, "gopage.jsonc"))
	}
}

func TestGopageTemplatesUseTheirOwnDelimiters(t *testing.T) {
	dir := create(t, Config{Module: "example.com/blog", Template: "blog"})
	page := read(t, dir, "app/page.gopage")
	if !strings.Contains(page, `"example.com/blog/content"`) {
		t.Errorf("page = %q, want the module substituted", page)
	}
	if !strings.Contains(page, "{{ Heading }}") {
		t.Errorf("page = %q, want the gopage interpolation left alone", page)
	}
}

func TestEveryTemplateIsListed(t *testing.T) {
	names := Names()
	if !slices.Contains(names, "hello-world") || !slices.Contains(names, "blog") {
		t.Errorf("names = %v", names)
	}
	for _, name := range names {
		if !Has(name) {
			t.Errorf("%s is listed but not available", name)
		}
	}
}

func TestTheBlogTemplateIsComplete(t *testing.T) {
	dir := create(t, Config{Module: "example.com/blog", Template: "blog"})
	for _, name := range []string{
		"app/page.gopage",
		"app/posts/[slug]/page.gopage",
		"app/feed.xml/route.go",
		"content/posts.go",
		"content/posts/hello.md",
		"components/PostCard/template.gopage",
		"locales/en.json",
		"gopage.jsonc",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s is missing: %v", name, err)
		}
	}
	if !strings.Contains(read(t, dir, "gopage.jsonc"), `"/feed.xml"`) {
		t.Error("the feed must be a reserved path so it keeps its own url")
	}
}

func TestTheThemeDecidesWhatIsGenerated(t *testing.T) {
	cases := map[string]struct {
		toggle bool
		forced string
	}{
		ThemeToggle: {toggle: true, forced: ""},
		ThemeSystem: {toggle: false, forced: ""},
		ThemeLight:  {toggle: false, forced: "light"},
		ThemeDark:   {toggle: false, forced: "dark"},
	}
	for theme, want := range cases {
		t.Run(theme, func(t *testing.T) {
			dir := create(t, Config{Theme: theme})
			_, err := os.Stat(filepath.Join(dir, "components", "ThemeToggle.gopage"))
			if want.toggle && err != nil {
				t.Errorf("the toggle island is missing: %v", err)
			}
			if !want.toggle && err == nil {
				t.Error("a project without the toggle must not carry its island")
			}
			layout := read(t, dir, "app/layout.gopage")
			markup := read(t, dir, "app/page.gopage")
			if want.forced != "" && !strings.Contains(layout, `data-theme="`+want.forced+`"`) {
				t.Errorf("layout = %q, want the theme forced", layout)
			}
			if want.forced == "" && strings.Contains(layout, "data-theme=") {
				t.Errorf("layout = %q, want the theme left to the browser", layout)
			}
			if want.toggle != strings.Contains(markup, "<ThemeToggle") {
				t.Errorf("layout = %q, toggle = %v", layout, want.toggle)
			}
		})
	}
}

func TestThemesAreListed(t *testing.T) {
	if got := Themes(); len(got) != 4 || got[0] != ThemeToggle {
		t.Errorf("Themes() = %v", got)
	}
}

func TestTheDefaultThemeCarriesTheToggle(t *testing.T) {
	dir := create(t, Config{})
	if _, err := os.Stat(filepath.Join(dir, "components", "ThemeToggle.gopage")); err != nil {
		t.Errorf("the default project should ship the toggle: %v", err)
	}
}

func TestTheStarterIsOnePageInOneLanguage(t *testing.T) {
	dir := create(t, Config{Module: "example.com/demo", Template: "hello-world"})
	for _, name := range []string{
		"app/page.gopage",
		"app/layout.gopage",
		"components/Ticker.gopage",
		"components/HackerNews.gopage",
		"locales/en.json",
		"public/llms.txt",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s is missing: %v", name, err)
		}
	}
	for _, name := range []string{"app/about/page.gopage", "locales/pl.json", "components/Search.gopage"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err == nil {
			t.Errorf("%s is still generated", name)
		}
	}
	page := read(t, dir, "app/page.gopage")
	if !strings.Contains(page, `class="ln"`) {
		t.Error("the source block carries line numbers")
	}
	if !strings.Contains(page, `class="icon"`) {
		t.Error("the badges carry icons")
	}
	if !strings.Contains(page, `data-pane="client-live"`) || !strings.Contains(page, "frame-url") {
		t.Error("the client component previews as a running component")
	}
	if !strings.Contains(page, `data-pane="server-live"`) || !strings.Contains(page, `Method="GET" Endpoint="/api/stories"`) {
		t.Error("the api route previews as the live response it returns")
	}
	if !strings.Contains(page, "hacker-news.firebaseio.com/v0/topstories.json") {
		t.Error("the server pane shows the upstream call itself, not a wrapper around it")
	}
	if !strings.Contains(page, "marked") || !strings.Contains(page, "by-logo") {
		t.Error("the hero carries the marker and the footer the maker")
	}
	hackerNews := read(t, dir, "components/HackerNews.gopage")
	if strings.Contains(hackerNews, "<script client") {
		t.Error("the news list is a server component: it ships no browser code of its own")
	}
	if !strings.Contains(hackerNews, "{% for story in Stories %}") {
		t.Error("the news list renders its rows on the server")
	}
	if !strings.Contains(hackerNews, `class="hacker-news-title"`) || !strings.Contains(hackerNews, `class="hacker-news-logo"`) {
		t.Error("the Hacker News component carries its own masthead")
	}
	if !strings.Contains(page, `{% fragment "HackerNews" defer="visible" %}`) || !strings.Contains(page, "{% placeholder %}") {
		t.Error("the list is fetched only once the slot is on screen, behind a server-rendered placeholder")
	}
	if !strings.Contains(page, "func HackerNews(ctx *gopage.Ctx) ([]Story, error)") {
		t.Error("the deferred fragment is fed by a loader named after it")
	}
	if !strings.Contains(page, "hackernews.Top(ctx.Request())") {
		t.Error("the loader does real server work instead of returning a canned list")
	}
	if !strings.Contains(read(t, dir, "server/hackernews/live.go"), "hacker-news.firebaseio.com") {
		t.Error("the shared package carries the upstream fetch")
	}
	if strings.Contains(read(t, dir, "app/api/stories/route.go"), "firebaseio") {
		t.Error("the api route reuses the shared package instead of its own copy")
	}
	if strings.Contains(page, "/api/health") || strings.Contains(page, `>/api/stories<`) {
		t.Error("the footer lists the crawlable files, not the api")
	}
	if !strings.Contains(page, `class="frame-url">{{ Host }}<`) {
		t.Error("the preview chrome shows the address the page was served from")
	}
	if !strings.Contains(read(t, dir, "package.json"), `"react": "^19`) {
		t.Error("a react starter declares react in package.json")
	}
	if !strings.Contains(read(t, dir, "gopage.jsonc"), `"client": {"react": "react"}`) {
		t.Error("gopage.toml names the browser engine")
	}
	for _, name := range []string{"client-code", "client-preview", "list-code", "list-preview", "server-code", "server-preview"} {
		if !strings.Contains(page, `:aria-label="t('pane.`+name+`')"`) {
			t.Errorf("the %s radio needs a name of its own: its label is hidden while another pane is open", name)
		}
	}
	if strings.Contains(read(t, dir, "styles/app.css"), "opacity: 0.45") {
		t.Error("dimming the line numbers drops them to 2.3:1 against the pane")
	}
	if !strings.Contains(page, `class="panel-pill"`) || !strings.Contains(page, `class="panel-mode"`) {
		t.Error("the panel has a sliding pill and a labelled code | preview control")
	}
	if !strings.Contains(page, `class="source-title mb-3"`) {
		t.Error("the source heading has a local type scale")
	}
	if strings.Contains(page, `id="news"`) || strings.Contains(page, `<Stories `) || !strings.Contains(page, "<HackerNews :Stories=") {
		t.Error("Hacker News appears only in the component preview")
	}
	if !strings.Contains(page, "<footer") || !strings.Contains(page, "foot-row") {
		t.Error("the page ends with a footer row")
	}
	ticker := read(t, dir, "components/Ticker.gopage")
	if !strings.Contains(ticker, `<script client lang="tsx">`) || !strings.Contains(ticker, "useState(Start)") {
		t.Error("the counter is a react component too")
	}
	for _, name := range []string{"components/Stars.gopage", "components/Response.gopage"} {
		if !strings.Contains(read(t, dir, name), "export default function") {
			t.Errorf("%s is a react component", name)
		}
	}
	if !strings.Contains(page, `<Stars client="idle"`) || !strings.Contains(page, `<Response client="visible"`) {
		t.Error("the header badge hydrates on idle and the response preview on sight")
	}
	toggle := read(t, dir, "components/ThemeToggle.gopage")
	if strings.Contains(toggle, "startViewTransition") || strings.Contains(toggle, "switch-bite") {
		t.Error("the theme change is a plain fade with a sun and a moon, no reveal and no mask")
	}
	if !strings.Contains(toggle, `role="switch"`) || strings.Contains(toggle, "<select") {
		t.Error("the theme control is a switch, not a select")
	}
	if !strings.Contains(toggle, "switch-knob") || !strings.Contains(toggle, "switch-moon") {
		t.Error("a switch reads as a knob that slides between a sun and a moon")
	}
	if strings.Contains(read(t, dir, "styles/app.css"), "switch-track") {
		t.Error("the switch is monochrome: ink knob on a plain track, no accent fill")
	}
	if !strings.Contains(read(t, dir, "styles/app.css"), ".stars {\n        height: 1.5rem;") {
		t.Error("the GitHub control shares the switch height")
	}
	if strings.Contains(toggle, "catch {}") {
		t.Error("an empty catch says nothing about what happens when storage is refused")
	}
}

func TestReactCanBeSwitchedOffOrSwappedForPreact(t *testing.T) {
	off := create(t, Config{Module: "example.com/demo", React: ReactOff})
	if _, err := os.Stat(filepath.Join(off, "package.json")); err == nil {
		t.Error("a starter without react has no package.json")
	}
	for _, name := range []string{"components/Ticker.gopage", "components/Stars.gopage", "components/Response.gopage"} {
		plain := read(t, off, name)
		if !strings.Contains(plain, "export function mount") || strings.Contains(plain, "lang=\"tsx\"") {
			t.Errorf("%s: without react every island is plain typescript", name)
		}
	}
	if strings.Contains(read(t, off, "gopage.jsonc"), `"client"`) {
		t.Error("without react there is no browser engine to name")
	}

	preact := create(t, Config{Module: "example.com/demo", React: ReactPreact})
	if !strings.Contains(read(t, preact, "package.json"), `"preact"`) {
		t.Error("the preact engine installs preact instead of react")
	}
	if !strings.Contains(read(t, preact, "gopage.jsonc"), `"react": "preact"`) {
		t.Error("gopage.toml names preact")
	}
	if got := Reacts(); len(got) != 3 || got[0] != ReactOn {
		t.Errorf("reacts = %v", got)
	}
}
