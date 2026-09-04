package smoke

import "fmt"

const (
	CacheHeader = "GOPAGE-Cache"
	CachedPath  = "/"
)

func RunCache(fetch Fetcher, base string) error {
	first, err := fetch(base + CachedPath)
	if err != nil {
		return err
	}
	if got := first.Headers.Get(CacheHeader); got != "miss" && got != "hit" {
		return fmt.Errorf("%s: first read reported %q, want a miss or a hit", CachedPath, got)
	}
	second, err := fetch(base + CachedPath)
	if err != nil {
		return err
	}
	if got := second.Headers.Get(CacheHeader); got != "hit" {
		return fmt.Errorf("%s: second read reported %q, want a hit", CachedPath, got)
	}
	if first.Body != second.Body {
		return fmt.Errorf("%s: the cached body differs from the rendered one", CachedPath)
	}
	return nil
}
