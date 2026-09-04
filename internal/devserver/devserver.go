package devserver

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/apptivitypl/gopage/internal/diag"
)

const Debounce = 120 * time.Millisecond

type Build func() (http.Handler, []diag.Diagnostic, map[string]string, error)

type state struct {
	handler     http.Handler
	diagnostics []diag.Diagnostic
	sources     map[string]string
	failure     string
}

type Server struct {
	build   Build
	current atomic.Pointer[state]
	log     func(string, ...any)
	clients *clients
}

func New(build Build, log func(string, ...any)) *Server {
	if log == nil {
		log = func(string, ...any) {}
	}
	server := &Server{build: build, log: log, clients: newClients()}
	server.current.Store(&state{})
	return server
}

func (s *Server) Rebuild() bool {
	handler, diagnostics, sources, err := s.build()
	if err != nil {
		s.keep(diagnostics, sources, failureOf(err, diagnostics))
		s.log("build failed: %v", err)
		s.clients.notify()
		return false
	}
	s.current.Store(&state{handler: handler, diagnostics: diagnostics, sources: sources})
	s.clients.notify()
	return true
}

func (s *Server) keep(diagnostics []diag.Diagnostic, sources map[string]string, failure string) {
	held := s.current.Load()
	s.current.Store(&state{
		handler:     held.handler,
		diagnostics: diagnostics,
		sources:     sources,
		failure:     failure,
	})
}

func failureOf(err error, diagnostics []diag.Diagnostic) string {
	if errorsIn(diagnostics) {
		return ""
	}
	return err.Error()
}

func (s *Server) Broken() bool {
	return errorsIn(s.current.Load().diagnostics)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == ReloadPath {
		s.reload(w, r)
		return
	}
	held := s.current.Load()
	if errorsIn(held.diagnostics) {
		Overlay(w, held.diagnostics, held.sources)
		return
	}
	if held.handler == nil {
		if held.failure != "" {
			OverlayFailure(w, held.failure)
			return
		}
		Overlay(w, nil, nil)
		return
	}
	wrapped := &injector{ResponseWriter: w}
	held.handler.ServeHTTP(wrapped, r)
	wrapped.finish()
}

func errorsIn(diagnostics []diag.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Severity == diag.Error {
			return true
		}
	}
	return false
}
