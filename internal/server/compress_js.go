//go:build js

package server

import "net/http"

func (a *App) compressed(next http.Handler) http.Handler {
	return next
}
