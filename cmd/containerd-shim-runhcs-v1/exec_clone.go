package main

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
)

func newClonedExec(ctx context.Context, events publisher, tid, bundle string, io upstreamIO) *clonedExec {
	log.G(ctx).WithFields(logrus.Fields{
		"tid":    tid,
		"eid":    tid, // Init exec ID is always same as Task ID
		"bundle": bundle,
	}).Debug("newClonedExec")

	wpse := &wcowPodSandboxExec{
		events:     events,
		tid:        tid,
		bundle:     bundle,
		state:      shimExecStateCreated,
		exitStatus: 255, // By design for non-exited process status.
		exited:     make(chan struct{}),
	}

	ce := &clonedExec{
		wpse,
		io,
	}
	return ce
}

var _ = (shimExec)(&clonedExec{})

// ClonedExec is almost the same as wcowPodSandboxExec in that it doesn't actually track
// a process (it is just a dummy exec). However, Cloned exec tracks the IO channels passed
// with the request and it closes them correctly when the exec kill is called.
type clonedExec struct {
	*wcowPodSandboxExec
	io upstreamIO
}

func (ce *clonedExec) Kill(ctx context.Context, signal uint32) error {
	err := ce.wcowPodSandboxExec.Kill(ctx, signal)
	ce.io.Close(ctx)
	return err
}

func (ce *clonedExec) ForceExit(ctx context.Context, status int) {
	ce.wcowPodSandboxExec.ForceExit(ctx, status)
	ce.io.Close(ctx)
}
