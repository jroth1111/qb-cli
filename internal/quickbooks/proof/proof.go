// Package proof carries an in-process, adapter-issued independent-readback
// receipt. API response fields cannot manufacture this completion signal.
package proof

import (
	"context"
	"sync/atomic"
)

type key struct{}
type scope struct{ complete, submitted atomic.Bool }

func WithScope(ctx context.Context) context.Context { return context.WithValue(ctx, key{}, new(scope)) }
func Confirm(ctx context.Context) {
	if s, ok := ctx.Value(key{}).(*scope); ok {
		s.complete.Store(true)
	}
}
func Confirmed(ctx context.Context) bool {
	s, ok := ctx.Value(key{}).(*scope)
	return ok && s.complete.Load()
}

func Submitted(ctx context.Context) {
	if s, ok := ctx.Value(key{}).(*scope); ok {
		s.submitted.Store(true)
		s.complete.Store(false)
	}
}
func WasSubmitted(ctx context.Context) bool {
	s, ok := ctx.Value(key{}).(*scope)
	return ok && s.submitted.Load()
}
