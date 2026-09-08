package server

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/image"
	"github.com/apptivitypl/gopage/internal/reply"
)

const (
	ImagePath      = "/_gopage/image"
	ImageFreshness = "public, max-age=31536000, immutable"
	maxRemoteBytes = image.MaxSourceSize
)

var errNoSource = errors.New("no such image")

type imageRequest struct {
	source  string
	width   int
	quality int
	format  string
}

func (a *App) image(w http.ResponseWriter, r *http.Request) {
	asked, err := a.imageRequest(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	load := func(bool) (cache.Value, cache.Policy, error) {
		source, err := a.imageSource(r, asked.source)
		if err != nil {
			return cache.Value{}, cache.Policy{}, err
		}
		body, format, err := image.Transform(source, image.Options{
			Width: asked.width, Quality: asked.quality, Format: asked.format,
		}, a.encoders)
		if err != nil {
			return cache.Value{}, cache.Policy{}, err
		}
		return cache.Value{
			Body:   body,
			Tags:   []string{"image", "image:" + asked.source},
			Header: http.Header{"Content-Type": []string{image.ContentType(format)}},
		}, cache.Policy{TTL: a.imageTTL()}, nil
	}
	value, status, err := a.serveImage(asked.key(), load)
	if err != nil {
		a.failImage(w, r, err)
		return
	}
	reply.Apply(w, value.Header)
	w.Header().Set(CacheHeader, status.String())
	w.Header().Set("Cache-Control", ImageFreshness)
	w.Header().Set("Content-Length", strconv.Itoa(len(value.Body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(value.Body); err != nil {
		a.logger.Error("write failed", "path", r.URL.Path, "error", err)
	}
}

func (a *App) serveImage(key string, load cache.Loader) (cache.Value, cache.Status, error) {
	if a.cache == nil {
		value, _, err := load(false)
		return value, cache.StatusBypass, err
	}
	return a.cache.Do(key, load)
}

func (i imageRequest) key() string {
	return ImageSource(ImagePath, i.source, i.width, i.quality, i.format)
}

func (a *App) failImage(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errNoSource) {
		http.NotFound(w, r)
		return
	}
	a.logger.Error("image failed", "path", r.URL.RawQuery, "error", err)
	http.Error(w, "image unavailable", http.StatusBadRequest)
}

func (a *App) imageRequest(r *http.Request) (imageRequest, error) {
	query := r.URL.Query()
	source := query.Get("src")
	if source == "" {
		return imageRequest{}, errNoSource
	}
	width, err := strconv.Atoi(query.Get("w"))
	if query.Get("w") != "" && (err != nil || width < 1 || width > image.MaxWidth) {
		return imageRequest{}, errNoSource
	}
	quality := a.config.Images.Sharpness()
	if asked := query.Get("q"); asked != "" {
		quality, err = strconv.Atoi(asked)
		if err != nil || quality < 1 || quality > 100 {
			return imageRequest{}, errNoSource
		}
	}
	format := query.Get("f")
	if format != "" && !image.Known(format) && a.encoders[format] == nil {
		return imageRequest{}, errNoSource
	}
	return imageRequest{source: source, width: width, quality: quality, format: format}, nil
}

func (a *App) imageSource(r *http.Request, source string) ([]byte, error) {
	if strings.HasPrefix(source, "/") {
		return a.localImage(r, source)
	}
	return a.remoteImage(r, source)
}

func (a *App) localImage(r *http.Request, source string) ([]byte, error) {
	if a.assets == nil {
		return nil, errNoSource
	}
	recorder := &imageWriter{header: http.Header{}}
	request := r.Clone(r.Context())
	request.Method = http.MethodGet
	request.URL = &url.URL{Path: source}
	request.Header = http.Header{}
	a.assets.ServeHTTP(recorder, request)
	if recorder.overflow {
		return nil, image.ErrTooLarge
	}
	if recorder.status != 0 && recorder.status != http.StatusOK {
		return nil, errNoSource
	}
	if len(recorder.body) == 0 {
		return nil, errNoSource
	}
	return recorder.body, nil
}

func (a *App) remoteImage(r *http.Request, source string) ([]byte, error) {
	target, err := url.Parse(source)
	if err != nil || target.Scheme != "https" || !a.config.Images.Serves(target.Hostname()) {
		return nil, errNoSource
	}
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, errNoSource
	}
	return io.ReadAll(io.LimitReader(response.Body, maxRemoteBytes+1))
}

type imageWriter struct {
	header   http.Header
	body     []byte
	status   int
	overflow bool
}

func (w *imageWriter) Header() http.Header { return w.header }

func (w *imageWriter) WriteHeader(status int) { w.status = status }

func (w *imageWriter) Write(data []byte) (int, error) {
	if len(w.body)+len(data) > maxRemoteBytes {
		w.overflow = true
		return 0, image.ErrTooLarge
	}
	w.body = append(w.body, data...)
	return len(data), nil
}

func (a *App) imageTTL() time.Duration {
	ttl, _ := a.config.Images.Freshness()
	return ttl
}

func ImageSource(prefix, source string, width, quality int, format string) string {
	query := url.Values{"src": []string{source}}
	if width > 0 {
		query.Set("w", strconv.Itoa(width))
	}
	if quality > 0 {
		query.Set("q", strconv.Itoa(quality))
	}
	if format != "" {
		query.Set("f", format)
	}
	return prefix + "?" + query.Encode()
}
