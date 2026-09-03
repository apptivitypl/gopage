package devserver

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	FirstPort    = 3000
	LastPort     = 3099
	LoopbackHost = "127.0.0.1"
	EveryHost    = ""
)

const (
	HeaderTimeout  = 10 * time.Second
	ReadTimeout    = 30 * time.Second
	IdleTimeout    = 2 * time.Minute
	MaxHeaderBytes = 1 << 20
)

func HTTP(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: HeaderTimeout,
		ReadTimeout:       ReadTimeout,
		IdleTimeout:       IdleTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
	}
}

func Listen(addr string, shared bool) (net.Listener, error) {
	if addr != "" {
		return net.Listen("tcp", addr)
	}
	host := LoopbackHost
	if shared {
		host = EveryHost
	}
	return Scan(host, FirstPort, LastPort)
}

func Scan(host string, first, last int) (net.Listener, error) {
	var held error
	for port := first; port <= last; port++ {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			return listener, nil
		}
		held = err
	}
	return nil, fmt.Errorf("every port from %d to %d is taken: %w", first, last, held)
}

func Addresses(listener net.Listener) (string, string) {
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return listener.Addr().String(), ""
	}
	return Urls(address.IP, strconv.Itoa(address.Port), interfaces())
}

func Urls(ip net.IP, port string, others []net.Addr) (string, string) {
	local := "http://localhost:" + port + "/"
	switch {
	case ip.IsLoopback():
		return local, ""
	case !ip.IsUnspecified():
		return local, "http://" + ip.String() + ":" + port + "/"
	default:
		return local, Reachable(others, port)
	}
}

func Reachable(entries []net.Addr, port string) string {
	for _, entry := range entries {
		address, ok := entry.(*net.IPNet)
		if !ok || address.IP.IsLoopback() || address.IP.To4() == nil {
			continue
		}
		return "http://" + address.IP.String() + ":" + port + "/"
	}
	return ""
}

func interfaces() []net.Addr {
	entries, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	return entries
}

func Relative(root, path string) string {
	slashed := strings.ReplaceAll(path, "\\", "/")
	prefix := strings.ReplaceAll(root, "\\", "/")
	if prefix == "" || prefix == "." || !strings.HasPrefix(slashed, prefix) {
		return slashed
	}
	return strings.TrimPrefix(strings.TrimPrefix(slashed, prefix), "/")
}
