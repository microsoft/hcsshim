// This package holds common synchronization primitives used in test writing
package sync

import (
	"sync"
	"testing"
)

type LazyString struct {
	once sync.Once
	f    func() (string, error)
	s    string
	err  error
}

func NewLazyString(f func() (string, error)) *LazyString {
	return &LazyString{f: f}
}

func (x *LazyString) String(tb testing.TB) string {
	tb.Helper()
	// since we cannot call test.Helper in the function once.Do uses to call f, avoid
	// tests and panics within the once, and save it for the outside.
	x.once.Do(func() { x.s, x.err = x.f() })
	if x.err != nil {
		tb.Fatalf("lazy string load failed: %v", x.err)
	}
	return x.s
}
