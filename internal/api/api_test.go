package api

import (
	"log/slog"

	"errors"
	"github.com/apptivitypl/rill/internal/logs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func call(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder, _ := heard(t, handler, method, target)
	return recorder
}

func heard(t *testing.T, handler http.Handler, method, target string) (*httptest.ResponseRecorder, *strings.Builder) {
	t.Helper()
	var written strings.Builder
	logger := slog.New(slog.NewTextHandler(&written, nil))
	request := httptest.NewRequest(method, target, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request.WithContext(logs.With(request.Context(), logger)))
	return recorder, &written
}

func TestJSONEncodesTheValue(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet: func(*http.Request) (Response, error) { return JSON(map[string]int{"n": 1}), nil },
	})
	recorder := call(t, handler, http.MethodGet, "/api/x")
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"n":1}` {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content type = %q", got)
	}
}

func TestStatusCanBeOverridden(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodPost: func(*http.Request) (Response, error) { return JSON(nil).WithStatus(http.StatusCreated), nil },
	})
	if recorder := call(t, handler, http.MethodPost, "/api/x"); recorder.Code != http.StatusCreated {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestTextAndNoContent(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet:    func(*http.Request) (Response, error) { return Text("pong"), nil },
		http.MethodDelete: func(*http.Request) (Response, error) { return NoContent(), nil },
	})
	if recorder := call(t, handler, http.MethodGet, "/api/x"); recorder.Body.String() != "pong" {
		t.Errorf("body = %q", recorder.Body.String())
	}
	recorder := call(t, handler, http.MethodDelete, "/api/x")
	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestUnsupportedMethodListsWhatIsAllowed(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet:  func(*http.Request) (Response, error) { return Text("ok"), nil },
		http.MethodPost: func(*http.Request) (Response, error) { return Text("ok"), nil },
	})
	recorder := call(t, handler, http.MethodPut, "/api/x")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET, HEAD, POST" {
		t.Errorf("allow = %q", got)
	}
}

func TestHeadFallsBackToGet(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet: func(*http.Request) (Response, error) { return Text("ok"), nil },
	})
	if recorder := call(t, handler, http.MethodHead, "/api/x"); recorder.Code != http.StatusOK {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestHandlerErrorsBecomeFiveHundred(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet: func(*http.Request) (Response, error) { return nil, errors.New("nope") },
	})
	recorder, written := heard(t, handler, http.MethodGet, "/api/x")
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "internal server error") {
		t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(written.String(), "api handler failed") {
		t.Errorf("log = %q, want the failure named", written.String())
	}
}

func TestNilResponseIsAnError(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet: func(*http.Request) (Response, error) { return nil, nil },
	})
	recorder, written := heard(t, handler, http.MethodGet, "/api/x")
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
	if !strings.Contains(written.String(), "answered nothing") {
		t.Errorf("log = %q, want a silent 500 to leave a line", written.String())
	}
}

func TestUnencodableValueIsReported(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet: func(*http.Request) (Response, error) { return JSON(make(chan int)), nil },
	})
	_, written := heard(t, handler, http.MethodGet, "/api/x")
	if !strings.Contains(written.String(), "api write failed") {
		t.Errorf("log = %q", written.String())
	}
}

func TestMuxWithoutHandlersAllowsNothing(t *testing.T) {
	recorder, written := heard(t, Mux(nil), http.MethodGet, "/api/x")
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != "" {
		t.Errorf("status = %d, allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
	if !strings.Contains(written.String(), "api method not allowed") {
		t.Errorf("log = %q, want a refused method to leave a line", written.String())
	}
}

func TestContentCarriesItsOwnType(t *testing.T) {
	handler := Mux(map[string]Handler{
		http.MethodGet: func(*http.Request) (Response, error) {
			return Content("application/rss+xml; charset=utf-8", []byte("<rss/>")), nil
		},
	})
	recorder := call(t, handler, http.MethodGet, "/feed.xml")
	if recorder.Header().Get("Content-Type") != "application/rss+xml; charset=utf-8" {
		t.Errorf("content type = %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() != "<rss/>" {
		t.Errorf("body = %q", recorder.Body.String())
	}
}
