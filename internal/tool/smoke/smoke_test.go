package smoke

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChecksCoverStaticDynamicAndMissing(t *testing.T) {
	paths := map[string]bool{}
	for _, check := range Checks() {
		paths[check.Path] = true
	}
	for _, want := range []string{"/", "/api/health", "/api/stories", "/favicon.ico", "/nope"} {
		if !paths[want] {
			t.Errorf("the smoke suite does not probe %q", want)
		}
	}
}

func TestVerifyAcceptsAMatchingResponse(t *testing.T) {
	check := Check{Path: "/", Status: 200, Contains: "hello", ContentType: "text/html"}
	res := Response{Status: 200, Body: "<p>hello</p>", ContentType: "text/html; charset=utf-8"}
	if err := Verify(check, res); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestVerifyReportsEachMismatch(t *testing.T) {
	check := Check{Path: "/", Status: 200, Contains: "hello", ContentType: "text/html"}
	cases := map[string]Response{
		"status":       {Status: 500, Body: "hello", ContentType: "text/html"},
		"contain":      {Status: 200, Body: "nope", ContentType: "text/html"},
		"content type": {Status: 200, Body: "hello", ContentType: "application/json"},
	}
	for want, res := range cases {
		err := Verify(check, res)
		if err == nil {
			t.Fatalf("%s mismatch was accepted", want)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention the %s", err, want)
		}
	}
}

func TestVerifySkipsEmptyExpectations(t *testing.T) {
	if err := Verify(Check{Path: "/x", Status: 404}, Response{Status: 404, Body: "anything"}); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestFreePortIsUsable(t *testing.T) {
	port, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("port = %d", port)
	}
}

func TestRunAgainstAServerThatSatisfiesEveryCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Vary", "GOPAGE-Fragment, GOPAGE-Partial")
			_, _ = w.Write([]byte(`<link rel="stylesheet" href="/assets/app.ABC.css"><link rel="icon" href="/favicon.ico" sizes="32x32"><link rel="preload" href="/fonts/jetbrains-mono-latin.woff2" as="font" type="font/woff2" crossorigin><title>Rendered in Go. Interactive in React.</title><h1>Rendered in Go. Interactive in React.</h1><link rel="canonical" href="http://x/"><span class="mark"></span><gopage-island style="display:contents" name="Ticker"></gopage-island><span class="ln"> 1</span><span class="panel-pill"></span><label class="panel-mode"></label><svg class="hacker-news-logo"></svg><div class="foot-row"></div><h3 class="hacker-news-title"></h3><gopage-island style="display:contents" name="Stars" strategy="idle"></gopage-island><gopage-island style="display:contents" name="Response"></gopage-island><link rel="modulepreload" href="/assets/island.HELPER.js"><script type="module" async src="/assets/gopage.client.ABC.js"></script><!--gopage:o0-->the head was already flushed<main>`))
		case "/api/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","runtime":"go"}`))
		case "/about":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<title>what happened to this request</title>what happened to this request`))
		case "/pl/nope":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("router przeszedł manifest"))
		case "/pl":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`silnik szablonów pracuje w buildzie wyspa`))
		case "/api/stories":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"title":"a story","score":1}]`))
		case "/fonts/jetbrains-mono-latin.woff2":
			w.Header().Set("Content-Type", "font/woff2")
			_, _ = w.Write([]byte("font"))
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/vnd.microsoft.icon")
			_, _ = w.Write([]byte("icon"))
		case "/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte("<svg/>"))
		case "/en/about":
			http.Redirect(w, r, "/about", http.StatusMovedPermanently)
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<loc>http://x/</loc><xhtml:link hreflang="pl"/>`))
		case "/llms.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("# demo\n\n## Pages\n"))
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("Sitemap: http://x/sitemap.xml"))
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<title>no route answers this address</title><main class="place-items-center">the router walked the manifest and found nothing</main>`))
		}
	}))
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if err := Run(HTTPFetcher(client), server.URL); err != nil {
		t.Errorf("Run: %v", err)
	}
}

func TestRunFailsWhenTheApiIsServedByTheWrongThing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","runtime":"javascript"}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("GOPAGE-Cache", "hit")
		_, _ = w.Write([]byte(`<link rel="stylesheet" href="/assets/app.ABC.css"><link rel="icon" href="/favicon.ico" sizes="32x32"><link rel="preload" href="/fonts/jetbrains-mono-latin.woff2" as="font" type="font/woff2" crossorigin><title>Rendered in Go. Interactive in React.</title><h1>Rendered in Go. Interactive in React.</h1><link rel="canonical" href="http://x/"><span class="mark"></span><gopage-island style="display:contents" name="Ticker"></gopage-island><span class="ln"> 1</span><gopage-island style="display:contents" name="HackerNews"></gopage-island><script type="module" async src="/assets/gopage.client.ABC.js"></script><!--gopage:o0-->the head was already flushed<main>`))
	}))
	defer server.Close()

	fetch := HTTPFetcher(server.Client())
	response, err := fetch(server.URL + "/api/health")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	check := Check{Path: "/api/health", Status: http.StatusOK, Contains: `"runtime":"go"`}
	if err := Verify(check, response); err == nil {
		t.Error("an api answered by javascript must fail the check")
	}
}

func TestRunReportsTransportErrors(t *testing.T) {
	fetch := func(string) (Response, error) { return Response{}, errors.New("connection refused") }
	if err := Run(fetch, "http://127.0.0.1:1"); err == nil {
		t.Error("a transport error must fail the smoke run")
	}
}

func TestWaitReadyStopsAtTheFirstResponse(t *testing.T) {
	var calls int
	fetch := func(string) (Response, error) {
		calls++
		if calls < 3 {
			return Response{}, errors.New("not up yet")
		}
		return Response{Status: 200}, nil
	}
	if err := WaitReady(fetch, "http://x", 10, time.Millisecond); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want it to stop as soon as the server answers", calls)
	}
}

func TestWaitReadyGivesUp(t *testing.T) {
	fetch := func(string) (Response, error) { return Response{}, errors.New("down") }
	err := WaitReady(fetch, "http://x", 2, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "never became ready") {
		t.Errorf("err = %v", err)
	}
}

func TestSizeBudget(t *testing.T) {
	if err := CheckSize(WorkerSizeBudget); err != nil {
		t.Errorf("a module exactly at the budget must pass: %v", err)
	}
	err := CheckSize(WorkerSizeBudget + 1)
	if err == nil || !strings.Contains(err.Error(), "budget") {
		t.Errorf("err = %v, want the budget named", err)
	}
}
