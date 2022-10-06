// Package provides functionality for timing out operations and waiting
// for deadlines.
package timeout

import (
	"context"
	"testing"
	"time"
)

const ConnectTimeout = time.Second * 10

type ErrorFunc func(error) error

// WaitForError waits until f returns or the context is done.
// The returned error will be passed to the error processing functions fe, and (if non-nil),
// then passed to to [testing.Fatal].
//
// fe should expect nil errors.
func WaitForError(ctx context.Context, tb testing.TB, f func() error, fe ErrorFunc) {
	tb.Helper()
	var err error
	ch := make(chan struct{})

	go func() {
		err = f()
		close(ch)
	}()

	select {
	case <-ch:
		fatalOnError(tb, fe, err)
	case <-ctx.Done():
		fatalOnError(tb, fe, ctx.Err())
	}
}

func fatalOnError(tb testing.TB, f func(error) error, err error) {
	tb.Helper()
	if f != nil {
		err = f(err)
	}
	if err != nil {
		tb.Fatal(err.Error())
	}
}
