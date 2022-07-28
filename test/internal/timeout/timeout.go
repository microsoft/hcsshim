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
func WaitForError(ctx context.Context, t testing.TB, f func() error, fe ErrorFunc) {
	var err error
	ch := make(chan struct{})

	go func() {
		err = f()
		close(ch)
	}()

	select {
	case <-ch:
		fatalOnError(t, fe, err)
	case <-ctx.Done():
		fatalOnError(t, fe, ctx.Err())
	}
}

func fatalOnError(t testing.TB, f func(error) error, err error) {
	if f != nil {
		err = f(err)
	}
	if err != nil {
		t.Helper()
		t.Fatal(err.Error())
	}
}
