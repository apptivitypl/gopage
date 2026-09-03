package cookie

import (
	"context"
	"net/http"
)

const HostPrefix = "__Host-"

type Options struct {
	Secure bool
}

type key struct{}

func With(ctx context.Context, opts Options) context.Context {
	return context.WithValue(ctx, key{}, opts)
}

func Of(r *http.Request) Options {
	if opts, ok := r.Context().Value(key{}).(Options); ok {
		return opts
	}
	return Options{Secure: r.TLS != nil}
}

func Name(base string, secure bool) string {
	if secure {
		return HostPrefix + base
	}
	return base
}

func Read(r *http.Request, base string) string {
	held, err := r.Cookie(Name(base, Of(r).Secure))
	if err != nil {
		return ""
	}
	return held.Value
}

func Set(w http.ResponseWriter, r *http.Request, base, value string) {
	http.SetCookie(w, shape(r, base, value, 0))
}

func Clear(w http.ResponseWriter, r *http.Request, base string) {
	http.SetCookie(w, shape(r, base, "", -1))
}

func shape(r *http.Request, base, value string, age int) *http.Cookie {
	secure := Of(r).Secure
	return &http.Cookie{
		Name:     Name(base, secure),
		Value:    value,
		Path:     "/",
		MaxAge:   age,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}
