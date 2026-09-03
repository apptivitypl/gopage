package smoke

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type formServer struct {
	token   string
	flash   string
	noToken bool
	status  int
}

func (f *formServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		f.post(w, r)
		return
	}
	if !f.noToken {
		http.SetCookie(w, &http.Cookie{Name: "rill.csrf", Value: f.token, Path: "/"})
	}
	page := `<form method="post">`
	if !f.noToken {
		page += `<input type="hidden" name="__csrf" value="` + f.token + `">`
	}
	if f.flash != "" {
		page += `<p class="flash">` + f.flash + `</p>`
		f.flash = ""
	}
	_, _ = w.Write([]byte(page + "</form>"))
}

func (f *formServer) post(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.PostFormValue("__csrf") != f.token {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.PostFormValue("Email") == "nope" {
		w.WriteHeader(f.rejectStatus())
		_, _ = w.Write([]byte(`<p class="field-error">no</p><input value="A">`))
		return
	}
	f.flash = "thanks, " + FlashText
	http.Redirect(w, r, FormPath, http.StatusSeeOther)
}

func (f *formServer) rejectStatus() int {
	if f.status != 0 {
		return f.status
	}
	return http.StatusUnprocessableEntity
}

func serve(t *testing.T, handler http.Handler) (*http.Client, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client, server.URL
}

func TestRunFormWalksTheWholeCycle(t *testing.T) {
	client, base := serve(t, &formServer{token: "tok"})
	if err := RunForm(client, base); err != nil {
		t.Errorf("RunForm: %v", err)
	}
}

func TestRunFormNeedsAToken(t *testing.T) {
	client, base := serve(t, &formServer{token: "tok", noToken: true})
	if err := RunForm(client, base); err == nil || !strings.Contains(err.Error(), "__csrf") {
		t.Errorf("err = %v", err)
	}
}

func TestRunFormRequires422(t *testing.T) {
	client, base := serve(t, &formServer{token: "tok", status: http.StatusOK})
	if err := RunForm(client, base); err == nil || !strings.Contains(err.Error(), "want 422") {
		t.Errorf("err = %v", err)
	}
}

func TestRunFormReportsAnUnreachableServer(t *testing.T) {
	if err := RunForm(&http.Client{}, "http://127.0.0.1:1"); err == nil {
		t.Error("an unreachable server must be reported")
	}
}

type cacheServer struct {
	calls  int
	always string
}

func (c *cacheServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	c.calls++
	status := c.always
	if status == "" {
		status = "hit"
		if c.calls == 1 {
			status = "miss"
		}
	}
	w.Header().Set(CacheHeader, status)
	_, _ = w.Write([]byte("page"))
}

func cacheFetcher(t *testing.T, handler http.Handler) (Fetcher, string) {
	t.Helper()
	client, base := serve(t, handler)
	return HTTPFetcher(client), base
}

func TestRunCacheWantsTheSecondReadCached(t *testing.T) {
	fetch, base := cacheFetcher(t, &cacheServer{})
	if err := RunCache(fetch, base); err != nil {
		t.Errorf("RunCache: %v", err)
	}
}

func TestRunCacheRejectsAServerThatNeverCaches(t *testing.T) {
	fetch, base := cacheFetcher(t, &cacheServer{always: "miss"})
	if err := RunCache(fetch, base); err == nil || !strings.Contains(err.Error(), "want a hit") {
		t.Errorf("err = %v", err)
	}
}

func TestRunCacheRejectsAMissingHeader(t *testing.T) {
	fetch, base := cacheFetcher(t, &cacheServer{always: "-"})
	if err := RunCache(fetch, base); err == nil || !strings.Contains(err.Error(), "want a miss or a hit") {
		t.Errorf("err = %v", err)
	}
}

func TestRunCacheReportsAnUnreachableServer(t *testing.T) {
	fetch := HTTPFetcher(&http.Client{})
	if err := RunCache(fetch, "http://127.0.0.1:1"); err == nil {
		t.Error("an unreachable server must be reported")
	}
}

type partialServer struct {
	contentType string
	level       string
	body        string
}

func (p *partialServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(PartialHeader) == "" {
		_, _ = w.Write([]byte("<!doctype html><html><body>full</body></html>"))
		return
	}
	w.Header().Set("Content-Type", p.contentType)
	w.Header().Set(LevelHeader, p.level)
	_, _ = w.Write([]byte(p.body))
}

func TestRunPartialAcceptsAProperFragment(t *testing.T) {
	client, base := serve(t, &partialServer{contentType: PartialType, level: "1", body: "<h1>features</h1>"})
	if err := RunPartial(client, base); err != nil {
		t.Errorf("RunPartial: %v", err)
	}
}

func TestRunPartialRejectsWhatIsNotAPartial(t *testing.T) {
	cases := map[string]*partialServer{
		"wrong type":     {contentType: "text/html", level: "1", body: "<h1>x</h1>"},
		"nothing kept":   {contentType: PartialType, level: "0", body: "<h1>x</h1>"},
		"whole document": {contentType: PartialType, level: "1", body: "<!doctype html><h1>x</h1>"},
		"no page at all": {contentType: PartialType, level: "1", body: "<p>x</p>"},
	}
	for name, server := range cases {
		t.Run(name, func(t *testing.T) {
			client, base := serve(t, server)
			if err := RunPartial(client, base); err == nil {
				t.Error("expected the check to fail")
			}
		})
	}
}

func TestRunPartialReportsAnUnreachableServer(t *testing.T) {
	if err := RunPartial(&http.Client{}, "http://127.0.0.1:1"); err == nil {
		t.Error("an unreachable server must be reported")
	}
}

type referenceServer struct{ body string }

func (r *referenceServer) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/api/health":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","runtime":"go"}`))
	case FeedPath:
		w.Header().Set("Content-Type", FeedType)
		_, _ = w.Write([]byte("event: item\ndata: {}\n\nevent: done\ndata: \n\n"))
	case "/sitemap.xml":
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte("<loc>http://x/</loc>"))
	case "/robots.txt":
		_, _ = w.Write([]byte("Sitemap: http://x/sitemap.xml"))
	case "/en":
		http.Redirect(w, request, "/", http.StatusMovedPermanently)
	case "/items/99":
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<title>page not found</title>"))
	case "/items/2":
		_, _ = w.Write([]byte("flat by the river 14 354.00 PLN per m2"))
	case "/pl":
		_, _ = w.Write([]byte("4 oferty aktywna"))
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if request.URL.RawQuery != "" {
			_, _ = w.Write([]byte(strings.Replace(r.body, "4 listings", "2 listings", 1)))
			return
		}
		_, _ = w.Write([]byte(r.body))
	}
}

func referenceBody(count string) string {
	return "what is on offer " + count + ` listings <span class="item-price">410 000.00 PLN</span>` +
		`<rill-island></rill-island><link rel="alternate" hreflang="pl" href="/pl">`
}

func TestTheReferenceChecksPassAgainstAMatchingServer(t *testing.T) {
	client, base := serve(t, &referenceServer{body: referenceBody("4")})
	fetch := HTTPFetcher(client)
	if err := RunReference(fetch, base); err != nil {
		t.Errorf("RunReference: %v", err)
	}
}

func TestTheReferenceChecksCatchAMissingPiece(t *testing.T) {
	client, base := serve(t, &referenceServer{body: "what is on offer 4 listings"})
	if err := RunReference(HTTPFetcher(client), base); err == nil {
		t.Error("a page without the island must fail the reference run")
	}
}

func TestTwoAdaptersMustAgree(t *testing.T) {
	left, leftBase := serve(t, &referenceServer{body: referenceBody("4")})
	right, rightBase := serve(t, &referenceServer{body: referenceBody("4")})
	if err := CompareAdapters(HTTPFetcher(left), HTTPFetcher(right), leftBase, rightBase); err != nil {
		t.Errorf("CompareAdapters: %v", err)
	}

	other, otherBase := serve(t, &referenceServer{body: referenceBody("5")})
	if err := CompareAdapters(HTTPFetcher(left), HTTPFetcher(other), leftBase, otherBase); err == nil {
		t.Error("two adapters answering differently must fail")
	}
}

func TestComparingAgainstAnUnreachableAdapter(t *testing.T) {
	client, base := serve(t, &referenceServer{body: referenceBody("4")})
	dead := HTTPFetcher(&http.Client{})
	if err := CompareAdapters(HTTPFetcher(client), dead, base, "http://127.0.0.1:1"); err == nil {
		t.Error("an unreachable adapter must be reported")
	}
	if err := CompareAdapters(dead, HTTPFetcher(client), "http://127.0.0.1:1", base); err == nil {
		t.Error("an unreachable adapter must be reported")
	}
	if err := RunReference(dead, "http://127.0.0.1:1"); err == nil {
		t.Error("an unreachable server must be reported")
	}
}

func TestAStatusMismatchBetweenAdaptersIsReported(t *testing.T) {
	client, base := serve(t, &referenceServer{body: referenceBody("4")})
	other, otherBase := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	if err := CompareAdapters(HTTPFetcher(client), HTTPFetcher(other), base, otherBase); err == nil {
		t.Error("a status mismatch must be reported")
	}
}
