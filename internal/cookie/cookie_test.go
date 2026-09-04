package cookie

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestASecureCookieCarriesTheHostPrefix(t *testing.T) {
	if got := Name("gopage.csrf", true); got != HostPrefix+"gopage.csrf" {
		t.Errorf("name = %q", got)
	}
	if got := Name("gopage.csrf", false); got != "gopage.csrf" {
		t.Errorf("name = %q", got)
	}
}

func TestTheShapeFollowsTheDecisionInTheContext(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(With(request.Context(), Options{Secure: true}))
	recorder := httptest.NewRecorder()
	Set(recorder, request, "gopage.csrf", "abc")

	held := recorder.Result().Cookies()
	if len(held) != 1 {
		t.Fatalf("cookies = %v", held)
	}
	if held[0].Name != HostPrefix+"gopage.csrf" || !held[0].Secure {
		t.Errorf("cookie = %+v, want a secure __Host- cookie", held[0])
	}
	if !held[0].HttpOnly || held[0].Path != "/" || held[0].SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = %+v", held[0])
	}
	if held[0].Domain != "" {
		t.Errorf("domain = %q, a __Host- cookie must carry none", held[0].Domain)
	}
}

func TestPlainTransportKeepsThePlainName(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(With(request.Context(), Options{}))
	recorder := httptest.NewRecorder()
	Set(recorder, request, "gopage.flash", "sent")

	held := recorder.Result().Cookies()[0]
	if held.Name != "gopage.flash" || held.Secure {
		t.Errorf("cookie = %+v", held)
	}
}

func TestWithoutADecisionTheTransportDecides(t *testing.T) {
	plain := httptest.NewRequest(http.MethodGet, "/", nil)
	if Of(plain).Secure {
		t.Error("a plain request is not secure")
	}
	encrypted := httptest.NewRequest(http.MethodGet, "/", nil)
	encrypted.TLS = &tls.ConnectionState{}
	if !Of(encrypted).Secure {
		t.Error("a tls request is secure")
	}
	if Of(plain.WithContext(context.Background())).Secure {
		t.Error("an empty context falls back to the transport")
	}
}

func TestAValueIsReadBackUnderTheSameName(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(With(request.Context(), Options{Secure: true}))
	request.AddCookie(&http.Cookie{Name: HostPrefix + "gopage.csrf", Value: "abc"})
	if got := Read(request, "gopage.csrf"); got != "abc" {
		t.Errorf("read = %q", got)
	}
	if got := Read(request, "gopage.flash"); got != "" {
		t.Errorf("read = %q, want nothing", got)
	}
}

func TestClearingSendsAnExpiredCookie(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	Clear(recorder, request, "gopage.flash")
	if held := recorder.Result().Cookies()[0]; held.MaxAge != -1 || held.Value != "" {
		t.Errorf("cookie = %+v", held)
	}
}
