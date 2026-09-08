package config

import (
	"strings"
	"testing"
	"time"
)

func TestTheSeoGeneratorsAreOnByDefault(t *testing.T) {
	config := parse(t, "")
	if !config.SEO.Sitemap.Enabled() || !config.SEO.Robots.Enabled() || !config.SEO.Sitemap.Probes() {
		t.Errorf("seo = %+v", config.SEO)
	}
	if config.SEO.Sitemap.Entries() != DefaultSitemapLimit {
		t.Errorf("limit = %d", config.SEO.Sitemap.Entries())
	}
	ttl, stale := config.SEO.Sitemap.Freshness()
	if ttl != DefaultSEOTTL || stale != DefaultSEOStale {
		t.Errorf("freshness = %v / %v", ttl, stale)
	}
}

func TestAMissingFileKeepsTheSeoDefaults(t *testing.T) {
	config := Default()
	if !config.SEO.Sitemap.Enabled() || !config.SEO.Sitemap.Probes() || config.SEO.Sitemap.Entries() != DefaultSitemapLimit {
		t.Errorf("seo = %+v", config.SEO)
	}
}

func TestEitherGeneratorCanBeTurnedOff(t *testing.T) {
	config := parse(t, `{"seo": {"sitemap": {"mode": "off"}, "robots": {"mode": "off"}}}`)
	if config.SEO.Sitemap.Enabled() || config.SEO.Robots.Enabled() {
		t.Errorf("seo = %+v", config.SEO)
	}
	if config.SEO.Sitemap.Probes() {
		t.Error("a sitemap that is off probes nothing")
	}
}

func TestTheProbeCanBeTurnedOffOnItsOwn(t *testing.T) {
	config := parse(t, `{"seo": {"sitemap": {"probe": "off"}}}`)
	if !config.SEO.Sitemap.Enabled() || config.SEO.Sitemap.Probes() {
		t.Errorf("seo = %+v", config.SEO)
	}
}

func TestFreshnessReadsDurations(t *testing.T) {
	config := parse(t, `{"seo": {"sitemap": {"ttl": "5m", "stale": "30m"}, "robots": {"ttl": "2h"}}}`)
	ttl, stale := config.SEO.Sitemap.Freshness()
	if ttl != 5*time.Minute || stale != 30*time.Minute {
		t.Errorf("sitemap freshness = %v / %v", ttl, stale)
	}
	ttl, stale = config.SEO.Robots.Freshness()
	if ttl != 2*time.Hour || stale != DefaultSEOStale {
		t.Errorf("robots freshness = %v / %v", ttl, stale)
	}
}

func TestAnOversizedLimitFallsBackToTheProtocolCeiling(t *testing.T) {
	sitemap := Sitemap{Limit: DefaultSitemapLimit + 1}
	if sitemap.Entries() != DefaultSitemapLimit {
		t.Errorf("entries = %d", sitemap.Entries())
	}
}

func TestABrokenFreshnessFallsBackToTheDefault(t *testing.T) {
	sitemap := Sitemap{TTL: "soon", Stale: "-1h"}
	ttl, stale := sitemap.Freshness()
	if ttl != DefaultSEOTTL || stale != DefaultSEOStale {
		t.Errorf("freshness = %v / %v", ttl, stale)
	}
}

func TestTheSeoSectionIsValidated(t *testing.T) {
	cases := map[string]string{
		`{"seo": {"sitemap": {"mode": "maybe"}}}`:                                    "seo.sitemap.mode",
		`{"seo": {"robots": {"mode": "maybe"}}}`:                                     "seo.robots.mode",
		`{"seo": {"sitemap": {"probe": "sometimes"}}}`:                               "seo.sitemap.probe",
		`{"seo": {"sitemap": {"limit": 50001}}}`:                                     "seo.sitemap.limit",
		`{"seo": {"sitemap": {"limit": -1}}}`:                                        "seo.sitemap.limit",
		`{"seo": {"sitemap": {"changefreq": "sometimes"}}}`:                          "seo.sitemap.changefreq",
		`{"seo": {"sitemap": {"priority": 1.5}}}`:                                    "seo.sitemap.priority",
		`{"seo": {"sitemap": {"ttl": "soon"}}}`:                                      "seo.sitemap.ttl",
		`{"seo": {"sitemap": {"stale": "-1h"}}}`:                                     "seo.sitemap.stale",
		`{"seo": {"robots": {"ttl": "soon"}}}`:                                       "seo.robots.ttl",
		`{"seo": {"sitemap": {"exclude": ["map"]}}}`:                                 "seo.sitemap.exclude",
		`{"seo": {"robots": {"groups": [{"userAgent": []}]}}}`:                       "userAgent",
		`{"seo": {"robots": {"groups": [{"userAgent": ["*"], "crawlDelay": -1}]}}}`:  "crawlDelay",
		`{"seo": {"robots": {"groups": [{"userAgent": ["*"], "allow": ["x"]}]}}}`:    "seo.robots allow",
		`{"seo": {"robots": {"groups": [{"userAgent": ["*"], "disallow": ["x"]}]}}}`: "seo.robots disallow",
		`{"seo": {"robots": {"sitemaps": ["hubs.xml"]}}}`:                            "seo.robots.sitemaps",
	}
	for text, want := range cases {
		err := parseErr(t, text)
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want it to name %s", text, err, want)
		}
	}
}

func TestAValidSeoSectionSurvives(t *testing.T) {
	config := parse(t, `{"seo": {
		"sitemap": {"limit": 100, "changefreq": "daily", "priority": 0.5, "exclude": ["/map"]},
		"robots": {"groups": [{"userAgent": ["*"], "allow": ["/"], "disallow": ["/api"], "crawlDelay": 5}],
		           "sitemaps": ["/sitemaps/hubs.xml", "https://cdn.example.com/s.xml"]}}}`)
	if config.SEO.Sitemap.Entries() != 100 || len(config.SEO.Robots.Groups) != 1 {
		t.Errorf("seo = %+v", config.SEO)
	}
}

func TestTheShardPrefixIsReserved(t *testing.T) {
	if !parse(t, "").Reserves("/sitemap/1.xml") {
		t.Error("shards live under a reserved prefix")
	}
}

func TestPrivateCookiesAreNamed(t *testing.T) {
	config := parse(t, `{"security": {"privateCookies": ["session", "refresh"]}}`)
	if !config.Security.Personal("session") || config.Security.Personal("theme") {
		t.Errorf("privateCookies = %v", config.Security.PrivateCookies)
	}
	if parse(t, "").Security.Personal("session") {
		t.Error("nothing is personal by default")
	}
	for _, text := range []string{
		`{"security": {"privateCookies": [""]}}`,
		`{"security": {"privateCookies": ["a=b"]}}`,
		`{"security": {"privateCookies": ["two words"]}}`,
	} {
		if err := parseErr(t, text); !strings.Contains(err.Error(), "security.privateCookies") {
			t.Errorf("%s: error = %v", text, err)
		}
	}
}

func TestAliasesAreCheckedAgainstTheLocales(t *testing.T) {
	cases := map[string]string{
		`{"routing": {"aliases": {"pl": {"jobs": "praca"}}}}`:                                                "not a configured locale",
		`{"i18n": {"locales": ["en", "pl"]}, "routing": {"aliases": {"pl": {"jobs": "pra/ca"}}}}`:            "not a path segment",
		`{"i18n": {"locales": ["en", "pl"]}, "routing": {"aliases": {"pl": {"": "praca"}}}}`:                 "not a path segment",
		`{"i18n": {"locales": ["en", "pl"]}, "routing": {"aliases": {"pl": {"jobs": "x", "news": "x"}}}}`:    "same segment",
		`{"i18n": {"locales": ["en", "pl"]}, "routing": {"aliases": {"pl": {"jobs": "news", "news": "x"}}}}`: "two meanings",
	}
	for text, want := range cases {
		if err := parseErr(t, text); !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want %q", text, err, want)
		}
	}
}

func TestAValidAliasTableSurvives(t *testing.T) {
	config := parse(t, `{
		"i18n": {"locales": ["en", "pl", "de"]},
		"routing": {"aliases": {"pl": {"jobs": "praca"}, "de": {"jobs": "arbeit"}}}
	}`)
	if config.Routing.Aliases["pl"]["jobs"] != "praca" || config.Routing.Aliases["de"]["jobs"] != "arbeit" {
		t.Errorf("aliases = %v", config.Routing.Aliases)
	}
	if parse(t, "").Routing.Aliases != nil {
		t.Error("no aliases by default")
	}
}
