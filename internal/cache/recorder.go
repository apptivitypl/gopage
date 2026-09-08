package cache

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
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

type Slot struct {
	Policy   *Recorder
	Response any
	Shared   *singleflight.Group
}

func With(ctx context.Context, slot *Slot) context.Context {
	return context.WithValue(ctx, recorderKey{}, slot)
}

func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	return With(ctx, &Slot{Policy: recorder, Response: Response(ctx), Shared: Shared(ctx)})
}

func WithResponse(ctx context.Context, response any) context.Context {
	current := held(ctx)
	return With(ctx, &Slot{Policy: current.Policy, Response: response, Shared: current.Shared})
}

func Shared(ctx context.Context) *singleflight.Group {
	return held(ctx).Shared
}

func From(ctx context.Context) *Recorder {
	if recorder := held(ctx).Policy; recorder != nil {
		return recorder
	}
	return NewRecorder()
}

func Response(ctx context.Context) any {
	return held(ctx).Response
}

func held(ctx context.Context) Slot {
	current, _ := ctx.Value(recorderKey{}).(*Slot)
	if current == nil {
		return Slot{}
	}
	return *current
}

func Shorter(current, candidate time.Duration) time.Duration {
	if candidate <= 0 {
		return current
	}
	if current <= 0 || candidate < current {
		return candidate
	}
	return current
}

func (r *Recorder) Merge(other *Recorder) {
	if other == nil {
		return
	}
	if !other.Shared() {
		r.Private()
		return
	}
	policy := other.Policy()
	r.mu.Lock()
	r.policy.TTL = Shorter(r.policy.TTL, policy.TTL)
	r.policy.Stale = Shorter(r.policy.Stale, policy.Stale)
	r.mu.Unlock()
	r.Tag(other.Tags()...)
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
