package devserver

import (
	"fmt"
	"net/http"
	"sync"
)

const (
	ReloadPath   = "/_rill/reload"
	ReloadScript = `<script>(()=>{const s=new EventSource("` + ReloadPath + `");` +
		`s.addEventListener("reload",()=>location.reload());})()</script>`
	reloadEvent = "reload"
)

type clients struct {
	mu      sync.Mutex
	next    int
	waiting map[int]chan struct{}
}

func newClients() *clients {
	return &clients{waiting: map[int]chan struct{}{}}
}

func (c *clients) join() (int, chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.next
	c.next++
	channel := make(chan struct{}, 1)
	c.waiting[id] = channel
	return id, channel
}

func (c *clients) leave(id int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.waiting, id)
}

func (c *clients) notify() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, channel := range c.waiting {
		select {
		case channel <- struct{}{}:
		default:
		}
	}
	return len(c.waiting)
}

func (s *Server) reload(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not available", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id, channel := s.clients.join()
	defer s.clients.leave(id)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-channel:
			if _, err := fmt.Fprintf(w, "event: %s\ndata: 1\n\n", reloadEvent); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

type injector struct {
	http.ResponseWriter
	injected bool
	carry    []byte
}

func (i *injector) Write(p []byte) (int, error) {
	if i.injected || !plainHTML(i.Header()) {
		return i.ResponseWriter.Write(p)
	}
	i.Header().Del("Content-Length")
	held := make([]byte, 0, len(i.carry)+len(p))
	held = append(held, i.carry...)
	held = append(held, p...)
	i.carry = nil
	if cut := lastIndex(held, closingBody); cut >= 0 {
		i.injected = true
		if _, err := i.ResponseWriter.Write(insertAt(held, cut)); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	keep := len(closingBody) - 1
	if len(held) < keep {
		keep = len(held)
	}
	i.carry = append([]byte(nil), held[len(held)-keep:]...)
	if _, err := i.ResponseWriter.Write(held[:len(held)-keep]); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (i *injector) finish() {
	if i.injected {
		return
	}
	if !plainHTML(i.Header()) {
		_, _ = i.ResponseWriter.Write(i.carry)
		return
	}
	i.injected = true
	tail := make([]byte, 0, len(i.carry)+len(ReloadScript))
	tail = append(tail, i.carry...)
	_, _ = i.ResponseWriter.Write(append(tail, ReloadScript...))
}

func (i *injector) Flush() {
	if flusher, ok := i.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (i *injector) WriteHeader(status int) {
	if plainHTML(i.Header()) {
		i.Header().Del("Content-Length")
	}
	i.ResponseWriter.WriteHeader(status)
}

func isHTML(contentType string) bool {
	return len(contentType) >= 9 && contentType[:9] == "text/html"
}

func plainHTML(header http.Header) bool {
	return isHTML(header.Get("Content-Type")) && header.Get("Content-Encoding") == ""
}

const closingBody = "</body>"

func insertAt(body []byte, cut int) []byte {
	out := make([]byte, 0, len(body)+len(ReloadScript))
	out = append(out, body[:cut]...)
	out = append(out, ReloadScript...)
	return append(out, body[cut:]...)
}

func lastIndex(body []byte, needle string) int {
	for i := len(body) - len(needle); i >= 0; i-- {
		if string(body[i:i+len(needle)]) == needle {
			return i
		}
	}
	return -1
}
