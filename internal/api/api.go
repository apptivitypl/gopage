package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/apptivitypl/gopage/internal/logs"
)

type Response interface {
	Respond(w http.ResponseWriter) error
}

type body struct {
	status      int
	contentType string
	payload     []byte
	value       any
	encode      bool
}

func (b *body) Respond(w http.ResponseWriter) error {
	payload := b.payload
	if b.encode {
		encoded, err := json.Marshal(b.value)
		if err != nil {
			return err
		}
		payload = encoded
	}
	w.Header().Set("Content-Type", b.contentType)
	w.WriteHeader(b.status)
	_, err := w.Write(payload)
	return err
}

func (b *body) WithStatus(status int) Response {
	b.status = status
	return b
}

type Coded interface {
	Response
	WithStatus(status int) Response
}

func JSON(value any) Coded {
	return &body{status: http.StatusOK, contentType: "application/json", value: value, encode: true}
}

func Text(text string) Coded {
	return &body{status: http.StatusOK, contentType: "text/plain; charset=utf-8", payload: []byte(text)}
}

func Content(contentType string, payload []byte) Coded {
	return &body{status: http.StatusOK, contentType: contentType, payload: payload}
}

func NoContent() Coded {
	return &body{status: http.StatusNoContent, contentType: "text/plain; charset=utf-8"}
}

type Handler func(*http.Request) (Response, error)

func Mux(handlers map[string]Handler) http.Handler {
	allow := allowHeader(handlers)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logs.From(r.Context())
		handler, ok := handlers[r.Method]
		if !ok && r.Method == http.MethodHead {
			handler, ok = handlers[http.MethodGet]
		}
		if !ok {
			logger.Warn("api method not allowed", "path", r.URL.Path, "method", r.Method, "allow", allow)
			w.Header().Set("Allow", allow)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		response, err := handler(r)
		if err != nil {
			logger.Error("api handler failed", "path", r.URL.Path, "method", r.Method, "error", err)
			writeError(w, http.StatusInternalServerError)
			return
		}
		if response == nil {
			logger.Error("api handler answered nothing", "path", r.URL.Path, "method", r.Method)
			writeError(w, http.StatusInternalServerError)
			return
		}
		if err := response.Respond(w); err != nil {
			logger.Error("api write failed", "path", r.URL.Path, "error", err)
		}
	})
}

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":` + statusText(status) + `}`))
}

func statusText(status int) string {
	return `"` + strings.ToLower(http.StatusText(status)) + `"`
}

func allowHeader(handlers map[string]Handler) string {
	methods := make([]string, 0, len(handlers)+1)
	for method := range handlers {
		methods = append(methods, method)
	}
	if _, ok := handlers[http.MethodGet]; ok {
		methods = append(methods, http.MethodHead)
	}
	sort.Strings(methods)
	return strings.Join(methods, ", ")
}
