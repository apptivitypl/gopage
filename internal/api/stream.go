package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const EventType = "text/event-stream"

type Stream struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	ctx     context.Context
	sent    int
}

func (s *Stream) Send(event string, value any) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	var payload []byte
	if value != nil {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		payload = encoded
	}
	var b strings.Builder
	if event != "" {
		fmt.Fprintf(&b, "event: %s\n", event)
	}
	if len(payload) == 0 {
		b.WriteString("data: \n\n")
	} else {
		fmt.Fprintf(&b, "data: %s\n\n", payload)
	}
	if _, err := s.writer.Write([]byte(b.String())); err != nil {
		return err
	}
	s.sent++
	s.Flush()
	return nil
}

func (s *Stream) Comment(text string) error {
	if _, err := fmt.Fprintf(s.writer, ": %s\n\n", text); err != nil {
		return err
	}
	s.Flush()
	return nil
}

func (s *Stream) Flush() {
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *Stream) Sent() int {
	return s.sent
}

func (s *Stream) Context() context.Context {
	return s.ctx
}

type events struct {
	produce func(*Stream) error
	request *http.Request
}

func Events(r *http.Request, produce func(*Stream) error) Response {
	return &events{produce: produce, request: r}
}

func (e *events) Respond(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", EventType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	stream := &Stream{writer: w, ctx: context.Background()}
	if e.request != nil {
		stream.ctx = e.request.Context()
	}
	if flusher, ok := w.(http.Flusher); ok {
		stream.flusher = flusher
	}
	stream.Flush()
	return e.produce(stream)
}
