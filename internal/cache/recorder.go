package cache

import (
	"context"
	"sync"
	"time"
)

type recorderKey struct{}

type Recorder struct {
	mu      sync.Mutex
	policy  Policy
	tags    []string
	private bool
}

func NewRecorder() *Recorder {
	return &Recorder{}
}

func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, recorder)
}

func From(ctx context.Context) *Recorder {
	recorder, _ := ctx.Value(recorderKey{}).(*Recorder)
	if recorder == nil {
		return NewRecorder()
	}
	return recorder
}

func (r *Recorder) TTL(d time.Duration) *Recorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy.TTL = d
	return r
}

func (r *Recorder) Stale(d time.Duration) *Recorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy.Stale = d
	return r
}

func (r *Recorder) Tag(tags ...string) *Recorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tags = append(r.tags, tags...)
	return r
}

func (r *Recorder) Private() *Recorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.private = true
	return r
}

func (r *Recorder) Shared() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.private
}

func (r *Recorder) Policy() Policy {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.private {
		return Policy{}
	}
	return r.policy
}

func (r *Recorder) Tags() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.tags) == 0 {
		return nil
	}
	return append([]string(nil), r.tags...)
}
