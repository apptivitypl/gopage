package reply

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func request(t *testing.T) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/", nil)
}

func TestAnAbsentRecorderIsAnEmptyOne(t *testing.T) {
	recorder := From(context.Background())
	if recorder == nil || recorder.Touched() {
		t.Errorf("recorder = %+v", recorder)
	}
}

func TestARecorderTravelsInTheContext(t *testing.T) {
	recorder := NewRecorder()
	recorder.Status(http.StatusGone)
	if got := From(WithRecorder(context.Background(), recorder)).Code(); got != http.StatusGone {
		t.Errorf("status = %d", got)
	}
}

func TestOnlyARealStatusIsKept(t *testing.T) {
	recorder := NewRecorder()
	for _, code := range []int{100, 199, 600, 0, -1} {
		recorder.Status(code)
		if recorder.Code() != 0 {
			t.Errorf("status %d was kept", code)
		}
	}
	recorder.Status(http.StatusGone)
	if recorder.Code() != http.StatusGone {
		t.Errorf("status = %d", recorder.Code())
	}
}

func TestARefusedStatusIsLoggedAndDropped(t *testing.T) {
	recorder := NewRecorder()
	recorder.Status(999)
	if !recorder.Touched() {
		t.Error("a refused status still counts as touching the response")
	}
	response := httptest.NewRecorder()
	recorder.Deliver(response, request(t), false)
	if response.Code != http.StatusOK {
		t.Errorf("code = %d", response.Code)
	}
}

func TestHeadersAndVaryTravelTogether(t *testing.T) {
	recorder := NewRecorder()
	recorder.Header().Set("X-Robots-Tag", "noindex, follow")
	recorder.Vary("Cookie", "Cookie", "")
	headers := recorder.Headers()
	if headers.Get("X-Robots-Tag") != "noindex, follow" || headers.Get(VaryHeader) != "Cookie" {
		t.Errorf("headers = %v", headers)
	}
	if NewRecorder().Headers() != nil {
		t.Error("an untouched recorder carries no headers")
	}
	bare := &Recorder{}
	bare.Vary(CookieVary)
	if got := bare.Headers().Get(VaryHeader); got != CookieVary {
		t.Errorf("vary = %q, want a zero recorder to answer too", got)
	}
}

func TestApplyMergesVaryAndAssignsTheRest(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set(VaryHeader, "GOPAGE-Fragment")
	response.Header().Set("X-Robots-Tag", "index")
	Apply(response, http.Header{
		VaryHeader:     []string{"Cookie"},
		"X-Robots-Tag": []string{"noindex"},
	})
	if got := response.Header().Get(VaryHeader); got != "GOPAGE-Fragment, Cookie" {
		t.Errorf("vary = %q", got)
	}
	if got := response.Header().Get("X-Robots-Tag"); got != "noindex" {
		t.Errorf("x-robots-tag = %q", got)
	}
}

func TestAddVaryKeepsTheHeaderUnique(t *testing.T) {
	response := httptest.NewRecorder()
	AddVary(response, "Cookie")
	AddVary(response, "Cookie", "")
	if got := response.Header().Get(VaryHeader); got != "Cookie" {
		t.Errorf("vary = %q", got)
	}
	AddVary(httptest.NewRecorder())
	Apply(response, nil)
}

func TestCookiesReachTheResponse(t *testing.T) {
	recorder := NewRecorder()
	recorder.SetCookie(&http.Cookie{Name: "session", Value: "abc", Domain: ".example.com", HttpOnly: true})
	recorder.SetCookie(nil)
	response := httptest.NewRecorder()
	recorder.Deliver(response, request(t), true)
	got := response.Header().Get("Set-Cookie")
	for _, want := range []string{"session=abc", "Domain=example.com", "Path=/", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(got, want) {
			t.Errorf("cookie = %q, want %q", got, want)
		}
	}
}

func TestARefusedCookieIsDropped(t *testing.T) {
	recorder := NewRecorder()
	recorder.SetCookie(&http.Cookie{Name: "gopage.csrf", Value: "x"})
	response := httptest.NewRecorder()
	recorder.Deliver(response, request(t), false)
	if got := response.Header().Get("Set-Cookie"); got != "" {
		t.Errorf("cookie = %q, want the framework namespace refused", got)
	}
}

func TestCheckRefusesWhatTheBrowserWouldDrop(t *testing.T) {
	cases := map[string]*http.Cookie{
		"framework namespace": {Name: "gopage.flash", Value: "x"},
		"hosted namespace":    {Name: "__Host-gopage.csrf", Value: "x"},
		"hosted with domain":  {Name: "__Host-session", Value: "x", Domain: "example.com"},
		"hosted off the root": {Name: "__Host-session", Value: "x", Path: "/deep"},
		"oversized":           {Name: "session", Value: strings.Repeat("x", MaxCookieBytes)},
		"malformed":           {Name: "sess ion", Value: "x"},
	}
	for name, held := range cases {
		if _, err := Check(held, false); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := Check(nil, false); err == nil {
		t.Error("a nil cookie was accepted")
	}
}

func TestCheckKeepsAHostedCookieAtTheRoot(t *testing.T) {
	shaped, err := Check(&http.Cookie{Name: "__Host-session", Value: "x"}, true)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if shaped.Path != "/" || !shaped.Secure {
		t.Errorf("cookie = %+v", shaped)
	}
}

func TestCheckLeavesADeliberateShapeAlone(t *testing.T) {
	shaped, err := Check(&http.Cookie{
		Name: "theme", Value: "dark", Path: "/settings", SameSite: http.SameSiteStrictMode,
	}, false)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if shaped.Path != "/settings" || shaped.SameSite != http.SameSiteStrictMode || shaped.Secure {
		t.Errorf("cookie = %+v", shaped)
	}
}
