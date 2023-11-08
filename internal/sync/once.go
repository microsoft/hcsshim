package sync

import (
	"context"
	"sync"
)

// TODO (go1.21): use pkg.go.dev/sync#OnceValues

// OnceValue is a wrapper around [sync.Once] that runs f only once and
// returns both a value (of type T) and an error.
func OnceValue[T any](f func() (T, error)) func() (T, error) {
	var (
		once sync.Once
		v    T
		err  error
	)

	return func() (T, error) {
		once.Do(func() {
			v, err = f()
		})
		return v, err
	}
}

// OnceValueCtx is similar to [OnceValue], but allows passing a context to f.
func OnceValueCtx[T any](f func(ctx context.Context) (T, error)) func(context.Context) (T, error) {
	var (
		once sync.Once
		v    T
		err  error
	)

	return func(ctx context.Context) (T, error) {
		once.Do(func() {
			v, err = f(ctx)
		})
		return v, err
	}
}
