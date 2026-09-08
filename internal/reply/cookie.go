package reply

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/apptivitypl/gopage/internal/cookie"
)

const (
	MaxCookieBytes = 4096
	reservedPrefix = "gopage."
	rootPath       = "/"
)

var (
	ErrReserved = errors.New("the name belongs to gopage")
	ErrHosted   = errors.New("a __Host- cookie may not carry a domain and must sit at /")
	ErrTooBig   = errors.New("a cookie over 4096 bytes is dropped by browsers")
)

func Check(held *http.Cookie, secure bool) (*http.Cookie, error) {
	if held == nil {
		return nil, errors.New("no cookie")
	}
	if strings.HasPrefix(strings.TrimPrefix(held.Name, cookie.HostPrefix), reservedPrefix) {
		return nil, ErrReserved
	}
	shaped := *held
	if shaped.Path == "" {
		shaped.Path = rootPath
	}
	if shaped.SameSite == 0 {
		shaped.SameSite = http.SameSiteLaxMode
	}
	if secure {
		shaped.Secure = true
	}
	if strings.HasPrefix(shaped.Name, cookie.HostPrefix) && (shaped.Domain != "" || shaped.Path != rootPath) {
		return nil, ErrHosted
	}
	if len(shaped.Name)+len(shaped.Value) > MaxCookieBytes {
		return nil, ErrTooBig
	}
	if err := shaped.Valid(); err != nil {
		return nil, fmt.Errorf("cookie is malformed: %w", err)
	}
	return &shaped, nil
}
