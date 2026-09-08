package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/reply"
)

const bucketSeparator = "\x1f"

type varyKey struct{}

type Buckets struct {
	dimensions []ir.Vary
	values     []string
}

func bucketsOf(r *http.Request, route ir.Route) *Buckets {
	if len(route.Vary) == 0 {
		return nil
	}
	held := &Buckets{dimensions: route.Vary, values: make([]string, len(route.Vary))}
	for index, dimension := range route.Vary {
		held.values[index] = dimension.Bucket(rawVary(r, dimension))
	}
	return held
}

func rawVary(r *http.Request, dimension ir.Vary) string {
	if dimension.Kind == ir.VaryHeader {
		return r.Header.Get(dimension.Name)
	}
	if held, err := r.Cookie(dimension.Name); err == nil {
		return held.Value
	}
	return ""
}

func (b *Buckets) Variant() string {
	if b == nil {
		return ""
	}
	return strings.Join(b.values, bucketSeparator)
}

func (b *Buckets) Cookie(name string) (string, bool) {
	if b == nil {
		return "", false
	}
	for index, dimension := range b.dimensions {
		if dimension.Kind == ir.VaryCookie && strings.EqualFold(dimension.Name, name) {
			return b.values[index], true
		}
	}
	return "", false
}

func (b *Buckets) Headers() []string {
	if b == nil {
		return nil
	}
	var names []string
	cookies := false
	for _, dimension := range b.dimensions {
		if dimension.Kind == ir.VaryHeader {
			names = append(names, dimension.Name)
			continue
		}
		if !cookies {
			cookies = true
			names = append(names, reply.CookieVary)
		}
	}
	return names
}

func WithBuckets(ctx context.Context, held *Buckets) context.Context {
	if held == nil {
		return ctx
	}
	return context.WithValue(ctx, varyKey{}, held)
}

func BucketsFrom(ctx context.Context) *Buckets {
	held, _ := ctx.Value(varyKey{}).(*Buckets)
	return held
}
