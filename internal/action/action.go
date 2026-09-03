package action

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/apptivitypl/rill/internal/cookie"
	"github.com/apptivitypl/rill/internal/redirect"
)

const (
	FlashCookie   = "rill.flash"
	RefreshHeader = "RILL-Refresh"
	FlashRoot     = "flash"
)

var (
	ErrNoTarget = errors.New("action: the redirect names neither a route nor a url")
	ErrTarget   = errors.New("action: the redirect target is neither a rooted path nor an http url")
)

type Resolver func(route string, params map[string]string) (string, error)

type Action interface {
	Apply(w http.ResponseWriter, r *http.Request, resolve Resolver) error
}

type Redirect struct {
	route    string
	location string
	params   map[string]string
	flash    string
	refresh  []string
	status   int
}

func To(location string) *Redirect {
	return &Redirect{location: location, status: http.StatusSeeOther}
}

func Route(name string) *Redirect {
	return &Redirect{route: name, status: http.StatusSeeOther}
}

func (r *Redirect) WithParam(name, value string) *Redirect {
	if r.params == nil {
		r.params = map[string]string{}
	}
	r.params[name] = value
	return r
}

func (r *Redirect) Flash(message string) *Redirect {
	r.flash = message
	return r
}

func (r *Redirect) Refresh(fragments ...string) *Redirect {
	r.refresh = append(r.refresh, fragments...)
	return r
}

func (r *Redirect) Status(code int) *Redirect {
	r.status = code
	return r
}

func (r *Redirect) Apply(w http.ResponseWriter, request *http.Request, resolve Resolver) error {
	target, err := r.target(resolve)
	if err != nil {
		return err
	}
	location, ok := redirect.Location(target)
	if !ok {
		return fmt.Errorf("%w: %q", ErrTarget, target)
	}
	if r.flash != "" {
		cookie.Set(w, request, FlashCookie, url.QueryEscape(r.flash))
	}
	if len(r.refresh) > 0 {
		w.Header().Set(RefreshHeader, strings.Join(r.refresh, ", "))
	}
	http.Redirect(w, request, location, r.status)
	return nil
}

func (r *Redirect) target(resolve Resolver) (string, error) {
	if r.location != "" {
		return r.location, nil
	}
	if r.route == "" || resolve == nil {
		return "", ErrNoTarget
	}
	return resolve(r.route, r.params)
}

func TakeFlash(w http.ResponseWriter, r *http.Request) string {
	held := cookie.Read(r, FlashCookie)
	if held == "" {
		return ""
	}
	cookie.Clear(w, r, FlashCookie)
	message, err := url.QueryUnescape(held)
	if err != nil {
		return ""
	}
	return message
}

func HasFlash(r *http.Request) bool {
	return cookie.Read(r, FlashCookie) != ""
}
