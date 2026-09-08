package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/apptivitypl/gopage/internal/jsonc"
	"github.com/apptivitypl/gopage/internal/paths"
)

const FileName = paths.Config

type Mode string

const (
	ModePath      Mode = "path"
	ModeSubdomain Mode = "subdomain"
	ModeSingle    Mode = "single"
)

type App struct {
	Name          string `json:"name,omitempty"`
	Scheme        string `json:"scheme,omitempty"`
	CanonicalHost string `json:"canonicalHost,omitempty"`
}

type Security struct {
	TrustedProxy   bool     `json:"trustedProxy,omitempty"`
	MaxBodySize    string   `json:"maxBodySize,omitempty"`
	TrustedOrigins []string `json:"trustedOrigins,omitempty"`
	MaxConnections int      `json:"maxConnections,omitempty"`
	PrivateCookies []string `json:"privateCookies,omitempty"`
}

func (s Security) Personal(name string) bool {
	return slices.Contains(s.PrivateCookies, name)
}

const (
	DefaultMaxBodySize = 8 << 20
	SchemeHTTP         = "http"
	SchemeHTTPS        = "https"
)

func (s Security) MaxBody() int64 {
	if s.MaxBodySize == "" {
		return DefaultMaxBodySize
	}
	limit, err := parseSize(s.MaxBodySize)
	if err != nil {
		return DefaultMaxBodySize
	}
	return int64(limit)
}

type I18n struct {
	Mode          Mode     `json:"mode,omitempty"`
	DefaultLocale string   `json:"defaultLocale,omitempty"`
	Locales       []string `json:"locales,omitempty"`
	PrefixDefault bool     `json:"prefixDefault,omitempty"`
	Timezone      string   `json:"timezone,omitempty"`
}

func (i I18n) Zone() *time.Location {
	if i.Timezone == "" {
		return time.UTC
	}
	zone, err := time.LoadLocation(i.Timezone)
	if err != nil {
		return time.UTC
	}
	return zone
}

type NavMode string

const (
	NavOff     NavMode = "off"
	NavPartial NavMode = "partial"
)

type Nav struct {
	Mode NavMode `json:"mode,omitempty"`
}

func (n Nav) Differential() bool {
	return n.Mode == NavPartial
}

type Routing struct {
	Reserved  []string                     `json:"reserved,omitempty"`
	Aliases   map[string]map[string]string `json:"aliases,omitempty"`
	Normalize Normalize                    `json:"normalize,omitempty"`
}

type Normalize struct {
	TrailingSlash string `json:"trailingSlash,omitempty"`
	Case          string `json:"case,omitempty"`
	Diacritics    string `json:"diacritics,omitempty"`
}

const (
	SlashStrip   = "strip"
	SlashKeep    = "keep"
	CaseLower    = "lower"
	FoldMarks    = "fold"
	NormalizeOff = "off"
)

func (n Normalize) Strips() bool {
	return n.TrailingSlash == SlashStrip
}

func (n Normalize) Keeps() bool {
	return n.TrailingSlash == SlashKeep
}

func (n Normalize) Lowers() bool {
	return n.Case == CaseLower
}

func (n Normalize) Folds() bool {
	return n.Diacritics == FoldMarks
}

func (n Normalize) Any() bool {
	return n.Strips() || n.Keeps() || n.Lowers() || n.Folds()
}

type Host struct {
	Pattern string `json:"pattern,omitempty"`
	Locale  string `json:"locale,omitempty"`
	Default bool   `json:"default,omitempty"`
}

type Redirect struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Status int    `json:"status,omitempty"`
}

type Rewrite struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type Fragments struct {
	Deferred string `json:"deferred,omitempty"`
	Budget   string `json:"budget,omitempty"`
}

type CSS struct {
	Engine      string `json:"engine,omitempty"`
	InlineLimit string `json:"inlineLimit,omitempty"`
}

const DefaultInlineLimit = 4 << 10

func (c CSS) Inline() int {
	if c.InlineLimit == "" {
		return DefaultInlineLimit
	}
	limit, err := parseSize(c.InlineLimit)
	if err != nil {
		return DefaultInlineLimit
	}
	return limit
}

func parseSize(text string) (int, error) {
	digits := strings.ToLower(strings.TrimSpace(text))
	unit := 1
	switch {
	case strings.HasSuffix(digits, "kb"):
		unit, digits = 1<<10, strings.TrimSuffix(digits, "kb")
	case strings.HasSuffix(digits, "mb"):
		unit, digits = 1<<20, strings.TrimSuffix(digits, "mb")
	case strings.HasSuffix(digits, "b"):
		digits = strings.TrimSuffix(digits, "b")
	}
	value, err := strconv.Atoi(strings.TrimSpace(digits))
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%q is not a byte size", text)
	}
	return value * unit, nil
}

type Client struct {
	React string `json:"react,omitempty"`
}

type Cache struct {
	Variants int `json:"variants,omitempty"`
}

const DefaultVariants = 16

const (
	ReactEngine  = "react"
	PreactEngine = "preact"
)

var reactEngines = []string{ReactEngine, PreactEngine}

const (
	EnginePlain    = "plain"
	EngineTailwind = "tailwind"
)

const (
	DeferredInline = "inline"
	DeferredTail   = "tail"
	DeferredFetch  = "fetch"
)

var deferredModes = []string{DeferredInline, DeferredTail, DeferredFetch}

const (
	StrategyVisible = "visible"
	StrategyIdle    = "idle"
)

func Strategies() []string {
	return []string{StrategyVisible, StrategyIdle}
}

func KnownStrategy(name string) bool {
	return name == StrategyVisible || name == StrategyIdle
}

func (f Fragments) Fetches() bool {
	return f.Deferred == DeferredFetch
}

func (f Fragments) Wait() time.Duration {
	if f.Deferred != DeferredTail {
		return 0
	}
	budget, err := time.ParseDuration(f.Budget)
	if f.Budget == "" || err != nil || budget <= 0 {
		return -1
	}
	return budget
}

type SEOMode string

const (
	SEOAuto SEOMode = "auto"
	SEOOff  SEOMode = "off"
)

type ProbeMode string

const (
	ProbeMeta ProbeMode = "meta"
	ProbeOff  ProbeMode = "off"
)

const (
	DefaultSitemapLimit = 50000
	DefaultSEOTTL       = time.Hour
	DefaultSEOStale     = 24 * time.Hour
)

var changeFrequencies = []string{"always", "hourly", "daily", "weekly", "monthly", "yearly", "never"}

type Sitemap struct {
	Mode       SEOMode   `json:"mode,omitempty"`
	Limit      int       `json:"limit,omitempty"`
	Probe      ProbeMode `json:"probe,omitempty"`
	TTL        string    `json:"ttl,omitempty"`
	Stale      string    `json:"stale,omitempty"`
	ChangeFreq string    `json:"changefreq,omitempty"`
	Priority   float64   `json:"priority,omitempty"`
	Exclude    []string  `json:"exclude,omitempty"`
}

func (s Sitemap) Enabled() bool {
	return s.Mode != SEOOff
}

func (s Sitemap) Probes() bool {
	return s.Enabled() && s.Probe != ProbeOff
}

func (s Sitemap) Entries() int {
	if s.Limit <= 0 || s.Limit > DefaultSitemapLimit {
		return DefaultSitemapLimit
	}
	return s.Limit
}

func (s Sitemap) Freshness() (time.Duration, time.Duration) {
	return freshness(s.TTL, s.Stale)
}

type RobotsGroup struct {
	UserAgent  []string `json:"userAgent,omitempty"`
	Allow      []string `json:"allow,omitempty"`
	Disallow   []string `json:"disallow,omitempty"`
	CrawlDelay int      `json:"crawlDelay,omitempty"`
}

type Robots struct {
	Mode     SEOMode       `json:"mode,omitempty"`
	TTL      string        `json:"ttl,omitempty"`
	Stale    string        `json:"stale,omitempty"`
	Groups   []RobotsGroup `json:"groups,omitempty"`
	Sitemaps []string      `json:"sitemaps,omitempty"`
}

func (r Robots) Enabled() bool {
	return r.Mode != SEOOff
}

func (r Robots) Freshness() (time.Duration, time.Duration) {
	return freshness(r.TTL, r.Stale)
}

type ImageMode string

const (
	ImagesOn  ImageMode = "on"
	ImagesOff ImageMode = "off"
)

type Images struct {
	Mode    ImageMode `json:"mode,omitempty"`
	Widths  []int     `json:"widths,omitempty"`
	Quality int       `json:"quality,omitempty"`
	Formats []string  `json:"formats,omitempty"`
	Hosts   []string  `json:"hosts,omitempty"`
	TTL     string    `json:"ttl,omitempty"`
}

var defaultWidths = []int{320, 640, 960, 1280, 1920}

func (i Images) Enabled() bool {
	return i.Mode == ImagesOn
}

func (i Images) Sizes() []int {
	if len(i.Widths) == 0 {
		return defaultWidths
	}
	return i.Widths
}

func (i Images) Sharpness() int {
	if i.Quality <= 0 || i.Quality > 100 {
		return DefaultImageQuality
	}
	return i.Quality
}

func (i Images) Freshness() (time.Duration, time.Duration) {
	return freshness(i.TTL, "")
}

func (i Images) Serves(host string) bool {
	return slices.Contains(i.Hosts, host)
}

const DefaultImageQuality = 75

type SEO struct {
	Sitemap Sitemap `json:"sitemap,omitempty"`
	Robots  Robots  `json:"robots,omitempty"`
}

func freshness(ttl, stale string) (time.Duration, time.Duration) {
	fresh, err := time.ParseDuration(ttl)
	if ttl == "" || err != nil || fresh < 0 {
		fresh = DefaultSEOTTL
	}
	held, err := time.ParseDuration(stale)
	if stale == "" || err != nil || held < 0 {
		held = DefaultSEOStale
	}
	return fresh, held
}

type Config struct {
	Schema    string     `json:"$schema,omitempty"`
	App       App        `json:"app,omitempty"`
	CSS       CSS        `json:"css,omitempty"`
	Fragments Fragments  `json:"fragments,omitempty"`
	I18n      I18n       `json:"i18n,omitempty"`
	Routing   Routing    `json:"routing,omitempty"`
	Nav       Nav        `json:"nav,omitempty"`
	SEO       SEO        `json:"seo,omitempty"`
	Images    Images     `json:"images,omitempty"`
	Hosts     []Host     `json:"hosts,omitempty"`
	Security  Security   `json:"security,omitempty"`
	Client    Client     `json:"client,omitempty"`
	Cache     Cache      `json:"cache,omitempty"`
	Redirects []Redirect `json:"redirects,omitempty"`
	Rewrites  []Rewrite  `json:"rewrites,omitempty"`
}

var defaultReserved = []string{"/api", "/_gopage", "/robots.txt", "/sitemap.xml", "/sitemap", "/favicon.ico"}

func Default() Config {
	return Config{
		I18n: I18n{
			Mode:          ModePath,
			DefaultLocale: "en",
			Locales:       []string{"en"},
		},
		Routing:   Routing{Reserved: slices.Clone(defaultReserved)},
		Cache:     Cache{Variants: DefaultVariants},
		Fragments: Fragments{Deferred: DeferredFetch},
		SEO: SEO{
			Sitemap: Sitemap{Mode: SEOAuto, Limit: DefaultSitemapLimit, Probe: ProbeMeta},
			Robots:  Robots{Mode: SEOAuto},
		},
	}
}

func Load(fsys fs.FS) (Config, error) {
	data, err := fs.ReadFile(fsys, FileName)
	if err != nil {
		return Default(), nil
	}
	return Parse(string(data))
}

func Parse(text string) (Config, error) {
	source := []byte(text)
	if strings.TrimSpace(text) == "" {
		config := Default()
		normalize(&config)
		return config, validate(config)
	}
	plain, err := jsonc.ToJSON(source)
	if err != nil {
		return Config{}, describe(source, err)
	}
	config := Default()
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, describe(source, err)
	}
	normalize(&config)
	return config, validate(config)
}

func describe(source []byte, err error) error {
	var offset int
	var syntax *json.SyntaxError
	var mismatch *json.UnmarshalTypeError
	var raw *jsonc.SyntaxError
	switch {
	case errors.As(err, &syntax):
		offset = int(syntax.Offset)
	case errors.As(err, &mismatch):
		offset = int(mismatch.Offset)
	case errors.As(err, &raw):
		offset = raw.Offset
	default:
		return fmt.Errorf("%s: %w", FileName, err)
	}
	return fmt.Errorf("%s: %w", FileName, jsonc.Locate(source, offset, err))
}

func normalize(config *Config) {
	if config.Cache.Variants == 0 {
		config.Cache.Variants = DefaultVariants
	}
	if config.Fragments.Deferred == "" {
		config.Fragments.Deferred = DeferredFetch
	}
	if config.I18n.Mode == "" {
		config.I18n.Mode = ModePath
	}
	if config.I18n.DefaultLocale == "" {
		config.I18n.DefaultLocale = "en"
	}
	if len(config.I18n.Locales) == 0 {
		config.I18n.Locales = []string{config.I18n.DefaultLocale}
	}
	if !slices.Contains(config.I18n.Locales, config.I18n.DefaultLocale) {
		config.I18n.Locales = append([]string{config.I18n.DefaultLocale}, config.I18n.Locales...)
	}
	if config.Nav.Mode == "" {
		config.Nav.Mode = NavOff
	}
	if config.SEO.Sitemap.Mode == "" {
		config.SEO.Sitemap.Mode = SEOAuto
	}
	if config.SEO.Sitemap.Probe == "" {
		config.SEO.Sitemap.Probe = ProbeMeta
	}
	if config.SEO.Sitemap.Limit == 0 {
		config.SEO.Sitemap.Limit = DefaultSitemapLimit
	}
	if config.SEO.Robots.Mode == "" {
		config.SEO.Robots.Mode = SEOAuto
	}
	if len(config.Routing.Reserved) == 0 {
		config.Routing.Reserved = slices.Clone(defaultReserved)
	}
	for i := range config.Redirects {
		if config.Redirects[i].Status == 0 {
			config.Redirects[i].Status = http.StatusMovedPermanently
		}
	}
	for i := range config.Hosts {
		config.Hosts[i].Pattern = NormalizeHost(config.Hosts[i].Pattern)
	}
}

func validateFragments(fragments Fragments) error {
	if fragments.Deferred != "" && !slices.Contains(deferredModes, fragments.Deferred) {
		return fmt.Errorf("%s: unknown deferred mode %q, want inline, tail or fetch",
			FileName, fragments.Deferred)
	}
	if fragments.Budget == "" {
		return nil
	}
	if fragments.Deferred != DeferredTail {
		return fmt.Errorf("%s: a fragment budget only applies to deferred %q, "+
			"because no other mode has a moment to wait in", FileName, DeferredTail)
	}
	budget, err := time.ParseDuration(fragments.Budget)
	if err != nil {
		return fmt.Errorf("%s: fragment budget %q is not a duration, want something like 25ms",
			FileName, fragments.Budget)
	}
	if budget <= 0 {
		return fmt.Errorf("%s: fragment budget %q must be positive, "+
			"drop the key to send every fragment in the tail", FileName, fragments.Budget)
	}
	return nil
}

func validateCSS(css CSS) error {
	if css.InlineLimit == "" {
		return nil
	}
	if _, err := parseSize(css.InlineLimit); err != nil {
		return fmt.Errorf("%s: css.inlineLimit %w, want a size like \"4kb\", \"512b\" or \"0\" to link every stylesheet",
			FileName, err)
	}
	return nil
}

func validateSecurity(security Security) error {
	if security.MaxConnections < 0 {
		return fmt.Errorf("%s: security.maxConnections %d must not be negative, drop the key for no limit",
			FileName, security.MaxConnections)
	}
	for _, name := range security.PrivateCookies {
		if name == "" || strings.ContainsAny(name, " \t;=,") {
			return fmt.Errorf("%s: security.privateCookies entry %q is not a cookie name", FileName, name)
		}
	}
	if security.MaxBodySize == "" {
		return nil
	}
	limit, err := parseSize(security.MaxBodySize)
	if err != nil {
		return fmt.Errorf("%s: security.maxBodySize %w, want a size like \"8mb\" or \"512kb\"", FileName, err)
	}
	if limit == 0 {
		return fmt.Errorf("%s: security.maxBodySize %q would refuse every submission, drop the key for the default",
			FileName, security.MaxBodySize)
	}
	return nil
}

func validateSEO(seo SEO) error {
	if err := validateSitemap(seo.Sitemap); err != nil {
		return err
	}
	return validateRobots(seo.Robots)
}

func validateSitemap(sitemap Sitemap) error {
	if err := seoMode(sitemap.Mode, "seo.sitemap.mode"); err != nil {
		return err
	}
	switch sitemap.Probe {
	case "", ProbeMeta, ProbeOff:
	default:
		return fmt.Errorf("%s: unknown seo.sitemap.probe %q, want meta or off", FileName, sitemap.Probe)
	}
	if sitemap.Limit < 0 || sitemap.Limit > DefaultSitemapLimit {
		return fmt.Errorf("%s: seo.sitemap.limit %d must be between 1 and %d, the ceiling the sitemap protocol sets",
			FileName, sitemap.Limit, DefaultSitemapLimit)
	}
	if sitemap.ChangeFreq != "" && !slices.Contains(changeFrequencies, sitemap.ChangeFreq) {
		return fmt.Errorf("%s: unknown seo.sitemap.changefreq %q, want one of %s",
			FileName, sitemap.ChangeFreq, strings.Join(changeFrequencies, ", "))
	}
	if sitemap.Priority < 0 || sitemap.Priority > 1 {
		return fmt.Errorf("%s: seo.sitemap.priority %v must be between 0 and 1", FileName, sitemap.Priority)
	}
	if err := freshnessFields(sitemap.TTL, sitemap.Stale, "seo.sitemap"); err != nil {
		return err
	}
	return absolutePaths(sitemap.Exclude, "seo.sitemap.exclude")
}

func validateRobots(robots Robots) error {
	if err := seoMode(robots.Mode, "seo.robots.mode"); err != nil {
		return err
	}
	if err := freshnessFields(robots.TTL, robots.Stale, "seo.robots"); err != nil {
		return err
	}
	for _, group := range robots.Groups {
		if len(group.UserAgent) == 0 {
			return fmt.Errorf("%s: a robots group needs at least one userAgent, write [\"*\"] for every crawler", FileName)
		}
		if group.CrawlDelay < 0 {
			return fmt.Errorf("%s: seo.robots crawlDelay %d must not be negative", FileName, group.CrawlDelay)
		}
		if err := absolutePaths(group.Allow, "seo.robots allow"); err != nil {
			return err
		}
		if err := absolutePaths(group.Disallow, "seo.robots disallow"); err != nil {
			return err
		}
	}
	for _, entry := range robots.Sitemaps {
		if strings.HasPrefix(entry, "/") || strings.HasPrefix(entry, "http://") || strings.HasPrefix(entry, "https://") {
			continue
		}
		return fmt.Errorf("%s: seo.robots.sitemaps entry %q must start with / or with a scheme", FileName, entry)
	}
	return nil
}

func seoMode(mode SEOMode, field string) error {
	switch mode {
	case "", SEOAuto, SEOOff:
		return nil
	default:
		return fmt.Errorf("%s: unknown %s %q, want auto or off", FileName, field, mode)
	}
}

func freshnessFields(ttl, stale, field string) error {
	for name, value := range map[string]string{"ttl": ttl, "stale": stale} {
		if value == "" {
			continue
		}
		span, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s: %s.%s %q is not a duration, want something like \"1h\"", FileName, field, name, value)
		}
		if span < 0 {
			return fmt.Errorf("%s: %s.%s %q must not be negative", FileName, field, name, value)
		}
	}
	return nil
}

func absolutePaths(paths []string, field string) error {
	for _, path := range paths {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("%s: %s entry %q must start with /", FileName, field, path)
		}
	}
	return nil
}

func validateAliases(config Config) error {
	for locale, aliases := range config.Routing.Aliases {
		if !slices.Contains(config.I18n.Locales, locale) {
			return fmt.Errorf("%s: routing.aliases names %q, which is not a configured locale", FileName, locale)
		}
		taken := make(map[string]string, len(aliases))
		for canonical, public := range aliases {
			if err := segment(canonical, locale); err != nil {
				return err
			}
			if err := segment(public, locale); err != nil {
				return err
			}
			if other, clash := taken[public]; clash {
				return fmt.Errorf("%s: routing.aliases maps %s and %s of %q onto the same segment %q",
					FileName, other, canonical, locale, public)
			}
			taken[public] = canonical
		}
		for canonical := range aliases {
			if other, clash := taken[canonical]; clash && other != canonical {
				return fmt.Errorf("%s: routing.aliases for %q gives %q two meanings", FileName, locale, canonical)
			}
		}
	}
	return nil
}

func segment(value, locale string) error {
	if value == "" || strings.ContainsAny(value, "/[]?# ") {
		return fmt.Errorf("%s: routing.aliases for %q holds %q, which is not a path segment", FileName, locale, value)
	}
	return nil
}

func validateNormalize(normalize Normalize) error {
	switch normalize.TrailingSlash {
	case "", NormalizeOff, SlashStrip, SlashKeep:
	default:
		return fmt.Errorf("%s: unknown routing.normalize.trailingSlash %q, want strip or keep",
			FileName, normalize.TrailingSlash)
	}
	switch normalize.Case {
	case "", NormalizeOff, CaseLower:
	default:
		return fmt.Errorf("%s: unknown routing.normalize.case %q, want lower", FileName, normalize.Case)
	}
	switch normalize.Diacritics {
	case "", NormalizeOff, FoldMarks:
	default:
		return fmt.Errorf("%s: unknown routing.normalize.diacritics %q, want fold", FileName, normalize.Diacritics)
	}
	return nil
}

func validateImages(images Images) error {
	switch images.Mode {
	case "", ImagesOn, ImagesOff:
	default:
		return fmt.Errorf("%s: unknown images.mode %q, want on or off", FileName, images.Mode)
	}
	if images.Quality < 0 || images.Quality > 100 {
		return fmt.Errorf("%s: images.quality %d must be between 1 and 100", FileName, images.Quality)
	}
	for _, width := range images.Widths {
		if width < 1 || width > MaxImageWidth {
			return fmt.Errorf("%s: images.widths holds %d, want a width between 1 and %d", FileName, width, MaxImageWidth)
		}
	}
	for _, host := range images.Hosts {
		if host == "" || strings.ContainsAny(host, "/:") {
			return fmt.Errorf("%s: images.hosts entry %q is not a host name", FileName, host)
		}
	}
	if err := freshnessFields(images.TTL, "", "images"); err != nil {
		return err
	}
	return nil
}

const MaxImageWidth = 4096

func validate(config Config) error {
	if config.CSS.Engine != "" && config.CSS.Engine != EnginePlain && config.CSS.Engine != EngineTailwind {
		return fmt.Errorf("%s: unknown css engine %q, want plain or tailwind", FileName, config.CSS.Engine)
	}
	if config.App.Scheme != "" && config.App.Scheme != SchemeHTTP && config.App.Scheme != SchemeHTTPS {
		return fmt.Errorf("%s: unknown scheme %q, want http or https", FileName, config.App.Scheme)
	}
	if config.Client.React != "" && !slices.Contains(reactEngines, config.Client.React) {
		return fmt.Errorf("%s: unknown react engine %q, want react or preact", FileName, config.Client.React)
	}
	if config.Cache.Variants < 0 {
		return fmt.Errorf("%s: cache.variants is %d, and a route holds at least one entry",
			FileName, config.Cache.Variants)
	}
	if err := validateFragments(config.Fragments); err != nil {
		return err
	}
	if err := validateSecurity(config.Security); err != nil {
		return err
	}
	if err := validateCSS(config.CSS); err != nil {
		return err
	}
	if err := validateSEO(config.SEO); err != nil {
		return err
	}
	if err := validateAliases(config); err != nil {
		return err
	}
	if err := validateImages(config.Images); err != nil {
		return err
	}
	if err := validateNormalize(config.Routing.Normalize); err != nil {
		return err
	}
	switch config.I18n.Mode {
	case ModePath, ModeSubdomain, ModeSingle:
	default:
		return fmt.Errorf("%s: unknown i18n mode %q, want path, subdomain or single", FileName, config.I18n.Mode)
	}
	if config.I18n.Timezone != "" {
		if _, err := time.LoadLocation(config.I18n.Timezone); err != nil {
			return fmt.Errorf("%s: i18n.timezone %q is not a zone the system knows: %w",
				FileName, config.I18n.Timezone, err)
		}
	}
	for _, locale := range config.I18n.Locales {
		if reserved := config.Reserves("/" + locale); reserved {
			return fmt.Errorf("%s: locale %q collides with the reserved prefix /%s", FileName, locale, locale)
		}
	}
	for _, redirect := range config.Redirects {
		if redirect.From == "" || redirect.To == "" {
			return fmt.Errorf("%s: a redirect needs both from and to", FileName)
		}
		if redirect.Status < 300 || redirect.Status > 399 {
			return fmt.Errorf("%s: redirect %s uses status %d, want a 3xx", FileName, redirect.From, redirect.Status)
		}
	}
	for _, rewrite := range config.Rewrites {
		if rewrite.From == "" || rewrite.To == "" {
			return fmt.Errorf("%s: a rewrite needs both from and to", FileName)
		}
	}
	switch config.Nav.Mode {
	case NavOff, NavPartial:
	default:
		return fmt.Errorf("%s: unknown nav mode %q, want off or partial", FileName, config.Nav.Mode)
	}
	if config.I18n.Mode == ModeSubdomain && len(config.Hosts) == 0 {
		return fmt.Errorf("%s: subdomain mode needs at least one entry under \"hosts\"", FileName)
	}
	return nil
}

func (c Config) Reserves(path string) bool {
	for _, prefix := range c.Routing.Reserved {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func (c Config) Localized() bool {
	return c.I18n.Mode == ModePath && len(c.I18n.Locales) > 0
}

func (c Config) Prefixes() []string {
	if c.I18n.Mode != ModePath {
		return nil
	}
	var prefixes []string
	for _, locale := range c.I18n.Locales {
		if locale == c.I18n.DefaultLocale && !c.I18n.PrefixDefault {
			continue
		}
		prefixes = append(prefixes, "/"+locale)
	}
	return prefixes
}

func (c Config) HostLocale(host string) (string, bool) {
	normalized := NormalizeHost(host)
	for _, entry := range c.Hosts {
		if entry.Pattern == normalized {
			return entry.Locale, true
		}
	}
	for _, entry := range c.Hosts {
		if entry.Default {
			return entry.Locale, false
		}
	}
	return c.I18n.DefaultLocale, len(c.Hosts) == 0
}

func NormalizeHost(host string) string {
	lowered := strings.ToLower(strings.TrimSpace(host))
	if cut := strings.LastIndex(lowered, ":"); cut > 0 && !strings.Contains(lowered[cut:], "]") {
		lowered = lowered[:cut]
	}
	return strings.TrimPrefix(lowered, "www.")
}

func (c Config) KnownHost(host string) bool {
	if len(c.Hosts) == 0 {
		return true
	}
	normalized := NormalizeHost(host)
	for _, entry := range c.Hosts {
		if entry.Pattern == normalized {
			return true
		}
	}
	return false
}
