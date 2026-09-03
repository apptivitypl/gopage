//go:build !js

package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/apptivitypl/rill/internal/compress"
)

const (
	Brotli = "br"
	Gzip   = "gzip"
)

func (a *App) compressed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coding := coding(r.Header.Get("Accept-Encoding"))
		if coding == "" {
			next.ServeHTTP(w, r)
			return
		}
		packer := &packer{ResponseWriter: w, coding: coding}
		next.ServeHTTP(packer, r)
		packer.finish(a)
	})
}

func coding(accept string) string {
	if accepts(accept, Brotli) {
		return Brotli
	}
	if accepts(accept, Gzip) {
		return Gzip
	}
	return ""
}

func accepts(accept, coding string) bool {
	for part := range strings.SplitSeq(accept, ",") {
		if name, _, _ := strings.Cut(strings.TrimSpace(part), ";"); name == coding {
			return true
		}
	}
	return false
}

type packer struct {
	http.ResponseWriter
	coding  string
	stream  *compress.Stream
	packing bool
	decided bool
}

func (p *packer) WriteHeader(status int) {
	if status < http.StatusOK {
		p.ResponseWriter.WriteHeader(status)
		return
	}
	p.decide()
	if p.packing {
		p.Header().Del("Content-Length")
		p.Header().Set("Content-Encoding", p.coding)
	}
	p.Header().Add("Vary", "Accept-Encoding")
	p.ResponseWriter.WriteHeader(status)
}

func (p *packer) decide() {
	if p.decided {
		return
	}
	p.decided = true
	header := p.Header()
	if header.Get("Content-Encoding") != "" {
		return
	}
	kind := header.Get("Content-Type")
	length, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil {
		p.packing = compress.Streamable(kind)
	} else {
		p.packing = compress.Compressible(kind, length)
	}
	if p.packing {
		p.stream = compress.Open(p.coding, p.ResponseWriter)
	}
}

func (p *packer) Write(chunk []byte) (int, error) {
	if !p.decided {
		p.WriteHeader(http.StatusOK)
	}
	if !p.packing {
		return p.ResponseWriter.Write(chunk)
	}
	return p.stream.Write(chunk)
}

func (p *packer) Flush() {
	if p.packing {
		_ = p.stream.Flush()
	}
	if flusher, ok := p.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (p *packer) finish(a *App) {
	if !p.packing {
		return
	}
	if err := p.stream.Close(); err != nil {
		a.logger.Error("compression failed", "coding", p.coding, "error", err)
	}
}
