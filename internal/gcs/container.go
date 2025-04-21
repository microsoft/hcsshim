//go:build windows

package gcs

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"go.opencensus.io/trace"
)

const hrComputeSystemDoesNotExist = 0xc037010e

// Container implements the cow.Container interface for containers
// created via GuestConnection.
type Container struct {
	gc        *GuestConnection
	id        string
	notifyCh  chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
	// waitBlock is the channel used to wait for container shutdown or termination
	waitBlock chan struct{}
	// waitError indicates the container termination error if any
	waitError error
}

var _ cow.Container = &Container{}

// CreateContainer creates a container using ID `cid` and `cfg`. The request
// will likely not be cancellable even if `ctx` becomes done.
func (gc *GuestConnection) CreateContainer(ctx context.Context, cid string, config interface{}) (_ *Container, err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::GuestConnection::CreateContainer", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", cid))

	c := &Container{
		gc:        gc,
		id:        cid,
		notifyCh:  make(chan struct{}),
		closeCh:   make(chan struct{}),
		waitBlock: make(chan struct{}),
	}
	err = gc.requestNotify(cid, c.notifyCh)
	if err != nil {
		return nil, err
	}
	req := prot.ContainerCreate{
		RequestBase:     makeRequest(ctx, cid),
		ContainerConfig: prot.AnyInString{config},
	}
	var resp prot.ContainerCreateResponse
	err = gc.brdg.RPC(ctx, prot.RpcCreate, &req, &resp, false)
	if err != nil {
		return nil, err
	}
	go c.waitBackground()
	return c, nil
}

// CloneContainer just creates the wrappers and sets up notification requests for a
// container that is already running inside the UVM (after cloning).
func (gc *GuestConnection) CloneContainer(ctx context.Context, cid string) (_ *Container, err error) {
	c := &Container{
		gc:        gc,
		id:        cid,
		notifyCh:  make(chan struct{}),
		closeCh:   make(chan struct{}),
		waitBlock: make(chan struct{}),
	}
	err = gc.requestNotify(cid, c.notifyCh)
	if err != nil {
		return nil, err
	}
	go c.waitBackground()
	return c, nil
}

// OS returns the operating system of the container, "linux" or "windows".
func (c *Container) OS() string {
	return c.gc.os
}

// IsOCI specifies whether CreateProcess should be called with an OCI
// specification in its input.
func (c *Container) IsOCI() bool {
	return c.gc.os != "windows"
}

// Close releases associated with the container.
func (c *Container) Close() error {
	c.closeOnce.Do(func() {
		_, span := oc.StartSpan(context.Background(), "gcs::Container::Close")
		defer span.End()
		span.AddAttributes(trace.StringAttribute("cid", c.id))

		close(c.closeCh)
	})
	return nil
}

// CreateProcess creates a process in the container.
func (c *Container) CreateProcess(ctx context.Context, config interface{}) (_ cow.Process, err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Container::CreateProcess", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	return c.gc.exec(ctx, c.id, config)
}

// ID returns the container's ID.
func (c *Container) ID() string {
	return c.id
}

// Modify sends a modify request to the container.
func (c *Container) Modify(ctx context.Context, config interface{}) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Container::Modify", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	req := prot.ContainerModifySettings{
		RequestBase: makeRequest(ctx, c.id),
		Request:     config,
	}
	var resp prot.ResponseBase
	return c.gc.brdg.RPC(ctx, prot.RpcModifySettings, &req, &resp, false)
}

// Properties returns the requested container properties targeting a V1 schema prot.Container.
func (c *Container) Properties(ctx context.Context, types ...schema1.PropertyType) (_ *schema1.ContainerProperties, err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Container::Properties", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	req := prot.ContainerGetProperties{
		RequestBase: makeRequest(ctx, c.id),
		Query:       prot.ContainerPropertiesQuery{PropertyTypes: types},
	}
	var resp prot.ContainerGetPropertiesResponse
	err = c.gc.brdg.RPC(ctx, prot.RpcGetProperties, &req, &resp, true)
	if err != nil {
		return nil, err
	}
	return (*schema1.ContainerProperties)(&resp.Properties), nil
}

// PropertiesV2 returns the requested container properties targeting a V2 schema container.
func (c *Container) PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (_ *hcsschema.Properties, err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Container::PropertiesV2", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	req := prot.ContainerGetPropertiesV2{
		RequestBase: makeRequest(ctx, c.id),
		Query:       prot.ContainerPropertiesQueryV2{PropertyTypes: types},
	}
	var resp prot.ContainerGetPropertiesResponseV2
	err = c.gc.brdg.RPC(ctx, prot.RpcGetProperties, &req, &resp, true)
	if err != nil {
		return nil, err
	}
	return (*hcsschema.Properties)(&resp.Properties), nil
}

// Start starts the container.
func (c *Container) Start(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Container::Start", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	req := makeRequest(ctx, c.id)
	var resp prot.ResponseBase
	return c.gc.brdg.RPC(ctx, prot.RpcStart, &req, &resp, false)
}

func (c *Container) shutdown(ctx context.Context, proc prot.RpcProc) error {
	req := makeRequest(ctx, c.id)
	var resp prot.ResponseBase
	err := c.gc.brdg.RPC(ctx, proc, &req, &resp, true)
	if err != nil {
		if uint32(resp.Result) != hrComputeSystemDoesNotExist {
			return err
		}
		select {
		case <-c.notifyCh:
		default:
			log.G(ctx).WithError(err).Warn("ignoring missing container")
		}
	}
	return nil
}

// Shutdown sends a graceful shutdown request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Shutdown(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Container::Shutdown", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, prot.RpcShutdownGraceful)
}

// Terminate sends a forceful terminate request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Terminate(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Container::Terminate", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, prot.RpcShutdownForced)
}

func (c *Container) WaitChannel() <-chan struct{} {
	return c.waitBlock
}

func (c *Container) WaitError() error {
	return c.waitError
}

// Wait waits for the container to terminate (or Close to be called, or the
// guest connection to terminate).
func (c *Container) Wait() error {
	<-c.WaitChannel()
	return c.WaitError()
}

func (c *Container) waitBackground() {
	ctx, span := oc.StartSpan(context.Background(), "gcs::Container::waitBackground")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	select {
	case <-c.notifyCh:
	case <-c.closeCh:
		c.waitError = errors.New("container closed")
	}
	close(c.waitBlock)

	log.G(ctx).Debug("container exited")
	oc.SetSpanStatus(span, c.waitError)
}
