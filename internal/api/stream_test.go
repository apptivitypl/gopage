package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type story struct {
	Title string `json:"title"`
}

func TestEventsWriteTheServerSentEventFormat(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/stories", nil)
	response := Events(request, func(out *Stream) error {
		if err := out.Send("story", story{Title: "first"}); err != nil {
			return err
		}
		return out.Send("done", nil)
	})
	if err := response.Respond(recorder); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	body := recorder.Body.String()
	for _, want := range []string{"event: story\n", `data: {"title":"first"}`, "event: done\n", "data: \n\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want %q", body, want)
		}
	}
	if got := recorder.Header().Get("Content-Type"); got != EventType {
		t.Errorf("content type = %q, want %q", got, EventType)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("cache control = %q", got)
	}
	if got := recorder.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("buffering = %q, want the proxy told to pass events through", got)
	}
}

func TestAStreamCountsWhatItSent(t *testing.T) {
	recorder := httptest.NewRecorder()
	var sent int
	response := Events(httptest.NewRequest(http.MethodGet, "/x", nil), func(out *Stream) error {
		for range 3 {
			if err := out.Send("tick", 1); err != nil {
				return err
			}
		}
		sent = out.Sent()
		return nil
	})
	if err := response.Respond(recorder); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if sent != 3 {
		t.Errorf("Sent() = %d, want 3", sent)
	}
}

func TestAStreamStopsWhenTheClientLeaves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	cancel()
	err := Events(request, func(out *Stream) error {
		return out.Send("story", story{Title: "never"})
	}).Respond(httptest.NewRecorder())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want the cancelled context", err)
	}
}

func TestAStreamCarriesTheRequestContext(t *testing.T) {
	type key struct{}
	request := httptest.NewRequest(http.MethodGet, "/x", nil)
	request = request.WithContext(context.WithValue(request.Context(), key{}, "carried"))
	var seen any
	if err := Events(request, func(out *Stream) error {
		seen = out.Context().Value(key{})
		return nil
	}).Respond(httptest.NewRecorder()); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if seen != "carried" {
		t.Errorf("context value = %v, want the request context", seen)
	}
}

func TestAStreamReportsAValueItCannotEncode(t *testing.T) {
	err := Events(httptest.NewRequest(http.MethodGet, "/x", nil), func(out *Stream) error {
		return out.Send("bad", make(chan int))
	}).Respond(httptest.NewRecorder())
	if err == nil {
		t.Error("a value json cannot encode must be reported")
	}
}

func TestCommentsKeepTheConnectionWarm(t *testing.T) {
	recorder := httptest.NewRecorder()
	if err := Events(httptest.NewRequest(http.MethodGet, "/x", nil), func(out *Stream) error {
		return out.Comment("keep-alive")
	}).Respond(recorder); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if got := recorder.Body.String(); got != ": keep-alive\n\n" {
		t.Errorf("body = %q", got)
	}
}

type unflushable struct{ http.ResponseWriter }

func TestAWriterWithoutAFlusherStillStreams(t *testing.T) {
	recorder := httptest.NewRecorder()
	if err := Events(httptest.NewRequest(http.MethodGet, "/x", nil), func(out *Stream) error {
		return out.Send("story", story{Title: "worker"})
	}).Respond(unflushable{recorder}); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if !strings.Contains(recorder.Body.String(), `"title":"worker"`) {
		t.Errorf("body = %q, want the event even without a flusher", recorder.Body.String())
	}
}

func TestEventsWithoutARequestUseTheBackgroundContext(t *testing.T) {
	if err := Events(nil, func(out *Stream) error {
		if out.Context() == nil {
			return errors.New("no context")
		}
		return out.Send("", "plain")
	}).Respond(httptest.NewRecorder()); err != nil {
		t.Fatalf("Respond: %v", err)
	}
}

type failingWriter struct {
	http.ResponseWriter
	err error
}

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestAStreamReportsAWriteItCannotFinish(t *testing.T) {
	broken := failingWriter{ResponseWriter: httptest.NewRecorder(), err: errors.New("client hung up")}
	err := Events(httptest.NewRequest(http.MethodGet, "/x", nil), func(out *Stream) error {
		return out.Send("story", story{Title: "lost"})
	}).Respond(broken)
	if err == nil || !strings.Contains(err.Error(), "client hung up") {
		t.Errorf("err = %v, want the write failure", err)
	}
}

func TestACommentReportsAWriteItCannotFinish(t *testing.T) {
	broken := failingWriter{ResponseWriter: httptest.NewRecorder(), err: errors.New("client hung up")}
	err := Events(httptest.NewRequest(http.MethodGet, "/x", nil), func(out *Stream) error {
		return out.Comment("keep-alive")
	}).Respond(broken)
	if err == nil {
		t.Error("a comment that cannot be written must be reported")
	}
}
