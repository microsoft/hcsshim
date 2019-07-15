package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/timeout"
)

// syscallWatcher is used as a very simple goroutine around calls into
// the platform. In some cases, we have seen HCS APIs not returning due to
// various bugs, and the goroutine making the syscall ends up not returning,
// prior to its async callback. By spinning up a syscallWatcher, it allows
// us to at least log a warning if a syscall doesn't complete in a reasonable
// amount of time.
//
// Usage is:
//
// syscallWatcher(ctx, func() {
//    err = <syscall>(args...)
// })
//

func syscallWatcher(ctx context.Context, syscallLambda func()) {
	ctx, cancel := context.WithTimeout(ctx, timeout.SyscallWatcher)
	defer cancel()
	go watchFunc(ctx)
	syscallLambda()
}

func watchFunc(ctx context.Context) {
	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			log.G(ctx).
				WithField(logfields.Timeout, timeout.SyscallWatcher).
				Warning("Syscall did not complete within operation timeout. This may indicate a platform issue. If it appears to be making no forward progress, obtain the stacks and see if there is a syscall stuck in the platform API for a significant length of time.")
		}
	}
}
