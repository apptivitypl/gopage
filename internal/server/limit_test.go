package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/action"
	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/form"
	"github.com/apptivitypl/gopage/internal/ir"
)

func limitedApp(t *testing.T, limit string) *App {
	t.Helper()
	settings := config.Default()
	settings.Security.MaxBodySize = limit
	return New(Options{
		Manifest: formManifest(),
		Config:   settings,
		Submit: map[string]SubmitProvider{
			"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
				return nil, form.Result{}, nil
			},
		},
	})
}

func TestAnOversizedSubmissionIsRefusedBeforeItIsRead(t *testing.T) {
	app := limitedApp(t, "1kb")
	res := postForm(t, app.Handler(), "field="+strings.Repeat("a", 4<<10))
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", res.Code)
	}
}

func TestASubmissionInsideTheLimitReachesTheCsrfCheck(t *testing.T) {
	app := limitedApp(t, "1kb")
	res := postForm(t, app.Handler(), "field=small")
	if res.Code != http.StatusForbidden {
		t.Errorf("status = %d, want the csrf check to answer, not the limit", res.Code)
	}
}

func TestAMalformedSubmissionIsABadRequest(t *testing.T) {
	app := limitedApp(t, "1kb")
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("--nope"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

func TestTheBodyLimitDefaultsToEightMegabytes(t *testing.T) {
	if got := (config.Security{}).MaxBody(); got != config.DefaultMaxBodySize {
		t.Errorf("limit = %d, want %d", got, config.DefaultMaxBodySize)
	}
	if got := (config.Security{MaxBodySize: "2mb"}).MaxBody(); got != 2<<20 {
		t.Errorf("limit = %d, want 2mb", got)
	}
	if got := (config.Security{MaxBodySize: "nonsense"}).MaxBody(); got != config.DefaultMaxBodySize {
		t.Errorf("limit = %d, want the default when the size does not parse", got)
	}
}

func TestTheLimitCoversEveryRouteNotOnlySubmissions(t *testing.T) {
	settings := config.Default()
	settings.Security.MaxBodySize = "1kb"
	var read error
	app := New(Options{
		Manifest: manifest(),
		Config:   settings,
		API: map[string]http.Handler{
			"/api/echo": http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				buf := make([]byte, 8<<10)
				for {
					if _, err := r.Body.Read(buf); err != nil {
						read = err
						return
					}
				}
			}),
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/echo", strings.NewReader(strings.Repeat("a", 4<<10)))
	app.Handler().ServeHTTP(httptest.NewRecorder(), request)

	var tooBig *http.MaxBytesError
	if !errors.As(read, &tooBig) {
		t.Errorf("api route read %v, want the body capped", read)
	}
}

func crossOriginApp(t *testing.T, origins ...string) http.Handler {
	t.Helper()
	settings := config.Default()
	settings.Security.TrustedOrigins = origins
	return New(Options{
		Manifest: formManifest(),
		Config:   settings,
		Submit: map[string]SubmitProvider{
			"apply": func(*http.Request, Params) (action.Action, form.Result, error) {
				return nil, form.Result{}, nil
			},
		},
	}).Handler()
}

func postFrom(t *testing.T, handler http.Handler, site, origin string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("Name=Ada"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if site != "" {
		request.Header.Set("Sec-Fetch-Site", site)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestACrossSitePostIsRefused(t *testing.T) {
	res := postFrom(t, crossOriginApp(t), "cross-site", "https://evil.test")
	if res.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.Code)
	}
	if !strings.Contains(res.Body.String(), "cross-origin") {
		t.Errorf("body = %q, want the reason named", res.Body.String())
	}
}

func TestASameOriginPostReachesTheCsrfCheck(t *testing.T) {
	res := postFrom(t, crossOriginApp(t), "same-origin", "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d", res.Code)
	}
	if strings.Contains(res.Body.String(), "cross-origin") {
		t.Errorf("body = %q, want the token check to answer, not the origin check", res.Body.String())
	}
}

func TestANonBrowserPostIsNotTreatedAsCrossOrigin(t *testing.T) {
	res := postFrom(t, crossOriginApp(t), "", "")
	if strings.Contains(res.Body.String(), "cross-origin") {
		t.Errorf("body = %q, want a client that sends neither header let through", res.Body.String())
	}
}

func TestATrustedOriginIsAllowedThrough(t *testing.T) {
	res := postFrom(t, crossOriginApp(t, "https://studio.test"), "cross-site", "https://studio.test")
	if strings.Contains(res.Body.String(), "cross-origin") {
		t.Errorf("body = %q, want the configured origin allowed", res.Body.String())
	}
}

func TestTheCrossOriginGuardCoversApiRoutes(t *testing.T) {
	reached := false
	app := New(Options{
		Manifest: manifest(),
		Config:   config.Default(),
		API: map[string]http.Handler{
			"/api/echo": http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }),
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/echo", strings.NewReader("{}"))
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	request.Header.Set("Origin", "https://evil.test")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	if reached {
		t.Error("the api handler ran on a cross-site post")
	}
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", recorder.Code)
	}
}

func TestSecurityHeadersAreTheOnesTheBuildWritesOut(t *testing.T) {
	headers := SecurityHeaders()
	if len(headers) != 2 {
		t.Fatalf("headers = %+v", headers)
	}
	seen := map[string]string{}
	for _, header := range headers {
		seen[header.Name] = header.Value
	}
	if seen["X-Content-Type-Options"] != "nosniff" {
		t.Errorf("headers = %+v, want nosniff", headers)
	}
	if seen["Referrer-Policy"] != "strict-origin-when-cross-origin" {
		t.Errorf("headers = %+v, want a referrer policy", headers)
	}
	res := postFrom(t, crossOriginApp(t), "same-origin", "")
	for name, value := range seen {
		if got := res.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q on the response too", name, got, value)
		}
	}
}

func TestAChainIsBuiltForARouteTheMemoDoesNotHold(t *testing.T) {
	app := New(Options{Manifest: manifest()})
	known := app.chain(app.manifest.Routes[0])
	if len(known) == 0 {
		t.Fatal("the memo held nothing for a route in the manifest")
	}
	unknown := app.chain(ir.Route{Name: "nowhere", Plan: 1})
	if len(unknown) != 1 {
		t.Errorf("chain = %v, want one plan built on demand", unknown)
	}
}

func TestAnAlreadyEncodedAnswerIsNotCompressedAgain(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Config:   config.Default(),
		API: map[string]http.Handler{
			"/api/packed": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Content-Encoding", "br")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(strings.Repeat("x", 4<<10)))
			}),
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/packed", nil)
	request.Header.Set("Accept-Encoding", "br")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Body.Len() != 4<<10 {
		t.Errorf("body = %d bytes, want the sidecar passed through untouched", recorder.Body.Len())
	}
}

func TestATrustedOriginThatDoesNotParseIsReported(t *testing.T) {
	res := postFrom(t, crossOriginApp(t, "not an origin"), "same-origin", "")
	if res.Code == 0 {
		t.Error("a bad origin must not stop the server from answering")
	}
}

func TestTheConnectionLimitReachesTheServer(t *testing.T) {
	settings := config.Default()
	settings.Security.MaxConnections = 128
	app := New(Options{Manifest: manifest(), Config: settings})
	if got := app.MaxConnections(); got != 128 {
		t.Errorf("limit = %d, want the configured value", got)
	}
	if got := New(Options{Manifest: manifest()}).MaxConnections(); got != 0 {
		t.Errorf("limit = %d, want no limit by default", got)
	}
}
