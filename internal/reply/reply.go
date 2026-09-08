package reply

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/apptivitypl/gopage/internal/cache"
	"github.com/apptivitypl/gopage/internal/logs"
)

const (
	VaryHeader = "Vary"
	CookieVary = "Cookie"
)

type Recorder struct {
	mu       sync.Mutex
	status   int
	rejected int
	header   http.Header
	cookies  []*http.Cookie
	vary     []string
}

func NewRecorder() *Recorder {
	return &Recorder{}
}

func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	return cache.WithResponse(ctx, recorder)
}

func From(ctx context.Context) *Recorder {
	recorder, _ := cache.Response(ctx).(*Recorder)
	if recorder == nil {
		return NewRecorder()
	}
	return recorder
}

func (r *Recorder) Status(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if code < http.StatusOK || code > 599 {
		r.rejected = code
		return
	}
	r.status = code
}

func (r *Recorder) Code() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Recorder) Header() http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}

func (r *Recorder) SetCookie(held *http.Cookie) {
	if held == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cookies = append(r.cookies, held)
}

func (r *Recorder) Vary(names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		if name != "" && !slices.Contains(r.vary, name) {
			r.vary = append(r.vary, name)
		}
	}
}

func (r *Recorder) Touched() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status != 0 || r.rejected != 0 || len(r.cookies) > 0 || len(r.header) > 0 || len(r.vary) > 0
}

func (r *Recorder) Headers() http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.header) == 0 && len(r.vary) == 0 {
		return nil
	}
	out := r.header.Clone()
	if out == nil {
		out = http.Header{}
	}
	if len(r.vary) > 0 {
		out.Set(VaryHeader, strings.Join(r.vary, ", "))
	}
	return out
}

func (r *Recorder) Deliver(w http.ResponseWriter, request *http.Request, secure bool) {
	r.mu.Lock()
	cookies := slices.Clone(r.cookies)
	rejected := r.rejected
	r.mu.Unlock()
	logger := logs.From(request.Context())
	if rejected != 0 {
		logger.Warn("status refused", "path", request.URL.Path, "status", rejected)
	}
	for _, held := range cookies {
		shaped, err := Check(held, secure)
		if err != nil {
			logger.Warn("cookie refused", "path", request.URL.Path, "cookie", held.Name, "error", err)
			continue
		}
		http.SetCookie(w, shaped)
	}
}

func Apply(w http.ResponseWriter, header http.Header) {
	for name, values := range header {
		if name == VaryHeader {
			continue
		}
		w.Header()[name] = slices.Clone(values)
	}
	AddVary(w, splitVary(header.Get(VaryHeader))...)
}

func AddVary(w http.ResponseWriter, names ...string) {
	merged := splitVary(w.Header().Get(VaryHeader))
	for _, name := range names {
		if name != "" && !slices.Contains(merged, name) {
			merged = append(merged, name)
		}
	}
	if len(merged) > 0 {
		w.Header().Set(VaryHeader, strings.Join(merged, ", "))
	}
}

func splitVary(value string) []string {
	var names []string
	for part := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}
