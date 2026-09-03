package smoke

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/sonquer/rill/internal/config"
	"github.com/sonquer/rill/internal/paths"
)

const ReferenceTemplate = "catalog"

func ReferenceChecks() []Check {
	return []Check{
		{Path: "/", Status: http.StatusOK, Contains: "what is on offer", ContentType: "text/html"},
		{Path: "/", Status: http.StatusOK, Contains: "4 listings"},
		{Path: "/", Status: http.StatusOK, Contains: `<span class="item-price">410 000.00 PLN</span>`},
		{Path: "/", Status: http.StatusOK, Contains: `<rill-island`},
		{Path: "/", Status: http.StatusOK, Contains: `<link rel="alternate" hreflang="pl"`},
		{Path: "/?city=krakow", Status: http.StatusOK, Contains: "2 listings"},
		{Path: "/pl", Status: http.StatusOK, Contains: "4 oferty"},
		{Path: "/pl", Status: http.StatusOK, Contains: "aktywna"},
		{Path: "/items/2", Status: http.StatusOK, Contains: "flat by the river"},
		{Path: "/items/2", Status: http.StatusOK, Contains: "14 354.00 PLN per m2"},
		{Path: "/items/99", Status: http.StatusNotFound, Contains: "<title>page not found</title>"},
		{Path: "/api/health", Status: http.StatusOK, Contains: `"runtime":"go"`, ContentType: "application/json"},
		{Path: FeedPath, Status: http.StatusOK, Contains: FeedEvent, ContentType: FeedType},
		{Path: FeedPath, Status: http.StatusOK, Contains: "event: done"},
		{Path: "/sitemap.xml", Status: http.StatusOK, Contains: "<loc>http://", ContentType: "application/xml"},
		{Path: "/robots.txt", Status: http.StatusOK, Contains: "Sitemap: http://"},
		{Path: "/en", Status: http.StatusMovedPermanently},
	}
}

func RunReference(fetch Fetcher, base string) error {
	for _, check := range ReferenceChecks() {
		response, err := fetch(base + check.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", check.Path, err)
		}
		if err := Verify(check, response); err != nil {
			return err
		}
	}
	return nil
}

func CompareAdapters(native, worker Fetcher, nativeBase, workerBase string) error {
	for _, check := range ReferenceChecks() {
		if check.Status != http.StatusOK {
			continue
		}
		left, err := native(nativeBase + check.Path)
		if err != nil {
			return fmt.Errorf("native %s: %w", check.Path, err)
		}
		right, err := worker(workerBase + check.Path)
		if err != nil {
			return fmt.Errorf("worker %s: %w", check.Path, err)
		}
		if left.Status != right.Status {
			return fmt.Errorf("%s: native answered %d, the worker answered %d",
				check.Path, left.Status, right.Status)
		}
		if normalise(left.Body, nativeBase) != normalise(right.Body, workerBase) {
			return fmt.Errorf("%s: the two adapters returned different documents", check.Path)
		}
	}
	return nil
}

var volatile = regexp.MustCompile(`name="__csrf" value="[^"]*"`)

func normalise(body, base string) string {
	return volatile.ReplaceAllString(strings.ReplaceAll(body, base, ""), `name="__csrf"`)
}

type HostCheck struct {
	Host string
	Check
}

type HostFetcher func(host, url string) (Response, error)

func SubdomainChecks() []HostCheck {
	return []HostCheck{
		{Host: DefaultHost, Check: Check{Path: "/", Status: http.StatusOK, Contains: "what is on offer"}},
		{Host: DefaultHost, Check: Check{Path: "/", Status: http.StatusOK, Contains: "4 listings"}},
		{Host: DefaultHost, Check: Check{Path: "/items/2", Status: http.StatusOK, Contains: "14 354.00 PLN per m2"}},
		{Host: LocaleHost, Check: Check{Path: "/", Status: http.StatusOK, Contains: "4 oferty"}},
		{Host: LocaleHost, Check: Check{Path: "/", Status: http.StatusOK, Contains: "aktywna"}},
		{Host: LocaleHost, Check: Check{Path: "/items/2", Status: http.StatusOK, Contains: "za m2"}},
		{Host: LocaleHost, Check: Check{Path: "/pl", Status: http.StatusNotFound}},
		{Host: DefaultHost, Check: Check{Path: "/api/health", Status: http.StatusOK, Contains: `"runtime":"go"`}},
		{Host: DefaultHost, Check: Check{Path: "/sitemap.xml", Status: http.StatusOK, Contains: "<loc>http://"}},
		{Host: "unlisted.example.net", Check: Check{Path: "/", Status: http.StatusMisdirectedRequest}},
	}
}

func RunSubdomain(fetch HostFetcher, base string) error {
	for _, check := range SubdomainChecks() {
		response, err := fetch(check.Host, base+check.Path)
		if err != nil {
			return fmt.Errorf("%s%s: %w", check.Host, check.Path, err)
		}
		if err := Verify(check.Check, response); err != nil {
			return fmt.Errorf("%s: %w", check.Host, err)
		}
	}
	return nil
}

const (
	DefaultHost = "catalog.example.com"
	LocaleHost  = "pl.catalog.example.com"
)

func SubdomainConfig(source string) (string, error) {
	settings, err := config.Parse(source)
	if err != nil {
		return "", err
	}
	settings.I18n.Mode = config.ModeSubdomain
	settings.Hosts = []config.Host{
		{Pattern: DefaultHost, Locale: "en", Default: true},
		{Pattern: LocaleHost, Locale: "pl"},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

var generated = skipped()

func skipped() map[string]bool {
	set := map[string]bool{".wrangler": true, "node_modules": true}
	for _, dir := range paths.Generated() {
		set[dir] = true
	}
	return set
}

func Fingerprint(root fs.FS) (map[string]string, error) {
	files := map[string]string{}
	err := fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if generated[path] {
				return fs.SkipDir
			}
			return nil
		}
		if path == paths.Config || strings.HasSuffix(path, ".log") {
			return nil
		}
		data, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		files[path] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func Changed(before, after map[string]string) []string {
	var changed []string
	for path, sum := range before {
		if next, ok := after[path]; !ok || next != sum {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

const (
	FeedPath  = "/api/feed"
	FeedType  = "text/event-stream"
	FeedEvent = "event: item"
)
