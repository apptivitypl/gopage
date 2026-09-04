package smoke

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Check struct {
	Path        string
	Status      int
	Contains    string
	ContentType string
	Header      string
	HeaderValue string
	HeaderAny   bool
}

func Checks() []Check {
	return []Check{
		{Path: "/", Status: http.StatusOK, Contains: "Rendered in Go.", ContentType: "text/html"},
		{Path: "/", Status: http.StatusOK, Contains: `<link rel="stylesheet" href="/assets/`},
		{Path: "/", Status: http.StatusOK, Contains: `<link rel="icon" href="/favicon.ico"`},
		{Path: "/", Status: http.StatusOK, Contains: "<title>Rendered in Go. Interactive in React.</title>"},
		{Path: "/", Status: http.StatusOK, Contains: `<link rel="canonical" href="http://`},
		{Path: "/", Status: http.StatusOK, Contains: `class="hacker-news-title"`},
		{Path: "/", Status: http.StatusOK, Contains: `<script type="module" async src="/assets/gopage.client.`},
		{Path: "/", Status: http.StatusOK, Contains: "<!--gopage:o0-->"},
		{Path: "/", Status: http.StatusOK, Contains: "<main", ContentType: "text/html"},
		{Path: "/", Status: http.StatusOK, Header: "Vary", HeaderValue: "GOPAGE-Fragment, GOPAGE-Partial"},
		{Path: "/", Status: http.StatusOK, Contains: `class="mark`},
		{Path: "/", Status: http.StatusOK, Contains: `<gopage-island style="display:contents" name="Ticker"`},
		{Path: "/", Status: http.StatusOK, Contains: `class="ln"`},
		{Path: "/", Status: http.StatusOK, Contains: `class="panel-pill"`},
		{Path: "/", Status: http.StatusOK, Contains: `class="panel-mode"`},
		{Path: "/", Status: http.StatusOK, Contains: `class="hacker-news-logo"`},
		{Path: "/", Status: http.StatusOK, Contains: `class="foot-row"`},
		{Path: "/", Status: http.StatusOK, Contains: `<gopage-island style="display:contents" name="Stars" strategy="idle"`},
		{Path: "/", Status: http.StatusOK, Contains: `<gopage-island style="display:contents" name="Response"`},
		{Path: "/", Status: http.StatusOK, Contains: `<link rel="modulepreload" href="/assets/island.`},
		{Path: "/fonts/jetbrains-mono-latin.woff2", Status: http.StatusOK},
		{Path: "/", Status: http.StatusOK, Contains: `rel="preload" href="/fonts/`},
		{Path: "/api/health", Status: http.StatusOK, Contains: `"runtime":"go"`, ContentType: "application/json"},
		{Path: "/favicon.ico", Status: http.StatusOK, ContentType: "icon"},
		{Path: "/logo.svg", Status: http.StatusOK, ContentType: "image/svg+xml"},
		{Path: "/nope", Status: http.StatusNotFound, Contains: "<title>no route answers this address</title>", ContentType: "text/html"},
		{Path: "/nope", Status: http.StatusNotFound, Contains: "the router walked the manifest"},
		{Path: "/nope", Status: http.StatusNotFound, Contains: "place-items-center"},
		{Path: "/sitemap.xml", Status: http.StatusOK, Contains: "<loc>http://", ContentType: "application/xml"},
		{Path: "/robots.txt", Status: http.StatusOK, Contains: "Sitemap: http://", ContentType: "text/plain"},
		{Path: "/llms.txt", Status: http.StatusOK, Contains: "## Pages", ContentType: "text/plain"},
		{Path: "/api/stories", Status: http.StatusOK, ContentType: "application/json", Contains: `"title"`},
	}
}

const EventType = "text/event-stream"

type Response struct {
	Status      int
	Body        string
	ContentType string
	Headers     http.Header
}

func Verify(check Check, res Response) error {
	if res.Status != check.Status {
		return fmt.Errorf("%s: status %d, want %d", check.Path, res.Status, check.Status)
	}
	if check.Contains != "" && !strings.Contains(res.Body, check.Contains) {
		return fmt.Errorf("%s: body does not contain %q", check.Path, check.Contains)
	}
	if check.ContentType != "" && !strings.Contains(res.ContentType, check.ContentType) {
		return fmt.Errorf("%s: content type %q, want %q", check.Path, res.ContentType, check.ContentType)
	}
	if check.Header == "" {
		return nil
	}
	got := res.Headers.Get(check.Header)
	if check.HeaderAny && got == "" {
		return fmt.Errorf("%s: header %s is missing", check.Path, check.Header)
	}
	if !check.HeaderAny && got != check.HeaderValue {
		return fmt.Errorf("%s: header %s is %q, want %q", check.Path, check.Header, got, check.HeaderValue)
	}
	return nil
}

func FreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("listener did not report a tcp address")
	}
	return addr.Port, nil
}

type Fetcher func(url string) (Response, error)

func HTTPFetcher(client *http.Client) Fetcher {
	return func(url string) (Response, error) {
		res, err := client.Get(url)
		if err != nil {
			return Response{}, err
		}
		defer func() { _ = res.Body.Close() }()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return Response{}, err
		}
		return Response{
			Status:      res.StatusCode,
			Body:        string(body),
			ContentType: res.Header.Get("Content-Type"),
			Headers:     res.Header,
		}, nil
	}
}

func WaitReady(fetch Fetcher, base string, attempts int, pause time.Duration) error {
	var last error
	for range attempts {
		res, err := fetch(base + "/")
		if err == nil && res.Status > 0 {
			return nil
		}
		last = err
		time.Sleep(pause)
	}
	if last == nil {
		last = errors.New("no response")
	}
	return fmt.Errorf("server never became ready: %w", last)
}

func Run(fetch Fetcher, base string) error {
	for _, check := range Checks() {
		res, err := fetch(base + check.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", check.Path, err)
		}
		if err := Verify(check, res); err != nil {
			return err
		}
	}
	return nil
}

const WorkerSizeBudget = 3 << 20

func CheckSize(compressed int) error {
	if compressed > WorkerSizeBudget {
		return fmt.Errorf("worker module is %d bytes compressed, over the %d byte budget",
			compressed, WorkerSizeBudget)
	}
	return nil
}
