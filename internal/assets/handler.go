package assets

import (
	"bytes"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const CacheControl = "public, max-age=31536000, immutable"

type entry struct {
	asset   Asset
	content []byte
	brotli  []byte
	gzip    []byte
}

const (
	BrotliSuffix = ".br"
	GzipSuffix   = ".gz"
)

func (e entry) pick(accept string) ([]byte, string) {
	if len(e.brotli) > 0 && accepts(accept, "br") {
		return e.brotli, "br"
	}
	if len(e.gzip) > 0 && accepts(accept, "gzip") {
		return e.gzip, "gzip"
	}
	return e.content, ""
}

func accepts(header, coding string) bool {
	for part := range strings.SplitSeq(header, ",") {
		name, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(name), coding) {
			return true
		}
	}
	return false
}

func variants(fsys fs.FS, asset Asset) ([]byte, []byte) {
	brotli, _ := fs.ReadFile(fsys, asset.Source+BrotliSuffix)
	gzip, _ := fs.ReadFile(fsys, asset.Source+GzipSuffix)
	return brotli, gzip
}

func Handler(fsys fs.FS, list []Asset) (http.Handler, error) {
	entries := make(map[string]entry, len(list))
	for _, asset := range list {
		content, err := fs.ReadFile(fsys, asset.Source)
		if err != nil {
			return nil, err
		}
		brotli, gzip := variants(fsys, asset)
		entries[asset.Path] = entry{asset: asset, content: content, brotli: brotli, gzip: gzip}
	}
	return serveEntries(entries), nil
}

func cacheControl(asset Asset) string {
	if asset.Cache != "" {
		return asset.Cache
	}
	return CacheControl
}

func serveEntries(entries map[string]entry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		found, ok := entries[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("ETag", found.asset.ETag)
		w.Header().Set("Cache-Control", cacheControl(found.asset))
		w.Header().Set("Content-Type", found.asset.Type)
		w.Header().Add("Vary", "Accept-Encoding")
		body, coding := found.pick(r.Header.Get("Accept-Encoding"))
		if coding == "" {
			http.ServeContent(w, r, found.asset.Path, time.Time{}, bytes.NewReader(body))
			return
		}
		etag := EncodedETag(found.asset.ETag, coding)
		w.Header().Set("Content-Encoding", coding)
		w.Header().Set("ETag", etag)
		w.Header().Set("Accept-Ranges", "none")
		if matches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	})
}

func matches(header, etag string) bool {
	for candidate := range strings.SplitSeq(header, ",") {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "*" || trimmed == etag || strings.TrimPrefix(trimmed, "W/") == etag {
			return true
		}
	}
	return false
}

func EncodedETag(etag, coding string) string {
	if coding == "" || !strings.HasSuffix(etag, `"`) {
		return etag
	}
	return strings.TrimSuffix(etag, `"`) + "-" + coding + `"`
}

type Store struct {
	FS    fs.FS
	Files []Asset
}

func Serve(stores []Store) (http.Handler, []Asset, error) {
	entries := map[string]entry{}
	var all []Asset
	for _, store := range stores {
		for _, asset := range store.Files {
			content, err := fs.ReadFile(store.FS, asset.Source)
			if err != nil {
				return nil, nil, err
			}
			brotli, gzip := variants(store.FS, asset)
			entries[asset.Path] = entry{asset: asset, content: content, brotli: brotli, gzip: gzip}
			all = append(all, asset)
		}
	}
	if len(entries) == 0 {
		return nil, nil, nil
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })
	return serveEntries(entries), all, nil
}
