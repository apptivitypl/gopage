//go:build !js

package gopage

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/netutil"

	"github.com/apptivitypl/gopage/internal/logs"
)

const (
	CertificateVar = "TLS_CERT"
	KeyVar         = "TLS_KEY"

	ReadTimeout    = 30 * time.Second
	HeaderTimeout  = 10 * time.Second
	IdleTimeout    = 2 * time.Minute
	GraceTimeout   = 10 * time.Second
	MaxHeaderBytes = 1 << 20
)

func Serve(addr string, app *App) error {
	return ServeTLS(addr, app, os.Getenv(CertificateVar), os.Getenv(KeyVar))
}

func ServeTLS(addr string, app *App, certificate, key string) error {
	server := Server(addr, app)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if os.Getenv(logs.DevVar) == "" {
		app.Log().Info("listening", "addr", addr)
	}
	listener, err := Listen(addr, app.MaxConnections())
	if err != nil {
		return err
	}
	failed := make(chan error, 1)
	go func() {
		if certificate == "" || key == "" {
			failed <- server.Serve(listener)
			return
		}
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		failed <- server.ServeTLS(listener, certificate, key)
	}()

	select {
	case err := <-failed:
		return Closed(err)
	case <-ctx.Done():
		stop()
		app.Log().Info("shutting down", "grace", GraceTimeout)
		return shutdown(server, GraceTimeout)
	}
}

func Listen(addr string, limit int) (net.Listener, error) {
	if addr == "" {
		addr = ":http"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if limit > 0 {
		return netutil.LimitListener(listener, limit), nil
	}
	return listener, nil
}

func Closed(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func shutdown(server *http.Server, grace time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return server.Close()
	}
	return nil
}

func Server(addr string, app *App) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{
		Addr:              addr,
		Handler:           app.Handler(),
		Protocols:         protocols,
		ErrorLog:          slog.NewLogLogger(app.Log().Handler(), slog.LevelWarn),
		ReadHeaderTimeout: HeaderTimeout,
		ReadTimeout:       ReadTimeout,
		IdleTimeout:       IdleTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
	}
}
