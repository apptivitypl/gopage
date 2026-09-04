package devserver

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"time"
)

type App struct {
	command *exec.Cmd
	base    *url.URL
}

type Launch struct {
	Dir     string
	Binary  string
	Env     []string
	Output  io.Writer
	Timeout time.Duration
	Pause   time.Duration
}

func Start(launch Launch) (*App, error) {
	port, err := FreePort()
	if err != nil {
		return nil, err
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	command := exec.Command(launch.Binary)
	command.Dir = launch.Dir
	command.Env = childEnv(launch.Env, address)
	output := launch.Output
	if output == nil {
		output = os.Stderr
	}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		return nil, err
	}
	if err := waitPort(address, launch.Timeout, launch.Pause); err != nil {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		return nil, err
	}
	base, err := url.Parse("http://" + address)
	if err != nil {
		return nil, err
	}
	return &App{command: command, base: base}, nil
}

func childEnv(extra []string, address string) []string {
	return append(append(os.Environ(), extra...), "ADDR="+address, "GOPAGE_DEV=1")
}

func (a *App) Handler() http.Handler {
	proxy := &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(a.base)
			r.SetXForwarded()
			r.Out.Host = r.In.Host
			r.Out.Header.Del("Accept-Encoding")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, "gopage dev: the application is not answering: "+err.Error(), http.StatusBadGateway)
		},
	}
	return proxy
}

func (a *App) Stop() {
	if a == nil || a.command == nil || a.command.Process == nil {
		return
	}
	_ = a.command.Process.Kill()
	_, _ = a.command.Process.Wait()
}

func FreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("the listener did not report a tcp address")
	}
	return address.Port, nil
}

func waitPort(address string, timeout, pause time.Duration) error {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if pause <= 0 {
		pause = 25 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, pause*4)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		time.Sleep(pause)
	}
	return fmt.Errorf("the application never listened on %s", address)
}
