//go:build !js

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/sonquer/rill/internal/compress"
	"github.com/sonquer/rill/internal/ir"
)

func bigManifest() *ir.Manifest {
	body := strings.Repeat("<p>a page worth compressing</p>", 60)
	plan := ir.Plan{Blob: []byte(body), Ops: []ir.Op{{Kind: ir.OpStatic, A: 0, B: uint32(len(body))}}}
	return &ir.Manifest{
		Plans:  []ir.Plan{plan},
		Routes: []ir.Route{{Pattern: "/", Name: "home", Plan: 0}},
	}
}

func asked(t *testing.T, app *App, path, accept string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if accept != "" {
		request.Header.Set("Accept-Encoding", accept)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestADocumentIsCompressedForAClientThatAsks(t *testing.T) {
	app := New(Options{Manifest: bigManifest()})
	plain := asked(t, app, "/", "")
	packed := asked(t, app, "/", "br")

	if got := packed.Header().Get("Content-Encoding"); got != Brotli {
		t.Fatalf("encoding = %q, want %q", got, Brotli)
	}
	if packed.Body.Len() >= plain.Body.Len() {
		t.Errorf("packed %d bytes from %d, want it smaller", packed.Body.Len(), plain.Body.Len())
	}
	if packed.Header().Get("Content-Length") != "" {
		t.Error("a compressed body cannot keep the length of the original")
	}
	read, err := io.ReadAll(brotli.NewReader(packed.Body))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(read) != plain.Body.String() {
		t.Error("the document did not survive compression")
	}
	if !strings.Contains(strings.Join(packed.Header().Values("Vary"), " "), "Accept-Encoding") {
		t.Error("a compressed answer must vary on the encoding")
	}
}

func TestGzipIsTakenWhenBrotliIsNotOffered(t *testing.T) {
	app := New(Options{Manifest: bigManifest()})
	if got := asked(t, app, "/", "gzip, deflate").Header().Get("Content-Encoding"); got != Gzip {
		t.Errorf("encoding = %q, want %q", got, Gzip)
	}
	if got := asked(t, app, "/", "deflate").Header().Get("Content-Encoding"); got != "" {
		t.Errorf("encoding = %q, want nothing we cannot produce", got)
	}
}

func TestAStreamedAnswerIsCompressedToo(t *testing.T) {
	app := streamApp(t, 0, "inline")
	recorder := asked(t, app, "/", "br")
	if got := recorder.Header().Get("Content-Encoding"); got != Brotli {
		t.Errorf("encoding = %q, want a stream compressed like any other answer", got)
	}
	if got := recorder.Header().Get("Content-Length"); got != "" {
		t.Errorf("content length = %q, want none once the length is unknown", got)
	}
	if recorder.Body.Len() == 0 {
		t.Error("the compressed stream carried no bytes")
	}
}

func TestAnEventStreamIsLeftAlone(t *testing.T) {
	app := New(Options{
		Manifest: plainManifest(),
		API: map[string]http.Handler{
			"/api/events": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", compress.EventType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(strings.Repeat("data: x\n\n", 200)))
			}),
		},
	})
	if got := asked(t, app, "/api/events", "br").Header().Get("Content-Encoding"); got != "" {
		t.Errorf("encoding = %q, want server-sent events untouched", got)
	}
}

func TestASmallAnswerIsLeftAlone(t *testing.T) {
	app := New(Options{Manifest: plainManifest()})
	if got := asked(t, app, "/", "br").Header().Get("Content-Encoding"); got != "" {
		t.Errorf("encoding = %q, want nothing below the threshold", got)
	}
}

func TestTheCodingIsReadFromTheHeader(t *testing.T) {
	cases := map[string]string{
		"br":                  Brotli,
		"gzip, br":            Brotli,
		"gzip;q=1.0, *;q=0.1": Gzip,
		"identity":            "",
		"":                    "",
		"brotli":              "",
	}
	for accept, want := range cases {
		if got := coding(accept); got != want {
			t.Errorf("coding(%q) = %q, want %q", accept, got, want)
		}
	}
}

func TestEarlyHintsDoNotDecideTheEncoding(t *testing.T) {
	app := New(Options{Manifest: bigManifest(), AssetLink: `</assets/app.css>; rel=preload`})
	recorder := asked(t, app, "/", "br")
	if got := recorder.Header().Get("Content-Encoding"); got != Brotli {
		t.Errorf("encoding = %q, want the 103 to leave the decision to the real answer", got)
	}
}

type refusing struct{ http.ResponseWriter }

func (refusing) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestAWriteWithoutAStatusStillCompresses(t *testing.T) {
	app := New(Options{Manifest: bigManifest()})
	body := strings.Repeat("<p>written straight out</p>", 60)
	handler := app.compressed(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = io.WriteString(w, body)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept-Encoding", "br")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Encoding"); got != Brotli {
		t.Errorf("encoding = %q, want a body written without an explicit status compressed too", got)
	}
	if recorder.Body.Len() >= len(body) {
		t.Errorf("packed %d bytes from %d", recorder.Body.Len(), len(body))
	}
}

func TestACompressionFailureIsReported(t *testing.T) {
	app, written := watched(t, Options{Manifest: bigManifest()})
	body := strings.Repeat("<p>a page nobody will read</p>", 60)
	handler := app.compressed(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept-Encoding", "br")
	handler.ServeHTTP(refusing{httptest.NewRecorder()}, request)

	if !strings.Contains(written.String(), "compression failed") {
		t.Errorf("log = %q, want the failure recorded", written.String())
	}
}

func TestFlushingACompressedAnswerEmitsBytes(t *testing.T) {
	app := New(Options{
		Manifest: plainManifest(),
		API: map[string]http.Handler{
			"/api/slow": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(strings.Repeat("<p>a paragraph</p>", 200)))
				w.(http.Flusher).Flush()
				panic("the tail never runs")
			}),
		},
	})
	recorder := asked(t, app, "/api/slow", "br")
	if recorder.Header().Get("Content-Encoding") != Brotli {
		t.Fatalf("encoding = %q", recorder.Header().Get("Content-Encoding"))
	}
	if recorder.Body.Len() == 0 {
		t.Error("flush wrote nothing; the compressor is still holding the response")
	}
}
