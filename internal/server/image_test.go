package server

import (
	"bytes"
	"errors"
	"image"
	stdcolor "image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/apptivitypl/gopage/internal/cache"
	gimage "github.com/apptivitypl/gopage/internal/image"
)

func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.SetRGBA(x, y, stdcolor.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, picture, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return out.Bytes()
}

func imageApp(t *testing.T, text string, served []byte, calls *atomic.Int64) *App {
	t.Helper()
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		if r.URL.Path != "/photo.jpg" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(served)
	})
	return New(Options{
		Manifest: manifest(),
		Config:   settings(t, text),
		Cache:    cache.New(cache.Options{Limit: 4 << 20}),
		Assets:   assets,
		Images:   gimage.Support{},
	})
}

const optimising = `{"images": {"mode": "on", "widths": [320, 640]}}`

func TestTheEndpointResizesAProjectImage(t *testing.T) {
	app := imageApp(t, optimising, jpegBytes(t, 400, 200), nil)
	response := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg&w=100")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("content type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != ImageFreshness {
		t.Errorf("cache-control = %q", got)
	}
	picture, _, err := image.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if picture.Bounds().Dx() != 100 {
		t.Errorf("width = %d", picture.Bounds().Dx())
	}
}

func TestASecondImageRequestComesFromTheCache(t *testing.T) {
	var calls atomic.Int64
	app := imageApp(t, optimising, jpegBytes(t, 200, 100), &calls)
	handler := app.Handler()
	get(t, handler, ImagePath+"?src=%2Fphoto.jpg&w=50")
	response := get(t, handler, ImagePath+"?src=%2Fphoto.jpg&w=50")
	if got := response.Header().Get(CacheHeader); got != "hit" {
		t.Errorf("cache = %q", got)
	}
	if calls.Load() != 1 {
		t.Errorf("source reads = %d", calls.Load())
	}
	if dropped := app.Invalidate("image:/photo.jpg"); dropped != 1 {
		t.Errorf("invalidated %d, want the tagged entry", dropped)
	}
}

func TestTheEndpointRefusesNonsense(t *testing.T) {
	app := imageApp(t, optimising, jpegBytes(t, 40, 20), nil)
	handler := app.Handler()
	for _, target := range []string{
		ImagePath,
		ImagePath + "?src=%2Fphoto.jpg&w=0",
		ImagePath + "?src=%2Fphoto.jpg&w=99999",
		ImagePath + "?src=%2Fphoto.jpg&q=0",
		ImagePath + "?src=%2Fphoto.jpg&q=abc",
		ImagePath + "?src=%2Fphoto.jpg&f=tiff",
	} {
		if code := get(t, handler, target).Code; code != http.StatusBadRequest {
			t.Errorf("%s answered %d", target, code)
		}
	}
	if code := get(t, handler, ImagePath+"?src=%2Fmissing.jpg&w=10").Code; code != http.StatusNotFound {
		t.Errorf("a missing source answered %d", code)
	}
}

func TestARemoteSourceNeedsAnAllowedHost(t *testing.T) {
	app := imageApp(t, optimising, nil, nil)
	handler := app.Handler()
	for _, target := range []string{
		ImagePath + "?src=https%3A%2F%2Fcdn.example.com%2Fa.jpg",
		ImagePath + "?src=http%3A%2F%2Fcdn.example.com%2Fa.jpg",
		ImagePath + "?src=%3A%2F%2Fbroken",
	} {
		if code := get(t, handler, target).Code; code != http.StatusNotFound {
			t.Errorf("%s answered %d, want the host refused", target, code)
		}
	}
}

func TestAnAllowedHostIsFetched(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jpegBytes(t, 200, 100))
	}))
	defer upstream.Close()
	host := strings.TrimPrefix(upstream.URL, "http://")
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, `{"images": {"mode": "on", "hosts": ["`+strings.Split(host, ":")[0]+`"]}}`),
		Cache:    cache.New(cache.Options{Limit: 4 << 20}),
		Client:   upstream.Client(),
		Images:   gimage.Support{},
	})
	app.client = &http.Client{Transport: rewriting{to: upstream.URL}}
	response := get(t, app.Handler(), ImagePath+"?src=https%3A%2F%2F"+strings.Split(host, ":")[0]+"%2Fa.jpg&w=40")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
}

type rewriting struct {
	to string
}

func (r rewriting) RoundTrip(request *http.Request) (*http.Response, error) {
	target := *request.URL
	target.Scheme = "http"
	target.Host = strings.TrimPrefix(r.to, "http://")
	clone := request.Clone(request.Context())
	clone.URL = &target
	return http.DefaultTransport.RoundTrip(clone)
}

func TestTheEndpointIsOffUntilItIsAskedFor(t *testing.T) {
	app := imageApp(t, "{}", jpegBytes(t, 40, 20), nil)
	if code := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg").Code; code != http.StatusNotFound {
		t.Errorf("status = %d", code)
	}
}

func TestAnAppMayPlugAnEncoderIn(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, optimising),
		Assets: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(jpegBytes(t, 80, 40))
		}),
		Images: gimage.Support{Encoders: map[string]gimage.Encoder{
			"webp": func(w io.Writer, _ image.Image, _ int) error {
				_, err := w.Write([]byte("RIFFWEBP"))
				return err
			},
		}},
	})
	response := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg&w=40&f=webp")
	if response.Code != http.StatusOK || response.Body.String() != "RIFFWEBP" {
		t.Errorf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "image/webp" {
		t.Errorf("content type = %q", got)
	}
}

func TestAHeadRequestSendsNoBody(t *testing.T) {
	app := imageApp(t, optimising, jpegBytes(t, 40, 20), nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, ImagePath+"?src=%2Fphoto.jpg&w=20", nil))
	if recorder.Body.Len() != 0 {
		t.Errorf("body = %d bytes", recorder.Body.Len())
	}
}

func TestASourceThatBreaksIsReported(t *testing.T) {
	app := imageApp(t, optimising, []byte("not an image"), nil)
	if code := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg&w=10").Code; code != http.StatusBadRequest {
		t.Errorf("status = %d", code)
	}
}

func TestASourceThatAnswersAnErrorIsMissing(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, optimising),
		Assets: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}),
		Images: gimage.Support{},
	})
	if code := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg&w=10").Code; code != http.StatusNotFound {
		t.Errorf("status = %d", code)
	}
}

func TestAnAppWithoutAssetsHasNoLocalImages(t *testing.T) {
	app := New(Options{Manifest: manifest(), Config: settings(t, optimising), Images: gimage.Support{}})
	if code := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg&w=10").Code; code != http.StatusNotFound {
		t.Errorf("status = %d", code)
	}
}

func TestAnOversizedSourceIsRefused(t *testing.T) {
	huge := bytes.Repeat([]byte{0x7f}, gimage.MaxSourceSize+16)
	app := imageApp(t, optimising, huge, nil)
	if code := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg&w=10").Code; code != http.StatusBadRequest {
		t.Errorf("status = %d", code)
	}
}

func TestAnUnreachableHostIsReported(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, `{"images": {"mode": "on", "hosts": ["cdn.invalid"]}}`),
		Client:   &http.Client{Transport: failing{}},
		Images:   gimage.Support{},
	})
	if code := get(t, app.Handler(), ImagePath+"?src=https%3A%2F%2Fcdn.invalid%2Fa.jpg").Code; code != http.StatusBadRequest {
		t.Errorf("status = %d", code)
	}
}

func TestARemoteErrorIsMissing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, `{"images": {"mode": "on", "hosts": ["cdn.example.com"]}}`),
		Images:   gimage.Support{},
	})
	app.client = &http.Client{Transport: rewriting{to: upstream.URL}}
	if code := get(t, app.Handler(), ImagePath+"?src=https%3A%2F%2Fcdn.example.com%2Fa.jpg").Code; code != http.StatusNotFound {
		t.Errorf("status = %d", code)
	}
}

type failing struct{}

func (failing) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestAnEmptySourceIsMissing(t *testing.T) {
	app := imageApp(t, optimising, nil, nil)
	if code := get(t, app.Handler(), ImagePath+"?src=%2Fphoto.jpg&w=10").Code; code != http.StatusNotFound {
		t.Errorf("status = %d", code)
	}
}

func TestARemoteAddressThatCannotBeRequestedIsMissing(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, `{"images": {"mode": "on", "hosts": ["cdn.example.com"]}}`),
		Images:   gimage.Support{},
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := app.remoteImage(request, "https://cdn.example.com/\x7f"); !errors.Is(err, errNoSource) {
		t.Errorf("error = %v", err)
	}
}

func TestAnImageWriteFailureIsLogged(t *testing.T) {
	app := imageApp(t, optimising, jpegBytes(t, 40, 20), nil)
	app.Handler().ServeHTTP(&refusingWriter{}, httptest.NewRequest(http.MethodGet, ImagePath+"?src=%2Fphoto.jpg&w=20", nil))
}

func TestARemoteBodyThatBreaksIsReported(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = w.Write([]byte("short"))
	}))
	defer upstream.Close()
	app := New(Options{
		Manifest: manifest(),
		Config:   settings(t, `{"images": {"mode": "on", "hosts": ["cdn.example.com"]}}`),
		Images:   gimage.Support{},
	})
	app.client = &http.Client{Transport: rewriting{to: upstream.URL}}
	if code := get(t, app.Handler(), ImagePath+"?src=https%3A%2F%2Fcdn.example.com%2Fa.jpg").Code; code != http.StatusBadRequest {
		t.Errorf("status = %d", code)
	}
}
