//go:build windows

package gcs

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := otelutil.StartSpan(ctx, "gcs::GuestConnection::CreateContainer", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", cid)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

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
	req := containerCreate{
		requestBase:     makeRequest(ctx, cid),
		ContainerConfig: anyInString{config},
	}
	var resp containerCreateResponse
	err = gc.brdg.RPC(ctx, rpcCreate, &req, &resp, false)
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
		_, span := otelutil.StartSpan(context.Background(), "gcs::Container::Close", trace.WithAttributes(
			attribute.String("cid", c.id)))
		defer span.End()

		close(c.closeCh)
	})
	return nil
}

// CreateProcess creates a process in the container.
func (c *Container) CreateProcess(ctx context.Context, config interface{}) (_ cow.Process, err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::Container::CreateProcess", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	return c.gc.exec(ctx, c.id, config)
}

// ID returns the container's ID.
func (c *Container) ID() string {
	return c.id
}

// Modify sends a modify request to the container.
func (c *Container) Modify(ctx context.Context, config interface{}) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::Container::Modify", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	req := containerModifySettings{
		requestBase: makeRequest(ctx, c.id),
		Request:     config,
	}
	var resp responseBase
	return c.gc.brdg.RPC(ctx, rpcModifySettings, &req, &resp, false)
}

// Properties returns the requested container properties targeting a V1 schema container.
func (c *Container) Properties(ctx context.Context, types ...schema1.PropertyType) (_ *schema1.ContainerProperties, err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::Container::Properties", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	req := containerGetProperties{
		requestBase: makeRequest(ctx, c.id),
		Query:       containerPropertiesQuery{PropertyTypes: types},
	}
	var resp containerGetPropertiesResponse
	err = c.gc.brdg.RPC(ctx, rpcGetProperties, &req, &resp, true)
	if err != nil {
		return nil, err
	}
	return (*schema1.ContainerProperties)(&resp.Properties), nil
}

// PropertiesV2 returns the requested container properties targeting a V2 schema container.
func (c *Container) PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (_ *hcsschema.Properties, err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::Container::PropertiesV2", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	req := containerGetPropertiesV2{
		requestBase: makeRequest(ctx, c.id),
		Query:       containerPropertiesQueryV2{PropertyTypes: types},
	}
	var resp containerGetPropertiesResponseV2
	err = c.gc.brdg.RPC(ctx, rpcGetProperties, &req, &resp, true)
	if err != nil {
		return nil, err
	}
	return (*hcsschema.Properties)(&resp.Properties), nil
}

// Start starts the container.
func (c *Container) Start(ctx context.Context) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::Container::Start", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	req := makeRequest(ctx, c.id)
	var resp responseBase
	return c.gc.brdg.RPC(ctx, rpcStart, &req, &resp, false)
}

func (c *Container) shutdown(ctx context.Context, proc rpcProc) error {
	req := makeRequest(ctx, c.id)
	var resp responseBase
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
	ctx, span := otelutil.StartSpan(ctx, "gcs::Container::Shutdown", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, rpcShutdownGraceful)
}

// Terminate sends a forceful terminate request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Terminate(ctx context.Context) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::Container::Terminate", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, rpcShutdownForced)
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
	ctx, span := otelutil.StartSpan(context.Background(), "gcs::Container::waitBackground", trace.WithAttributes(
		attribute.String("cid", c.id)))
	defer span.End()

	select {
	case <-c.notifyCh:
	case <-c.closeCh:
		c.waitError = errors.New("container closed")
	}
	close(c.waitBlock)

	log.G(ctx).Debug("container exited")
	otelutil.SetSpanStatus(span, c.waitError)
}
