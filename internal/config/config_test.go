package config

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const twoHosts = `{
  "hosts": [
    {"pattern": "example.com", "locale": "en", "default": true},
    {"pattern": "pl.example.com", "locale": "pl"}
  ]
}`

func parse(t *testing.T, text string) Config {
	t.Helper()
	config, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return config
}

func parseErr(t *testing.T, text string) error {
	t.Helper()
	if _, err := Parse(text); err != nil {
		return err
	}
	t.Fatal("expected an error")
	return nil
}

func TestDefaults(t *testing.T) {
	config := parse(t, "")
	if config.I18n.Mode != ModePath {
		t.Errorf("mode = %q", config.I18n.Mode)
	}
	if config.I18n.DefaultLocale != "en" || !slices.Equal(config.I18n.Locales, []string{"en"}) {
		t.Errorf("i18n = %+v", config.I18n)
	}
	if !config.Reserves("/api") || !config.Reserves("/api/health") {
		t.Error("the api namespace is reserved by default")
	}
}

func TestLoadFallsBackToDefaults(t *testing.T) {
	config, err := Load(fstest.MapFS{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.I18n.Mode != ModePath {
		t.Errorf("mode = %q", config.I18n.Mode)
	}
}

func TestLoadReadsTheFile(t *testing.T) {
	config, err := Load(fstest.MapFS{FileName: &fstest.MapFile{Data: []byte(`{"app": {"name": "demo"}}`)}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.App.Name != "demo" {
		t.Errorf("name = %q", config.App.Name)
	}
}

func TestMalformedConfigIsReported(t *testing.T) {
	if err := parseErr(t, "{\"app\""); !strings.Contains(err.Error(), FileName) {
		t.Errorf("err = %v, want the file named", err)
	}
}

func TestDefaultLocaleIsAlwaysInTheList(t *testing.T) {
	config := parse(t, `{"i18n": {"defaultLocale": "pl", "locales": ["en", "de"]}}`)
	if config.I18n.Locales[0] != "pl" {
		t.Errorf("locales = %v, want the default first", config.I18n.Locales)
	}
}

func TestUnknownModeIsReported(t *testing.T) {
	err := parseErr(t, `{"i18n": {"mode": "domain"}}`)
	if !strings.Contains(err.Error(), "path, subdomain or single") {
		t.Errorf("err = %v", err)
	}
}

func TestLocaleCollidingWithAReservedPrefixIsReported(t *testing.T) {
	err := parseErr(t, `{"i18n": {"locales": ["en", "api"]}}`)
	if !strings.Contains(err.Error(), "collides") {
		t.Errorf("err = %v", err)
	}
}

func TestPrefixesSkipTheDefaultLocale(t *testing.T) {
	config := parse(t, `{"i18n": {"defaultLocale": "en", "locales": ["en", "pl", "de"]}}`)
	if got := config.Prefixes(); !slices.Equal(got, []string{"/pl", "/de"}) {
		t.Errorf("prefixes = %v", got)
	}
}

func TestPrefixDefaultAddsTheDefaultLocale(t *testing.T) {
	config := parse(t, `{"i18n": {"locales": ["en", "pl"], "prefixDefault": true}}`)
	if got := config.Prefixes(); !slices.Contains(got, "/en") {
		t.Errorf("prefixes = %v", got)
	}
}

func TestOtherModesHaveNoPathPrefixes(t *testing.T) {
	config := parse(t, `{"i18n": {"mode": "single", "locales": ["en", "pl"]}}`)
	if got := config.Prefixes(); got != nil {
		t.Errorf("prefixes = %v, want none", got)
	}
	if config.Localized() {
		t.Error("single mode is not path localized")
	}
}

func TestRedirectDefaultsToPermanent(t *testing.T) {
	config := parse(t, `{"redirects": [{"from": "/a", "to": "/b"}]}`)
	if config.Redirects[0].Status != 301 {
		t.Errorf("status = %d", config.Redirects[0].Status)
	}
}

func TestRedirectValidation(t *testing.T) {
	cases := map[string]string{
		"missing to":   `{"redirects": [{"from": "/a"}]}`,
		"missing from": `{"redirects": [{"to": "/b"}]}`,
		"wrong status": `{"redirects": [{"from": "/a", "to": "/b", "status": 200}]}`,
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			_ = parseErr(t, text)
		})
	}
}

func TestRewriteValidation(t *testing.T) {
	_ = parseErr(t, `{"rewrites": [{"from": "/a"}]}`)
	config := parse(t, `{"rewrites": [{"from": "/a", "to": "/b"}]}`)
	if config.Rewrites[0].To != "/b" {
		t.Errorf("rewrite = %+v", config.Rewrites[0])
	}
}

func TestSubdomainModeNeedsHosts(t *testing.T) {
	_ = parseErr(t, `{"i18n": {"mode": "subdomain"}}`)
	parse(t, `{"i18n": {"mode": "subdomain"}, "hosts": [{"pattern": "example.com", "locale": "en"}]}`)
}

func TestHostsAreLowercased(t *testing.T) {
	config := parse(t, `{"hosts": [{"pattern": "Example.COM", "locale": "en"}]}`)
	if config.Hosts[0].Pattern != "example.com" {
		t.Errorf("pattern = %q", config.Hosts[0].Pattern)
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"Example.COM":      "example.com",
		"example.com:8080": "example.com",
		"www.example.com":  "example.com",
		"  example.com  ":  "example.com",
		"[::1]:8080":       "[::1]",
	}
	for input, want := range cases {
		if got := NormalizeHost(input); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestKnownHost(t *testing.T) {
	config := parse(t, twoHosts)
	for _, host := range []string{"example.com", "EXAMPLE.com:443", "www.example.com", "pl.example.com"} {
		if !config.KnownHost(host) {
			t.Errorf("%q must be known", host)
		}
	}
	if config.KnownHost("evil.test") {
		t.Error("an unlisted host must be rejected")
	}
}

func TestEveryHostIsKnownWhenNoneAreListed(t *testing.T) {
	if !parse(t, "").KnownHost("anything.test") {
		t.Error("without a host list every host is accepted")
	}
}

func TestHostLocale(t *testing.T) {
	config := parse(t, twoHosts)

	if locale, matched := config.HostLocale("pl.example.com"); locale != "pl" || !matched {
		t.Errorf("locale = %q, matched = %v", locale, matched)
	}
	if locale, matched := config.HostLocale("unknown.test"); locale != "en" || matched {
		t.Errorf("fallback locale = %q, matched = %v", locale, matched)
	}
}

func TestHostLocaleWithoutHostsUsesTheDefault(t *testing.T) {
	config := parse(t, `{"i18n": {"defaultLocale": "de"}}`)
	if locale, matched := config.HostLocale("anything"); locale != "de" || !matched {
		t.Errorf("locale = %q, matched = %v", locale, matched)
	}
}

func TestReservedListCanBeReplaced(t *testing.T) {
	config := parse(t, `{"routing": {"reserved": ["/internal"]}}`)
	if !config.Reserves("/internal/x") {
		t.Error("the configured prefix is reserved")
	}
	if config.Reserves("/api") {
		t.Error("replacing the list drops the defaults")
	}
}

func TestNavDefaultsToOff(t *testing.T) {
	config := parse(t, "")
	if config.Nav.Mode != NavOff || config.Nav.Differential() {
		t.Errorf("nav = %+v", config.Nav)
	}
}

func TestNavCanBePartial(t *testing.T) {
	config := parse(t, `{"nav": {"mode": "partial"}}`)
	if !config.Nav.Differential() {
		t.Errorf("nav = %+v", config.Nav)
	}
}

func TestAnUnknownNavModeIsReported(t *testing.T) {
	if err := parseErr(t, `{"nav": {"mode": "turbo"}}`); !strings.Contains(err.Error(), "off or partial") {
		t.Errorf("err = %v", err)
	}
}

func TestTheFragmentStreamSettingIsRead(t *testing.T) {
	config, err := Parse(`{"app": {"name": "demo"}, "fragments": {"deferred": "tail"}}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if config.Fragments.Wait() != -1 {
		t.Errorf("wait = %v, want the tail mode never to wait", config.Fragments.Wait())
	}
	if (Fragments{Deferred: DeferredInline}).Wait() != 0 || (Fragments{}).Wait() != 0 {
		t.Error("only the tail mode ever gives up on a loader")
	}
}

func TestUnknownSettingsAreRejected(t *testing.T) {
	cases := map[string]string{
		"deferred": `{"fragments": {"deferred": "whenever"}}`,
		"engine":   `{"css": {"engine": "sass"}}`,
	}
	for name, text := range cases {
		if _, err := Parse(text); err == nil {
			t.Errorf("%s: an unknown value must be rejected", name)
		}
	}
}

func TestTheCssEngineIsRead(t *testing.T) {
	config, err := Parse(`{"css": {"engine": "tailwind"}}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if config.CSS.Engine != EngineTailwind {
		t.Errorf("engine = %q", config.CSS.Engine)
	}
	plain, err := Parse(`{"css": {"engine": "plain"}}`)
	if err != nil || plain.CSS.Engine != EnginePlain {
		t.Errorf("engine = %q, err = %v", plain.CSS.Engine, err)
	}
}

func TestAFragmentBudgetIsParsed(t *testing.T) {
	config, err := Parse(`{"app": {"name": "demo"}, "fragments": {"deferred": "tail", "budget": "25ms"}}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if config.Fragments.Wait() != 25*time.Millisecond {
		t.Errorf("wait = %v, want the budget from the file", config.Fragments.Wait())
	}
}

func TestABadFragmentBudgetIsRejected(t *testing.T) {
	cases := map[string]string{
		"not a duration": `{"fragments": {"deferred": "tail", "budget": "soon"}}`,
		"not positive":   `{"fragments": {"deferred": "tail", "budget": "0s"}}`,
		"negative":       `{"fragments": {"deferred": "tail", "budget": "-5ms"}}`,
		"inline":         `{"fragments": {"deferred": "inline", "budget": "25ms"}}`,
		"fetch":          `{"fragments": {"deferred": "fetch", "budget": "25ms"}}`,
	}
	for name, text := range cases {
		if _, err := Parse(text); err == nil {
			t.Errorf("%s: the budget must be rejected", name)
		}
	}
}

func TestAnUnparsableBudgetSendsEveryFragmentToTheTail(t *testing.T) {
	if got := (Fragments{Deferred: DeferredTail, Budget: "soon"}).Wait(); got != -1 {
		t.Errorf("wait = %v, want a budget that never validated to skip the wait", got)
	}
}

func TestFetchIsTheDefaultDeferredMode(t *testing.T) {
	config, err := Parse(`{"app": {"name": "demo"}}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !config.Fragments.Fetches() || !Default().Fragments.Fetches() {
		t.Errorf("deferred = %q, want a document that does not wait", config.Fragments.Deferred)
	}
}

func TestTheReactEngineIsReactOrPreact(t *testing.T) {
	for _, engine := range []string{"react", "preact"} {
		parsed, err := Parse(`{"client": {"react": "` + engine + `"}}`)
		if err != nil || parsed.Client.React != engine {
			t.Errorf("%s: react = %q, err = %v", engine, parsed.Client.React, err)
		}
	}
	if _, err := Parse(`{"client": {"react": "vue"}}`); err == nil || !strings.Contains(err.Error(), "react or preact") {
		t.Errorf("err = %v, want the engine refused", err)
	}
	empty, err := Parse("")
	if err != nil || empty.Client.React != "" {
		t.Errorf("react = %q, err = %v, want the engine left to the bundler's default", empty.Client.React, err)
	}
}

func TestFragmentStrategiesAreAClosedSet(t *testing.T) {
	for _, name := range Strategies() {
		if !KnownStrategy(name) {
			t.Errorf("%q is offered but not accepted", name)
		}
	}
	for _, name := range []string{"", "load", "media", "soon"} {
		if KnownStrategy(name) {
			t.Errorf("%q must not be a fragment strategy", name)
		}
	}
	if got := Strategies(); len(got) != 2 || got[0] != StrategyVisible {
		t.Errorf("strategies = %v", got)
	}
}

func TestCommentsAndTrailingCommasAreAccepted(t *testing.T) {
	config := parse(t, `{
  // the application, as the worker will be named
  "app": {"name": "demo"},
  /* one language, no prefix */
  "i18n": {
    "defaultLocale": "en",
    "locales": ["en",],
  },
}`)
	if config.App.Name != "demo" || config.I18n.DefaultLocale != "en" {
		t.Errorf("config = %+v", config)
	}
}

func TestTheSchemaKeyIsAccepted(t *testing.T) {
	config := parse(t, `{"$schema": "https://rill.dev/schema.json", "app": {"name": "demo"}}`)
	if config.Schema == "" || config.App.Name != "demo" {
		t.Errorf("config = %+v", config)
	}
}

func TestAMisspelledSettingIsReported(t *testing.T) {
	err := parseErr(t, `{"i18n": {"defaultLocal": "pl"}}`)
	if !strings.Contains(err.Error(), "defaultLocal") {
		t.Errorf("err = %v, want the unknown key named", err)
	}
}

func TestASyntaxErrorNamesTheLine(t *testing.T) {
	err := parseErr(t, "{\n  \"app\": {\"name\": \"demo\"},\n  \"nav\": {\"mode\": partial}\n}")
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("err = %v, want the line of the mistake", err)
	}
}

func TestATypeMismatchNamesTheLine(t *testing.T) {
	err := parseErr(t, "{\n  \"app\": {\"name\": 7}\n}")
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("err = %v, want the line of the mistake", err)
	}
}

func TestAnUnterminatedCommentIsReported(t *testing.T) {
	err := parseErr(t, "{\n  /* forever\n  \"app\": {}\n}")
	if !strings.Contains(err.Error(), "unterminated block comment") || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("err = %v", err)
	}
}

func TestWhitespaceOnlyConfigsFallBackToTheDefaults(t *testing.T) {
	for _, text := range []string{"", "  \n\t\n"} {
		config := parse(t, text)
		if config.I18n.Mode != ModePath {
			t.Errorf("%q: mode = %q", text, config.I18n.Mode)
		}
	}
}

func TestTheInlineLimitDefaultsToFourKilobytes(t *testing.T) {
	if got := parse(t, "").CSS.Inline(); got != DefaultInlineLimit {
		t.Errorf("limit = %d, want %d", got, DefaultInlineLimit)
	}
}

func TestTheInlineLimitAcceptsSizes(t *testing.T) {
	cases := map[string]int{
		`{"css": {"inlineLimit": "0"}}`:      0,
		`{"css": {"inlineLimit": "512b"}}`:   512,
		`{"css": {"inlineLimit": "1024"}}`:   1024,
		`{"css": {"inlineLimit": "4kb"}}`:    4 << 10,
		`{"css": {"inlineLimit": "  8KB "}}`: 8 << 10,
		`{"css": {"inlineLimit": "1mb"}}`:    1 << 20,
	}
	for text, want := range cases {
		if got := parse(t, text).CSS.Inline(); got != want {
			t.Errorf("%s: limit = %d, want %d", text, got, want)
		}
	}
}

func TestABadInlineLimitIsReportedByName(t *testing.T) {
	for _, text := range []string{
		`{"css": {"inlineLimit": "soon"}}`,
		`{"css": {"inlineLimit": "-1"}}`,
		`{"css": {"inlineLimit": "4 gigabytes"}}`,
	} {
		err := parseErr(t, text)
		if !strings.Contains(err.Error(), "css.inlineLimit") {
			t.Errorf("%s: err = %v, want the key named", text, err)
		}
	}
}

func TestAnUnparsableInlineLimitFallsBackToTheDefault(t *testing.T) {
	if got := (CSS{InlineLimit: "soon"}).Inline(); got != DefaultInlineLimit {
		t.Errorf("limit = %d, want the default when validation has been skipped", got)
	}
}

func TestTheSchemeIsHttpOrHttps(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		if got := parse(t, `{"app": {"scheme": "`+scheme+`"}}`).App.Scheme; got != scheme {
			t.Errorf("scheme = %q, want %q", got, scheme)
		}
	}
	err := parseErr(t, `{"app": {"scheme": "htp"}}`)
	if !strings.Contains(err.Error(), "http or https") {
		t.Errorf("err = %v, want the scheme refused", err)
	}
	if parse(t, "").App.Scheme != "" {
		t.Error("an absent scheme stays empty so the request decides")
	}
}

func TestTheBodyLimitDefaultsToEightMegabytes(t *testing.T) {
	if got := parse(t, "").Security.MaxBody(); got != DefaultMaxBodySize {
		t.Errorf("limit = %d, want %d", got, DefaultMaxBodySize)
	}
}

func TestTheBodyLimitAcceptsSizes(t *testing.T) {
	for text, want := range map[string]int64{
		`{"security": {"maxBodySize": "512b"}}`:  512,
		`{"security": {"maxBodySize": "256kb"}}`: 256 << 10,
		`{"security": {"maxBodySize": "2mb"}}`:   2 << 20,
	} {
		if got := parse(t, text).Security.MaxBody(); got != want {
			t.Errorf("%s: limit = %d, want %d", text, got, want)
		}
	}
}

func TestAnUnparsableBodyLimitFallsBackToTheDefault(t *testing.T) {
	if got := (Security{MaxBodySize: "soon"}).MaxBody(); got != DefaultMaxBodySize {
		t.Errorf("limit = %d, want the default", got)
	}
}

func TestABadBodyLimitIsReportedByName(t *testing.T) {
	for _, text := range []string{
		`{"security": {"maxBodySize": "soon"}}`,
		`{"security": {"maxBodySize": "-1"}}`,
		`{"security": {"maxBodySize": "0"}}`,
	} {
		err := parseErr(t, text)
		if err == nil || !strings.Contains(err.Error(), "security.maxBodySize") {
			t.Errorf("%s: err = %v, want the key named", text, err)
		}
	}
}

func TestANegativeConnectionLimitIsReportedByName(t *testing.T) {
	err := parseErr(t, `{"security": {"maxConnections": -1}}`)
	if err == nil || !strings.Contains(err.Error(), "security.maxConnections") {
		t.Errorf("err = %v, want the key named", err)
	}
}

func TestTheConnectionLimitDefaultsToNoLimit(t *testing.T) {
	if got := parse(t, "").Security.MaxConnections; got != 0 {
		t.Errorf("limit = %d, want no limit unless the project asks", got)
	}
	if got := parse(t, `{"security": {"maxConnections": 512}}`).Security.MaxConnections; got != 512 {
		t.Errorf("limit = %d, want 512", got)
	}
}
