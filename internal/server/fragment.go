package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/apptivitypl/rill/internal/cache"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/runtime"
)

const fragmentPrefix = "fragment\x1f"

type fragmentCache struct {
	cache    *cache.Cache
	locale   string
	recorder *cache.Recorder
}

func (a *App) fragmentHook(r *http.Request) runtime.Fragments {
	if a.cache == nil {
		return nil
	}
	return fragmentCache{cache: a.cache, locale: LocaleOf(r), recorder: cache.From(r.Context())}
}

func (f fragmentCache) key(key string) string {
	return fragmentPrefix + strconv.Itoa(len(f.locale)) + ":" + f.locale + "\x1f" + key
}

func (f fragmentCache) Load(_ ir.Fragment, key string) ([]byte, bool) {
	return f.cache.Peek(f.key(key))
}

func (f fragmentCache) Save(fragment ir.Fragment, key string, body []byte) {
	if f.recorder != nil && !f.recorder.Shared() {
		return
	}
	stored := make([]byte, len(body))
	copy(stored, body)
	f.cache.Put(f.key(key), cache.Value{Body: stored},
		cache.Policy{TTL: time.Duration(fragment.TTL), Stale: time.Duration(fragment.Stale)})
}
