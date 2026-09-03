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

	"github.com/apptivitypl/rill/internal/jsonc"
	"github.com/apptivitypl/rill/internal/paths"
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
	Reserved []string `json:"reserved,omitempty"`
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

type Config struct {
	Schema    string     `json:"$schema,omitempty"`
	App       App        `json:"app,omitempty"`
	CSS       CSS        `json:"css,omitempty"`
	Fragments Fragments  `json:"fragments,omitempty"`
	I18n      I18n       `json:"i18n,omitempty"`
	Routing   Routing    `json:"routing,omitempty"`
	Nav       Nav        `json:"nav,omitempty"`
	Hosts     []Host     `json:"hosts,omitempty"`
	Security  Security   `json:"security,omitempty"`
	Client    Client     `json:"client,omitempty"`
	Redirects []Redirect `json:"redirects,omitempty"`
	Rewrites  []Rewrite  `json:"rewrites,omitempty"`
}

var defaultReserved = []string{"/api", "/_rill", "/robots.txt", "/sitemap.xml", "/favicon.ico"}

func Default() Config {
	return Config{
		I18n: I18n{
			Mode:          ModePath,
			DefaultLocale: "en",
			Locales:       []string{"en"},
		},
		Routing:   Routing{Reserved: slices.Clone(defaultReserved)},
		Fragments: Fragments{Deferred: DeferredFetch},
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
	if err := validateFragments(config.Fragments); err != nil {
		return err
	}
	if err := validateSecurity(config.Security); err != nil {
		return err
	}
	if err := validateCSS(config.CSS); err != nil {
		return err
	}
	switch config.I18n.Mode {
	case ModePath, ModeSubdomain, ModeSingle:
	default:
		return fmt.Errorf("%s: unknown i18n mode %q, want path, subdomain or single", FileName, config.I18n.Mode)
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
