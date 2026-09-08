package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apptivitypl/gopage/internal/reply"
	"github.com/apptivitypl/gopage/internal/runtime"
)

func TestALoaderCannotForgeASecondHeader(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				reply.From(r.Context()).Header().Set("X-Probe", "ok\r\nX-Injected: yes")
				return runtime.Empty{}, nil
			},
		},
	})
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	answer, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = answer.Body.Close() }()
	t.Logf("X-Probe    = %q", answer.Header.Get("X-Probe"))
	t.Logf("X-Injected = %q", answer.Header.Get("X-Injected"))
	if answer.Header.Get("X-Injected") != "" {
		t.Error("a loader forged a second header on the wire")
	}
}

func TestACookieValueCannotForgeASecondCookie(t *testing.T) {
	app := New(Options{
		Manifest: manifest(),
		Props: map[string]PropsProvider{
			"index": func(r *http.Request, _ Params) (runtime.Accessible, error) {
				reply.From(r.Context()).SetCookie(&http.Cookie{Name: "probe", Value: "a\r\nSet-Cookie: evil=1"})
				return runtime.Empty{}, nil
			},
		},
	})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, held := range recorder.Result().Cookies() {
		t.Logf("cookie %s = %q", held.Name, held.Value)
		if held.Name == "evil" {
			t.Error("a cookie value forged a second cookie")
		}
	}
}
