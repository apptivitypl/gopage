package action

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func resolver(route string, params map[string]string) (string, error) {
	if route != "listings.thanks" {
		return "", errors.New("unknown route")
	}
	return "/listings/" + params["id"] + "/thanks", nil
}

func apply(t *testing.T, action Action, resolve Resolver) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	if err := action.Apply(recorder, httptest.NewRequest(http.MethodPost, "/apply", nil), resolve); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return recorder
}

func TestRedirectToALiteralUrl(t *testing.T) {
	recorder := apply(t, To("/thanks"), nil)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/thanks" {
		t.Errorf("status = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestRedirectToANamedRoute(t *testing.T) {
	recorder := apply(t, Route("listings.thanks").WithParam("id", "7"), resolver)
	if recorder.Header().Get("Location") != "/listings/7/thanks" {
		t.Errorf("location = %q", recorder.Header().Get("Location"))
	}
}

func TestRedirectWithoutATargetIsReported(t *testing.T) {
	err := Route("").Apply(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), resolver)
	if !errors.Is(err, ErrNoTarget) {
		t.Errorf("err = %v", err)
	}
}

func TestRedirectWithoutAResolverIsReported(t *testing.T) {
	err := Route("x").Apply(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), nil)
	if !errors.Is(err, ErrNoTarget) {
		t.Errorf("err = %v", err)
	}
}

func TestAnUnknownRouteIsReported(t *testing.T) {
	err := Route("nope").Apply(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), resolver)
	if err == nil {
		t.Error("an unresolvable route must be reported")
	}
}

func TestFlashAndRefreshTravelWithTheRedirect(t *testing.T) {
	recorder := apply(t, To("/thanks").Flash("apply.sent").Refresh("cart-badge", "count"), nil)
	if !strings.Contains(recorder.Header().Get("Set-Cookie"), FlashCookie+"=apply.sent") {
		t.Errorf("cookie = %q", recorder.Header().Get("Set-Cookie"))
	}
	if got := recorder.Header().Get(RefreshHeader); got != "cart-badge, count" {
		t.Errorf("refresh = %q", got)
	}
}

func TestStatusCanBeOverridden(t *testing.T) {
	recorder := apply(t, To("/thanks").Status(http.StatusMovedPermanently), nil)
	if recorder.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestTakeFlashReadsAndClears(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/thanks", nil)
	request.AddCookie(&http.Cookie{Name: FlashCookie, Value: "apply.sent"})
	recorder := httptest.NewRecorder()
	if got := TakeFlash(recorder, request); got != "apply.sent" {
		t.Errorf("flash = %q", got)
	}
	if !strings.Contains(recorder.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("the cookie must be cleared: %q", recorder.Header().Get("Set-Cookie"))
	}
}

func TestTakeFlashWithoutACookie(t *testing.T) {
	if got := TakeFlash(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Errorf("flash = %q", got)
	}
}

func TestFlashSurvivesEscaping(t *testing.T) {
	recorder := apply(t, To("/thanks").Flash("a b&c"), nil)
	cookie := recorder.Result().Cookies()[0]
	request := httptest.NewRequest(http.MethodGet, "/thanks", nil)
	request.AddCookie(cookie)
	if got := TakeFlash(httptest.NewRecorder(), request); got != "a b&c" {
		t.Errorf("flash = %q", got)
	}
}

func TestAMalformedFlashIsDropped(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/thanks", nil)
	request.AddCookie(&http.Cookie{Name: FlashCookie, Value: "%zz"})
	if got := TakeFlash(httptest.NewRecorder(), request); got != "" {
		t.Errorf("flash = %q", got)
	}
}

func TestARedirectOffSiteIsRefused(t *testing.T) {
	for _, target := range []string{"//evil.com", "/\\evil.com", "javascript:alert(1)", ""} {
		err := To(target).Apply(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), resolver)
		if target == "" {
			if !errors.Is(err, ErrNoTarget) {
				t.Errorf("%q: err = %v", target, err)
			}
			continue
		}
		if !errors.Is(err, ErrTarget) {
			t.Errorf("%q: err = %v, want the target refused", target, err)
		}
	}
}

func TestARedirectToAnotherSiteIsAllowed(t *testing.T) {
	recorder := apply(t, To("https://payments.test/checkout"), nil)
	if got := recorder.Header().Get("Location"); got != "https://payments.test/checkout" {
		t.Errorf("location = %q", got)
	}
}

func TestAResolvedRouteThatEscapesIsRefused(t *testing.T) {
	escape := func(string, map[string]string) (string, error) { return "//evil.com", nil }
	err := Route("anything").Apply(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), escape)
	if !errors.Is(err, ErrTarget) {
		t.Errorf("err = %v, want a resolver checked like any other target", err)
	}
}

func TestHasFlashSeesTheCookieTakeFlashWouldRead(t *testing.T) {
	bare := httptest.NewRequest(http.MethodGet, "/", nil)
	if HasFlash(bare) {
		t.Error("no cookie, no flash")
	}
	held := httptest.NewRequest(http.MethodGet, "/", nil)
	held.AddCookie(&http.Cookie{Name: FlashCookie, Value: "sent"})
	if !HasFlash(held) {
		t.Error("a flash cookie must be visible without consuming it")
	}
	if got := TakeFlash(httptest.NewRecorder(), held); got != "sent" {
		t.Errorf("flash = %q, want HasFlash to leave it alone", got)
	}
}
