package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"net/http"

	"github.com/apptivitypl/gopage/internal/cookie"
)

const (
	Field      = "__csrf"
	CookieName = "gopage.csrf"
	size       = 32
)

var ErrRejected = errors.New("csrf: the token is missing or does not match")

type Source = io.Reader

func Generate(source Source) (string, error) {
	if source == nil {
		source = rand.Reader
	}
	buffer := make([]byte, size)
	if _, err := io.ReadFull(source, buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func Token(r *http.Request) string {
	return cookie.Read(r, CookieName)
}

func Issue(w http.ResponseWriter, r *http.Request, source Source) (string, error) {
	token := Token(r)
	if token == "" {
		fresh, err := Generate(source)
		if err != nil {
			return "", err
		}
		cookie.Set(w, r, CookieName, fresh)
		token = fresh
	}
	return Mask(token, source)
}

func Mask(token string, source Source) (string, error) {
	secret, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(secret) == 0 {
		return "", ErrRejected
	}
	if source == nil {
		source = rand.Reader
	}
	out := make([]byte, 2*len(secret))
	if _, err := io.ReadFull(source, out[:len(secret)]); err != nil {
		return "", err
	}
	for i, b := range secret {
		out[len(secret)+i] = b ^ out[i]
	}
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func unmask(masked string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(masked)
	if err != nil || len(raw) == 0 || len(raw)%2 != 0 {
		return nil, false
	}
	half := len(raw) / 2
	secret := make([]byte, half)
	for i := range secret {
		secret[i] = raw[half+i] ^ raw[i]
	}
	return secret, true
}

func Verify(r *http.Request, submitted string) error {
	token := Token(r)
	if token == "" || submitted == "" {
		return ErrRejected
	}
	held, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ErrRejected
	}
	sent, ok := unmask(submitted)
	if !ok {
		return ErrRejected
	}
	if subtle.ConstantTimeCompare(held, sent) != 1 {
		return ErrRejected
	}
	return nil
}
