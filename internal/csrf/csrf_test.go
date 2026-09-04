package csrf

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/cookie"
)

type failingSource struct{}

func (failingSource) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func TestGenerateProducesDistinctTokens(t *testing.T) {
	first, err := Generate(nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	second, _ := Generate(nil)
	if first == second || len(first) < 40 {
		t.Errorf("tokens = %q, %q", first, second)
	}
}

func TestGenerateReportsAFailingSource(t *testing.T) {
	if _, err := Generate(failingSource{}); err == nil {
		t.Error("a source that cannot deliver must be reported")
	}
}

func TestIssueSetsACookieOnce(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/apply", nil)
	recorder := httptest.NewRecorder()
	token, err := Issue(recorder, request, nil)
	if err != nil || token == "" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
	held := recorder.Header().Get("Set-Cookie")
	if !strings.Contains(held, CookieName+"=") || !strings.Contains(held, "HttpOnly") {
		t.Errorf("cookie = %q", held)
	}
	if !strings.Contains(held, "SameSite=Lax") || strings.Contains(held, "Secure") {
		t.Errorf("cookie = %q", held)
	}
	if strings.Contains(held, CookieName+"="+token) {
		t.Errorf("cookie = %q, want the secret kept out of the rendered token", held)
	}

	back := httptest.NewRequest(http.MethodPost, "/apply", nil)
	back.Header.Set("Cookie", held)
	if err := Verify(back, token); err != nil {
		t.Errorf("the rendered token does not verify against its own cookie: %v", err)
	}
}

func TestIssueReusesAnExistingToken(t *testing.T) {
	existing, err := Generate(nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/apply", nil)
	request.AddCookie(&http.Cookie{Name: CookieName, Value: existing})
	recorder := httptest.NewRecorder()
	token, err := Issue(recorder, request, nil)
	if err != nil || token == "" || token == existing {
		t.Fatalf("token = %q, err = %v", token, err)
	}
	if verifyErr := Verify(request, token); verifyErr != nil {
		t.Errorf("the reissued mask does not verify: %v", verifyErr)
	}
	if recorder.Header().Get("Set-Cookie") != "" {
		t.Errorf("an existing token is not reissued: %q", recorder.Header().Get("Set-Cookie"))
	}
}

func TestIssueMarksTheCookieSecureOverTls(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.com/apply", nil)
	recorder := httptest.NewRecorder()
	if _, err := Issue(recorder, request, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recorder.Header().Get("Set-Cookie"), "Secure") {
		t.Errorf("cookie = %q", recorder.Header().Get("Set-Cookie"))
	}
}

func TestIssueReportsAFailingSource(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/apply", nil)
	if _, err := Issue(httptest.NewRecorder(), request, failingSource{}); err == nil {
		t.Error("a source that cannot deliver must be reported")
	}
}

func TestVerify(t *testing.T) {
	secret, err := Generate(nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	masked, err := Mask(secret, nil)
	if err != nil {
		t.Fatalf("Mask: %v", err)
	}
	other, _ := Generate(nil)
	elsewhere, _ := Mask(other, nil)

	cases := []struct {
		name      string
		cookie    string
		submitted string
		ok        bool
	}{
		{"match", secret, masked, true},
		{"mismatch", secret, elsewhere, false},
		{"the bare secret", secret, secret, false},
		{"no cookie", "", masked, false},
		{"no field", secret, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/apply", nil)
			if c.cookie != "" {
				request.AddCookie(&http.Cookie{Name: CookieName, Value: c.cookie})
			}
			err := Verify(request, c.submitted)
			if (err == nil) != c.ok {
				t.Errorf("err = %v, want ok = %v", err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrRejected) {
				t.Errorf("err = %v, want ErrRejected", err)
			}
		})
	}
}

func TestTokenWithoutACookie(t *testing.T) {
	if got := Token(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Errorf("token = %q", got)
	}
}

func TestTheRenderedTokenDiffersEveryTime(t *testing.T) {
	secret, err := Generate(nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	seen := map[string]bool{}
	for range 20 {
		masked, err := Mask(secret, nil)
		if err != nil {
			t.Fatalf("Mask: %v", err)
		}
		if masked == secret {
			t.Fatal("the rendered token is the secret itself")
		}
		if seen[masked] {
			t.Fatalf("the same masked token came back twice: %q", masked)
		}
		seen[masked] = true
	}
}

func TestEveryMaskOfTheSameSecretVerifies(t *testing.T) {
	secret, err := Generate(nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.AddCookie(&http.Cookie{Name: cookie.HostPrefix + CookieName, Value: secret})
	request = request.WithContext(cookie.With(request.Context(), cookie.Options{Secure: true}))

	for range 10 {
		masked, err := Mask(secret, nil)
		if err != nil {
			t.Fatalf("Mask: %v", err)
		}
		if err := Verify(request, masked); err != nil {
			t.Errorf("Verify(%q) = %v", masked, err)
		}
	}
}

func TestAMaskOfAnotherSecretIsRejected(t *testing.T) {
	mine, _ := Generate(nil)
	theirs, _ := Generate(nil)
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.AddCookie(&http.Cookie{Name: cookie.HostPrefix + CookieName, Value: mine})
	request = request.WithContext(cookie.With(request.Context(), cookie.Options{Secure: true}))

	masked, err := Mask(theirs, nil)
	if err != nil {
		t.Fatalf("Mask: %v", err)
	}
	if err := Verify(request, masked); err == nil {
		t.Error("a mask of somebody else's secret must be rejected")
	}
	if err := Verify(request, mine); err == nil {
		t.Error("the bare secret must not pass as a rendered token")
	}
}

func TestAMalformedMaskIsRejected(t *testing.T) {
	secret, _ := Generate(nil)
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.AddCookie(&http.Cookie{Name: cookie.HostPrefix + CookieName, Value: secret})
	request = request.WithContext(cookie.With(request.Context(), cookie.Options{Secure: true}))

	for _, bad := range []string{"!!!!", "AAA", ""} {
		if err := Verify(request, bad); err == nil {
			t.Errorf("Verify(%q) accepted a malformed token", bad)
		}
	}
	if _, err := Mask("!!!!", nil); err == nil {
		t.Error("masking a token that is not base64 must be reported")
	}
	if _, err := Mask("", nil); err == nil {
		t.Error("masking an empty token must be reported")
	}
}

func TestMaskReportsAFailingSource(t *testing.T) {
	secret, _ := Generate(nil)
	if _, err := Mask(secret, failingSource{}); err == nil {
		t.Error("a source that cannot deliver must be reported")
	}
}

func TestVerifyRejectsACookieThatIsNotBase64(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/apply", nil)
	request.AddCookie(&http.Cookie{Name: CookieName, Value: "!!!!"})
	if err := Verify(request, "AAAA"); err == nil {
		t.Error("a cookie that is not a token must be rejected")
	}
}
