package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/logs"
)

type Recorder struct {
	http.ResponseWriter
	status  int
	written int
}

func (r *Recorder) WriteHeader(status int) {
	if r.status == 0 && status >= http.StatusOK {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *Recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	written, err := r.ResponseWriter.Write(p)
	r.written += written
	return written, err
}

func (r *Recorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *Recorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

type Trace struct {
	Route    string
	Locale   string
	Path     string
	Method   string
	Status   int
	Cache    string
	Duration time.Duration
}

type Reporter func(Trace)

func (a *App) report(recorder *Recorder, r *http.Request, elapsed time.Duration) {
	if a.onRequest == nil {
		return
	}
	route := ""
	if matched, _, ok := a.router.Match(a.split(r.URL.Path).rest); ok {
		route = matched.Name
	}
	a.onRequest(Trace{
		Route:    route,
		Locale:   recorder.Header().Get(LocaleHeader),
		Path:     r.URL.Path,
		Method:   r.Method,
		Status:   recorder.Status(),
		Cache:    recorder.Header().Get(CacheHeader),
		Duration: elapsed,
	})
}

func (a *App) observe(next http.Handler) http.Handler {
	project := os.Getenv(logs.ProjectVar)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		logger := a.logger
		trace := logs.TraceOf(r)
		if attrs := trace.Attrs(project); len(attrs) > 0 {
			logger = slog.New(a.logger.Handler().WithAttrs(attrs))
		}
		recorder := &Recorder{ResponseWriter: w}
		request := r.WithContext(logs.With(r.Context(), logger))

		defer func() {
			if raised := recover(); raised != nil {
				a.panicked(recorder, request, logger, raised, debug.Stack())
			}
			elapsed := time.Since(started)
			a.access(recorder, request, logger, elapsed)
			a.report(recorder, request, elapsed)
		}()
		next.ServeHTTP(recorder, request)
	})
}

func (a *App) panicked(w *Recorder, r *http.Request, logger *slog.Logger, raised any, stack []byte) {
	logger.Error("handler panicked",
		"path", r.URL.Path, "method", r.Method,
		"error", fmt.Sprint(raised), "stack", string(stack))
	if w.status != 0 {
		return
	}
	a.fail(w, r, ir.FallbackError, http.StatusInternalServerError)
}

func (a *App) access(w *Recorder, r *http.Request, logger *slog.Logger, took time.Duration) {
	if !a.accessLog {
		return
	}
	status := w.Status()
	logger.LogAttrs(r.Context(), levelFor(status), logs.RequestMessage,
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Int("status", status),
		slog.Int("bytes", w.written),
		slog.Duration("took", took),
		logs.Request(r, status, w.written, took))
}

func levelFor(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
