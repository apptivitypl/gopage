package devserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sonquer/rill/internal/diag"
)

func page(text string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(text))
	})
}

func get(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	return recorder
}

func TestAGoodBuildIsServed(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return page("hello"), nil, nil, nil
	}, nil)
	if !server.Rebuild() || server.Broken() {
		t.Fatal("the first build must succeed")
	}
	if got := get(t, server); got.Body.String() != "hello" || got.Code != http.StatusOK {
		t.Errorf("status = %d, body = %q", got.Code, got.Body.String())
	}
}

func TestNothingBuiltYetShowsTheOverlay(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return page("x"), nil, nil, nil
	}, nil)
	got := get(t, server)
	if got.Code != http.StatusInternalServerError || !strings.Contains(got.Body.String(), "nothing has been built") {
		t.Errorf("status = %d, body = %q", got.Code, got.Body.String())
	}
}

func TestABrokenBuildKeepsTheServerUp(t *testing.T) {
	broken := false
	logged := 0
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		if broken {
			items := []diag.Diagnostic{diag.New(diag.C201, "app/page.rill", diag.Span{Start: 3, End: 5}, "expected a value")}
			return nil, items, map[string]string{"app/page.rill": "{% if %}"}, errors.New("build stopped")
		}
		return page("hello"), nil, nil, nil
	}, func(string, ...any) { logged++ })

	server.Rebuild()
	broken = true
	if server.Rebuild() {
		t.Fatal("a broken build must report failure")
	}
	if logged != 1 {
		t.Errorf("logged = %d, want the failure reported once", logged)
	}
	if !server.Broken() {
		t.Error("the server must know it is broken")
	}
	got := get(t, server)
	if got.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", got.Code)
	}
	for _, want := range []string{"RILL-C201", "expected a value", "app/page.rill", "no-store"} {
		if want == "no-store" {
			if got.Header().Get("Cache-Control") != want {
				t.Errorf("cache control = %q", got.Header().Get("Cache-Control"))
			}
			continue
		}
		if !strings.Contains(got.Body.String(), want) {
			t.Errorf("overlay = %q, want %q", got.Body.String(), want)
		}
	}

	broken = false
	if !server.Rebuild() || server.Broken() {
		t.Fatal("the server must recover")
	}
	if body := get(t, server).Body.String(); body != "hello" {
		t.Errorf("body = %q", body)
	}
}

func TestWarningsAloneDoNotBlockThePage(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return page("hello"), []diag.Diagnostic{diag.Warn(diag.W703, "app/page.rill", diag.Span{}, "careful")}, nil, nil
	}, nil)
	server.Rebuild()
	if server.Broken() {
		t.Error("a warning is not a broken build")
	}
	if got := get(t, server); got.Body.String() != "hello" {
		t.Errorf("body = %q", got.Body.String())
	}
}

func TestTheOverlayEscapesTheSource(t *testing.T) {
	items := []diag.Diagnostic{diag.New(diag.C201, "app/page.rill", diag.Span{Start: 0, End: 5}, "bad <script>")}
	recorder := httptest.NewRecorder()
	Overlay(recorder, items, map[string]string{"app/page.rill": "<script>alert(1)</script>"})
	body := recorder.Body.String()
	if strings.Contains(body, "<script>alert") {
		t.Errorf("overlay = %q, want the source escaped", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("overlay = %q", body)
	}
}

func TestTheWatcherReportsChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed := make(chan string, 1)
	done := make(chan struct{})
	defer close(done)
	if err := Watch(dir, changed, done); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "page.rill"), []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("a change in app/ must wake the watcher")
	}
}

func TestTheWatcherFollowsNewDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed := make(chan string, 1)
	done := make(chan struct{})
	defer close(done)
	if err := Watch(dir, changed, done); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "app", "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	<-changed
	if err := os.WriteFile(filepath.Join(dir, "app", "docs", "page.rill"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("a file in a new directory must wake the watcher")
	}
}

func TestGeneratedDirectoriesAreNotWatched(t *testing.T) {
	for _, name := range ignored {
		if !skip(filepath.Join("project", name)) {
			t.Errorf("%s must be skipped", name)
		}
	}
	if skip(filepath.Join("project", "app")) {
		t.Error("app must be watched")
	}
}

func TestWatchingAMissingProjectIsHarmless(t *testing.T) {
	changed := make(chan string, 1)
	done := make(chan struct{})
	defer close(done)
	if err := Watch(filepath.Join(t.TempDir(), "gone"), changed, done); err != nil {
		t.Errorf("Watch: %v", err)
	}
}

func TestConcurrentReadsSeeAConsistentBuild(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return page("hello"), nil, nil, nil
	}, nil)
	server.Rebuild()

	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 20 {
				server.Rebuild()
				get(t, server)
			}
		}()
	}
	group.Wait()
}

func TestTheOverlayWithoutDiagnostics(t *testing.T) {
	recorder := httptest.NewRecorder()
	Overlay(recorder, nil, nil)
	if !strings.Contains(recorder.Body.String(), "nothing has been built") {
		t.Errorf("overlay = %q", recorder.Body.String())
	}
}

func TestTheOverlayListsOnlyErrors(t *testing.T) {
	items := []diag.Diagnostic{
		diag.Warn(diag.W703, "app/page.rill", diag.Span{}, "careful"),
		diag.New(diag.C201, "app/page.rill", diag.Span{}, "broken"),
	}
	recorder := httptest.NewRecorder()
	Overlay(recorder, items, nil)
	body := recorder.Body.String()
	if strings.Contains(body, "careful") || !strings.Contains(body, "broken") {
		t.Errorf("overlay = %q", body)
	}
}

func TestWatchingAFileInsteadOfATreeIsHarmless(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app")
	if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := make(chan string, 1)
	done := make(chan struct{})
	defer close(done)
	if err := Watch(file, changed, done); err != nil {
		t.Errorf("Watch: %v", err)
	}
}

func TestTheWatcherStopsWhenItIsToldTo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed := make(chan string, 1)
	done := make(chan struct{})
	if err := Watch(dir, changed, done); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	close(done)
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "app", "page.rill"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Error("a stopped watcher must not report anything")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestTheReloadScriptIsInjectedIntoPages(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Length", "26")
			_, _ = w.Write([]byte("<html><body>hi</body></html>"))
		}), nil, nil, nil
	}, nil)
	server.Rebuild()
	got := get(t, server)
	if !strings.Contains(got.Body.String(), ReloadPath) {
		t.Errorf("body = %q, want the reload script", got.Body.String())
	}
	if !strings.Contains(got.Body.String(), "</script></body>") {
		t.Errorf("body = %q, want the script before the closing tag", got.Body.String())
	}
	if got.Header().Get("Content-Length") != "" {
		t.Error("the length must be dropped once the body grows")
	}
}

func TestTheReloadScriptIsNotInjectedIntoData(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}), nil, nil, nil
	}, nil)
	server.Rebuild()
	if body := get(t, server).Body.String(); body != `{"status":"ok"}` {
		t.Errorf("body = %q", body)
	}
}

func TestABodyWithoutAClosingTagStillGetsTheScript(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/html")
	wrapped := &injector{ResponseWriter: recorder}
	if _, err := wrapped.Write([]byte("<p>x</p>")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	wrapped.finish()
	if got := recorder.Body.String(); !strings.HasSuffix(got, ReloadScript) {
		t.Errorf("body = %q", got)
	}
}

func TestTheScriptGoesBeforeTheClosingBodyAcrossWrites(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/html")
	wrapped := &injector{ResponseWriter: recorder}
	for _, chunk := range []string{"<html><body><p>a</p>", "<p>b</p></bo", "dy></html>"} {
		if _, err := wrapped.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	wrapped.finish()

	body := recorder.Body.String()
	if want := "<p>b</p>" + ReloadScript + "</body></html>"; !strings.HasSuffix(body, want) {
		t.Errorf("body = %q, want the script before the closing tag", body)
	}
	if strings.Count(body, ReloadScript) != 1 {
		t.Errorf("body = %q, want the script once", body)
	}
	if !strings.HasPrefix(body, "<html><body><p>a</p>") {
		t.Errorf("body = %q, want the first chunk untouched", body)
	}
}

func TestAWriteReportsEveryByteItWasGiven(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/html")
	wrapped := &injector{ResponseWriter: recorder}
	chunk := []byte("<html><body>partial")
	written, err := wrapped.Write(chunk)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if written != len(chunk) {
		t.Errorf("written = %d, want %d so the caller sees a complete write", written, len(chunk))
	}
	wrapped.finish()
}

func TestTheInjectorPassesAFlushThrough(t *testing.T) {
	recorder := httptest.NewRecorder()
	wrapped := &injector{ResponseWriter: recorder}
	wrapped.Flush()
	if !recorder.Flushed {
		t.Error("a dev response must still stream")
	}
}

func TestTheOverlayReloadsItself(t *testing.T) {
	recorder := httptest.NewRecorder()
	Overlay(recorder, nil, nil)
	if !strings.Contains(recorder.Body.String(), ReloadPath) {
		t.Errorf("overlay = %q, want the reload script", recorder.Body.String())
	}
}

func TestARebuildWakesEveryWaitingBrowser(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return page("hello"), nil, nil, nil
	}, nil)
	first, firstChannel := server.clients.join()
	second, secondChannel := server.clients.join()
	defer server.clients.leave(first)
	defer server.clients.leave(second)

	if woken := server.clients.notify(); woken != 2 {
		t.Errorf("woken = %d", woken)
	}
	for _, channel := range []chan struct{}{firstChannel, secondChannel} {
		select {
		case <-channel:
		case <-time.After(time.Second):
			t.Fatal("a waiting browser was not woken")
		}
	}
	server.clients.leave(first)
	if woken := server.clients.notify(); woken != 1 {
		t.Errorf("woken = %d, want the remaining browser", woken)
	}
}

func TestTheReloadStreamEndsWithTheRequest(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return page("hello"), nil, nil, nil
	}, nil)
	server.Rebuild()

	request := httptest.NewRequest(http.MethodGet, ReloadPath, nil)
	ctx, cancel := context.WithCancel(request.Context())
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		server.ServeHTTP(recorder, request)
	}()
	time.Sleep(50 * time.Millisecond)
	server.Rebuild()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream did not end with the request")
	}
	if !strings.Contains(recorder.Body.String(), "event: reload") {
		t.Errorf("stream = %q", recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("content type = %q", recorder.Header().Get("Content-Type"))
	}
}

type noFlush struct{ http.ResponseWriter }

func TestAClientThatCannotStreamIsTold(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return page("hello"), nil, nil, nil
	}, nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(noFlush{recorder}, httptest.NewRequest(http.MethodGet, ReloadPath, nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
}

func TestAnExplicitStatusAlsoDropsTheLength(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Length", "13")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<body>no</body>"))
		}), nil, nil, nil
	}, nil)
	server.Rebuild()
	got := get(t, server)
	if got.Code != http.StatusNotFound {
		t.Errorf("status = %d", got.Code)
	}
	if got.Header().Get("Content-Length") != "" {
		t.Errorf("length = %q", got.Header().Get("Content-Length"))
	}
	if !strings.Contains(got.Body.String(), ReloadPath) {
		t.Errorf("body = %q", got.Body.String())
	}
}

func TestAnExplicitStatusOnDataKeepsItsLength(t *testing.T) {
	server := New(func() (http.Handler, []diag.Diagnostic, map[string]string, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "2")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}), nil, nil, nil
	}, nil)
	server.Rebuild()
	if got := get(t, server); got.Header().Get("Content-Length") != "2" {
		t.Errorf("length = %q", got.Header().Get("Content-Length"))
	}
}

type failingWriter struct {
	http.ResponseWriter
	fail bool
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.fail {
		return 0, errors.New("client hung up")
	}
	return f.ResponseWriter.Write(p)
}

func TestTheInjectorReportsAWriteThatFails(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/html")
	wrapped := &injector{ResponseWriter: &failingWriter{ResponseWriter: recorder, fail: true}}
	if _, err := wrapped.Write([]byte("<html><body>a")); err == nil {
		t.Error("a failed write must reach the caller while the page is still open")
	}
	if _, err := wrapped.Write([]byte("</body></html>")); err == nil {
		t.Error("a failed write must reach the caller at the closing tag")
	}
}

func TestTheInjectorHoldsAWriteShorterThanTheClosingTag(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/html")
	wrapped := &injector{ResponseWriter: recorder}
	for _, chunk := range []string{"<b", "o", "dy>x</body>"} {
		if _, err := wrapped.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	wrapped.finish()
	if got := recorder.Body.String(); got != "<body>x"+ReloadScript+"</body>" {
		t.Errorf("body = %q, want every held byte and one script", got)
	}
}

func TestTheInjectorPassesANonHtmlTailThrough(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/html")
	wrapped := &injector{ResponseWriter: recorder}
	if _, err := wrapped.Write([]byte("<p>x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	recorder.Header().Set("Content-Type", "application/json")
	wrapped.finish()
	if got := recorder.Body.String(); got != "<p>x" {
		t.Errorf("body = %q, want the held bytes without a script", got)
	}
}

func TestScanTakesTheFirstFreePort(t *testing.T) {
	held, err := net.Listen("tcp", LoopbackHost+":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = held.Close() }()
	taken, ok := held.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("the listener did not report a tcp address")
	}

	listener, err := Scan(LoopbackHost, taken.Port, taken.Port+1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	defer func() { _ = listener.Close() }()
	next, ok := listener.Addr().(*net.TCPAddr)
	if !ok || next.Port != taken.Port+1 {
		t.Errorf("addr = %v, want the port after the busy one", listener.Addr())
	}

	if _, err := Scan(LoopbackHost, taken.Port, taken.Port); err == nil {
		t.Error("a range with nothing free must be reported")
	}
}

func TestListenScansFromThreeThousand(t *testing.T) {
	if FirstPort != 3000 || LastPort <= FirstPort {
		t.Errorf("range = %d..%d, want a scan that starts at 3000", FirstPort, LastPort)
	}
}

func TestListenHonoursAnExplicitAddress(t *testing.T) {
	listener, err := Listen("127.0.0.1:0", false)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	if address, ok := listener.Addr().(*net.TCPAddr); !ok || !address.IP.IsLoopback() {
		t.Errorf("addr = %v, want the address it was given", listener.Addr())
	}
	if _, err := Listen("127.0.0.1:-1", false); err == nil {
		t.Error("a broken address must be reported, not scanned around")
	}
}

func TestAddressesNameTheLocalUrl(t *testing.T) {
	listener, err := Listen("127.0.0.1:0", false)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	local, network := Addresses(listener)
	if !strings.HasPrefix(local, "http://localhost:") {
		t.Errorf("local = %q", local)
	}
	if network != "" {
		t.Errorf("network = %q, want nothing for a loopback listener", network)
	}
}

func TestRelativeTrimsTheProjectRoot(t *testing.T) {
	if got := Relative("/tmp/app", "/tmp/app/app/page.rill"); got != "app/page.rill" {
		t.Errorf("Relative = %q", got)
	}
	if got := Relative("/tmp/app", "/other/x.rill"); got != "/other/x.rill" {
		t.Errorf("Relative = %q, want a path outside the project left alone", got)
	}
}

func TestUrlsFollowTheAddressTheListenerTook(t *testing.T) {
	others := []net.Addr{
		&net.IPNet{IP: net.IPv6loopback},
		&net.IPNet{IP: net.ParseIP("::1")},
		&net.IPNet{IP: net.ParseIP("192.168.1.14")},
	}
	local, network := Urls(net.IPv4zero, "3000", others)
	if local != "http://localhost:3000/" || network != "http://192.168.1.14:3000/" {
		t.Errorf("local = %q, network = %q", local, network)
	}
	if _, network := Urls(net.IPv4(127, 0, 0, 1), "3000", others); network != "" {
		t.Errorf("network = %q, want nothing for a loopback listener", network)
	}
	if _, network := Urls(net.ParseIP("10.0.0.7"), "3000", others); network != "http://10.0.0.7:3000/" {
		t.Errorf("network = %q, want the address the listener took", network)
	}
}

func TestReachableSkipsWhatCannotBeOpened(t *testing.T) {
	if got := Reachable(nil, "3000"); got != "" {
		t.Errorf("Reachable = %q, want nothing without an interface", got)
	}
	only := []net.Addr{&net.IPNet{IP: net.IPv4(127, 0, 0, 1)}, &net.TCPAddr{}}
	if got := Reachable(only, "3000"); got != "" {
		t.Errorf("Reachable = %q, want loopback and non-ip entries skipped", got)
	}
}

func TestAddressesFallBackToWhateverTheListenerReports(t *testing.T) {
	local, network := Addresses(unixish{})
	if local != "pipe" || network != "" {
		t.Errorf("local = %q, network = %q", local, network)
	}
}

type unixish struct{ net.Listener }

func (unixish) Addr() net.Addr { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "unix" }
func (fakeAddr) String() string  { return "pipe" }

func TestTheInjectorLeavesACompressedBodyAlone(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/html")
	recorder.Header().Set("Content-Encoding", "gzip")
	wrapped := &injector{ResponseWriter: recorder}
	packed := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00}
	if _, err := wrapped.Write(packed); err != nil {
		t.Fatalf("Write: %v", err)
	}
	wrapped.finish()
	if recorder.Body.Len() != len(packed) {
		t.Errorf("body = %d bytes, want the compressed bytes untouched", recorder.Body.Len())
	}
	if recorder.Header().Get("Content-Length") != "" {
		t.Log("length is dropped for html, which is fine")
	}
}

func TestScanStaysOnLoopbackByDefault(t *testing.T) {
	listener, err := Listen("", false)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("the listener did not report a tcp address")
	}
	if !address.IP.IsLoopback() {
		t.Errorf("addr = %v, want the dev server kept off the network", listener.Addr())
	}
}

func TestSharingIsAskedForExplicitly(t *testing.T) {
	listener, err := Listen("", true)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("the listener did not report a tcp address")
	}
	if !address.IP.IsUnspecified() {
		t.Errorf("addr = %v, want every interface", listener.Addr())
	}
}

func TestTheDevServerCarriesTimeouts(t *testing.T) {
	server := HTTP(http.NewServeMux())
	if server.ReadHeaderTimeout == 0 || server.ReadTimeout == 0 || server.IdleTimeout == 0 {
		t.Errorf("server = %+v, want a slow client bounded", server)
	}
	if server.MaxHeaderBytes == 0 {
		t.Error("a header budget must be set")
	}
}

func TestTheChildIsToldToStayOnLoopback(t *testing.T) {
	env := childEnv([]string{"EXTRA=1"}, LoopbackHost+":4321")
	var addr string
	for _, entry := range env {
		if strings.HasPrefix(entry, "ADDR=") {
			addr = strings.TrimPrefix(entry, "ADDR=")
		}
	}
	if addr != LoopbackHost+":4321" {
		t.Errorf("ADDR = %q, want the child bound to loopback, not every interface", addr)
	}
	if !slices.Contains(env, "RILL_DEV=1") || !slices.Contains(env, "EXTRA=1") {
		t.Errorf("env = %v, want the dev marker and the caller's entries kept", env)
	}
}

func TestGeneratedOutputNeverTriggersARebuild(t *testing.T) {
	root := filepath.FromSlash("/project")
	stale := []string{
		"internal/gen",
		"internal/gen/bundles",
		"internal/gen/bundles/island.ABC.js",
		"internal/gen/bundles/island.ABC.js.br",
		"internal/gen/public/sky.webp",
		"internal/gen/styles/app.css",
		"dist",
		"dist/server",
		"dist/assets/app.css",
		".rill",
		".rill/cache/tailwind-inventory.txt",
		"node_modules/react/index.js",
		".git/HEAD",
	}
	for _, name := range stale {
		path := filepath.Join(root, filepath.FromSlash(name))
		if Relevant(path) {
			t.Errorf("Relevant(%s) = true, want generated output to start no rebuild", name)
		}
	}

	live := []string{
		"app/page.rill",
		"components/Card/template.rill",
		"locales/en.json",
		"styles/app.css",
		"app/gen/page.rill",
		"app/dist-notes/page.rill",
	}
	for _, name := range live {
		path := filepath.Join(root, filepath.FromSlash(name))
		if !Relevant(path) {
			t.Errorf("Relevant(%s) = false, want a source file watched", name)
		}
	}
}
