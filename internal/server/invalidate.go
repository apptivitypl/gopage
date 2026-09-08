package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

const (
	InvalidatePath = "/_gopage/invalidate"
	maxTagsBody    = 64 << 10
)

type invalidation struct {
	Tags []string `json:"tags"`
}

func (a *App) invalidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.bearer(r) {
		a.logger.Warn("invalidation refused", "path", r.URL.Path)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var asked invalidation
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTagsBody)).Decode(&asked); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	dropped := a.Invalidate(asked.Tags...)
	a.logger.Info("invalidated", "tags", asked.Tags, "entries", dropped)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"dropped":` + strconv.Itoa(dropped) + `}`)); err != nil {
		a.logger.Error("write failed", "path", r.URL.Path, "error", err)
	}
}

func (a *App) bearer(r *http.Request) bool {
	given, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(given), []byte(a.token)) == 1
}
