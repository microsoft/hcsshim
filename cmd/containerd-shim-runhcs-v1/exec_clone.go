package main

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func newClonedExec(
	ctx context.Context,
	events publisher,
	tid string,
	host *uvm.UtilityVM,
	c cow.Container,
	id, bundle string,
	isWCOW bool,
	spec *specs.Process,
	io cmd.UpstreamIO) *clonedExec {
	log.G(ctx).WithFields(logrus.Fields{
		"tid":    tid,
		"eid":    id, // Init exec ID is always same as Task ID
		"bundle": bundle,
	}).Debug("newClonedExec")

	he := &hcsExec{
		events:      events,
		tid:         tid,
		host:        host,
		c:           c,
		id:          id,
		bundle:      bundle,
		isWCOW:      isWCOW,
		spec:        spec,
		io:          io,
		processDone: make(chan struct{}),
		state:       shimExecStateCreated,
		exitStatus:  255, // By design for non-exited process status.
		exited:      make(chan struct{}),
	}

	ce := &clonedExec{
		he,
	}
	go he.waitForContainerExit()
	return ce
}

var _ = (shimExec)(&clonedExec{})

// clonedExec inherits from hcsExec. The only difference between these two is that
// on starting a clonedExec it doesn't attempt to start the container even if the
// exec is the init process. This is because in case of clonedExec the container is
// already running inside the pod.
type clonedExec struct {
	*hcsExec
}

func (ce *clonedExec) Start(ctx context.Context) (err error) {
	// A cloned exec should never initialize the container as it should
	// already be running.
	return ce.startInternal(ctx, false)
}
