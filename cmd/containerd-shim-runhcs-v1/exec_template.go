package main

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func newTemplateExec(
	ctx context.Context,
	events publisher,
	tid string,
	host *uvm.UtilityVM,
	c cow.Container,
	id, bundle string,
	isWCOW bool,
	spec *specs.Process,
	io upstreamIO) shimExec {
	log.G(ctx).WithFields(logrus.Fields{
		"tid":    tid,
		"eid":    id, // Init exec ID is always same as Task ID
		"bundle": bundle,
		"wcow":   isWCOW,
	}).Debug("newTemplateExec")

	if !isWCOW {
		log.G(ctx).Error("Template exec creation request for non WCOW container / pod")
		return nil
	}

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

	te := &templateExec{
		he,
	}
	// Even for a template task it can happen that container exits for some reason
	// before init process is even started. So wait on this event.
	go te.waitForContainerExit()
	return te
}

var _ = (shimExec)(&templateExec{})

// Template Exec is almost exactly same as hcsExec. The only difference is that after starting
// the template exec we save the VM. This saved VM can never be resumed but it can be killed/
// destroyed. Hence, when a container is started inside this VM with the saveastemplate
// annotation the state of this VM will immediately switch to Stopped. It can only be
// deleted then.
type templateExec struct {
	*hcsExec
}

func (te *templateExec) Start(ctx context.Context) (err error) {

	err = te.hcsExec.Start(ctx)
	if err != nil {
		return err
	}

	// Now save this host as at template
	if err = SaveAsTemplate(ctx, te.host); err != nil {
		return err
	}

	return nil
}
