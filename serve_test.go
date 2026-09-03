//go:build !js

package rill

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"github.com/apptivitypl/rill/internal/logs"
)

func listenApp(t *testing.T) (*App, string) {
	t.Helper()
	app, err := New(Options{Manifest: demo(t), Config: []byte(`{
  "app": {
    "name": "demo"
  }
}`)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return app, address
}

func TestServeSpeaksHTTP2WithoutTLS(t *testing.T) {
	app, address := listenApp(t)
	server := Server(address, app)
	go func() { _ = server.ListenAndServe() }()
	t.Cleanup(func() { _ = server.Close() })

	client := &http.Client{Transport: h2cTransport()}
	response := until(t, client, "http://"+address+"/")
	defer func() { _ = response.Body.Close() }()

	if response.Proto != "HTTP/2.0" {
		t.Errorf("proto = %q, want HTTP/2.0 over h2c", response.Proto)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) == "" {
		t.Error("the page came back empty")
	}
}

func TestServeStillAnswersHTTP1(t *testing.T) {
	app, address := listenApp(t)
	server := Server(address, app)
	go func() { _ = server.ListenAndServe() }()
	t.Cleanup(func() { _ = server.Close() })

	response := until(t, http.DefaultClient, "http://"+address+"/")
	defer func() { _ = response.Body.Close() }()
	if response.Proto != "HTTP/1.1" {
		t.Errorf("proto = %q, want HTTP/1.1 for a client that cannot upgrade", response.Proto)
	}
}

func TestServeTLSNegotiatesHTTP2(t *testing.T) {
	app, address := listenApp(t)
	certificate, key := selfSigned(t)
	go func() { _ = ServeTLS(address, app, certificate, key) }()

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}},
		ForceAttemptHTTP2: true,
	}}
	response := until(t, client, "https://"+address+"/")
	defer func() { _ = response.Body.Close() }()

	if response.Proto != "HTTP/2.0" {
		t.Errorf("proto = %q, want HTTP/2.0 over TLS", response.Proto)
	}
}

func TestServeReadsTheCertificateFromTheEnvironment(t *testing.T) {
	app, address := listenApp(t)
	certificate, key := selfSigned(t)
	t.Setenv(CertificateVar, certificate)
	t.Setenv(KeyVar, key)
	go func() { _ = Serve(address, app) }()

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}}
	response := until(t, client, "https://"+address+"/")
	defer func() { _ = response.Body.Close() }()
	if response.Proto != "HTTP/2.0" {
		t.Errorf("proto = %q, want the environment to switch on TLS", response.Proto)
	}
}

func TestServeReportsAnAddressItCannotBind(t *testing.T) {
	app, _ := listenApp(t)
	if err := Serve("127.0.0.1:99999", app); err == nil {
		t.Error("Serve should report a port it cannot bind")
	}
}

func h2cTransport() http.RoundTripper {
	return &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, address string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
}

func until(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	var last error
	for range 50 {
		response, err := client.Get(url)
		if err == nil {
			return response
		}
		last = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the server never answered %s: %v", url, last)
	return nil
}

func selfSigned(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	dir := t.TempDir()
	certificatePath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	encoded, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	write(t, certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	write(t, keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded}))
	return certificatePath, keyPath
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestServeReportsAPortItCannotTake(t *testing.T) {
	app, address := listenApp(t)
	held, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = held.Close() }()
	if err := Serve(address, app); err == nil {
		t.Error("a taken port must be reported, not swallowed")
	}
}

func TestShutdownFallsBackToClosing(t *testing.T) {
	held := make(chan struct{})
	running := make(chan struct{})
	defer close(held)
	server := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			close(running)
			<-held
		}),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve(listener) }()
	go func() {
		response, err := http.Get("http://" + listener.Addr().String() + "/")
		if err == nil {
			_ = response.Body.Close()
		}
	}()

	select {
	case <-running:
	case <-time.After(5 * time.Second):
		t.Fatal("no request ever reached the server")
	}
	if err := shutdown(server, time.Nanosecond); err != nil {
		t.Errorf("shutdown: %v, want a request that will not finish to be closed", err)
	}
}

func TestAClosedServerIsNotAFailure(t *testing.T) {
	if err := Closed(http.ErrServerClosed); err != nil {
		t.Errorf("Closed = %v, want a clean stop", err)
	}
	if err := Closed(errors.New("bind failed")); err == nil {
		t.Error("a real failure must survive")
	}
}

func TestTheServerCarriesItsLimits(t *testing.T) {
	app, address := listenApp(t)
	server := Server(address, app)
	if server.ReadTimeout != ReadTimeout || server.ReadHeaderTimeout != HeaderTimeout {
		t.Errorf("timeouts = %v / %v", server.ReadTimeout, server.ReadHeaderTimeout)
	}
	if server.MaxHeaderBytes != MaxHeaderBytes || server.IdleTimeout != IdleTimeout {
		t.Errorf("limits = %d / %v", server.MaxHeaderBytes, server.IdleTimeout)
	}
	if server.WriteTimeout != 0 {
		t.Error("a write timeout would cut every stream and every sse response")
	}
	if server.ErrorLog == nil {
		t.Error("net/http diagnostics belong in the application log")
	}
}

func waitUp(t *testing.T, address string) {
	t.Helper()
	for range 100 {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the server never came up")
}

func TestServeAnnouncesTheAddressUnlessRillDevRunsIt(t *testing.T) {
	written := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(written, nil))
	app, err := New(Options{Manifest: demo(t), Config: []byte("{\"app\": {\"name\": \"demo\"}}"), Logger: logger})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()

	t.Setenv(logs.DevVar, "")
	go func() { _ = Serve(address, app) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(written.String(), "listening") {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(written.String(), "msg=listening") || !strings.Contains(written.String(), "addr="+address) {
		t.Errorf("log = %q, want the address announced", written.String())
	}

	quiet := &syncBuffer{}
	child, err := New(Options{Manifest: demo(t), Config: []byte("{\"app\": {\"name\": \"demo\"}}"),
		Logger: slog.New(slog.NewTextHandler(quiet, nil))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Setenv(logs.DevVar, "1")
	go func() { _ = Serve("127.0.0.1:0", child) }()
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(quiet.String(), "listening") {
		t.Errorf("log = %q, want the dev child to stay quiet, the banner has the address", quiet.String())
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestListenWithoutALimitAcceptsFreely(t *testing.T) {
	listener, err := Listen("127.0.0.1:0", 0)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	for range 4 {
		conn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		accepted, err := listener.Accept()
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		_ = accepted.Close()
		_ = conn.Close()
	}
}

func TestListenHoldsTheConnectionLimit(t *testing.T) {
	listener, err := Listen("127.0.0.1:0", 1)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	first, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = first.Close() }()
	held, err := listener.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	second, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = second.Close() }()

	waiting := make(chan struct{})
	go func() {
		next, err := listener.Accept()
		if err == nil {
			_ = next.Close()
		}
		close(waiting)
	}()

	select {
	case <-waiting:
		t.Error("the second connection was accepted while the first was still held")
	case <-time.After(150 * time.Millisecond):
	}

	_ = held.Close()
	select {
	case <-waiting:
	case <-time.After(2 * time.Second):
		t.Error("closing the first connection did not release the slot")
	}
}

func TestAnAddressWithoutAPortIsReported(t *testing.T) {
	if _, err := Listen("127.0.0.1:-1", 0); err == nil {
		t.Error("a broken address must be reported")
	}
}

func TestAnEmptyAddressFallsBackToTheHttpPort(t *testing.T) {
	listener, err := Listen("", 0)
	if err == nil {
		defer func() { _ = listener.Close() }()
		if _, port, _ := net.SplitHostPort(listener.Addr().String()); port != "80" {
			t.Errorf("port = %q, want the http port", port)
		}
		return
	}
	if !strings.Contains(err.Error(), "permission denied") && !strings.Contains(err.Error(), "in use") {
		t.Errorf("err = %v, want the http port attempted", err)
	}
}

func TestServeReportsAFailureAfterTheListenerIsUp(t *testing.T) {
	app, _ := listenApp(t)
	missing := filepath.Join(t.TempDir(), "absent.pem")
	if err := ServeTLS("127.0.0.1:0", app, missing, missing); err == nil {
		t.Error("a certificate that does not exist must be reported")
	}
}
