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

type Slot struct {
	Policy   *Recorder
	Response any
}

func With(ctx context.Context, slot *Slot) context.Context {
	return context.WithValue(ctx, recorderKey{}, slot)
}

func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	return With(ctx, &Slot{Policy: recorder, Response: Response(ctx)})
}

func WithResponse(ctx context.Context, response any) context.Context {
	return With(ctx, &Slot{Policy: held(ctx).Policy, Response: response})
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
