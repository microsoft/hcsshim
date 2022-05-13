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
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/sirupsen/logrus"
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
}

var _ cow.Container = &Container{}

// CreateContainer creates a container using ID `cid` and `cfg`. The request
// will likely not be cancellable even if `ctx` becomes done.
func (gc *GuestConnection) CreateContainer(ctx context.Context, cid string, config interface{}) (_ *Container, err error) {
	log.G(ctx).WithFields(logrus.Fields{
		logfields.ContainerID: cid,
	}).Trace("gcs::GuestConnection:: CreateContainer")

	c := &Container{
		gc:       gc,
		id:       cid,
		notifyCh: make(chan struct{}),
		closeCh:  make(chan struct{}),
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
		gc:       gc,
		id:       cid,
		notifyCh: make(chan struct{}),
		closeCh:  make(chan struct{}),
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
	c.logEntry(context.Background()).Trace("gcs::Container::Close")

	c.closeOnce.Do(func() {})
	return nil
}

// CreateProcess creates a process in the container.
func (c *Container) CreateProcess(ctx context.Context, config interface{}) (_ cow.Process, err error) {
	c.logEntry(ctx).Trace("gcs::Container::CreateProcess")

	return c.gc.exec(ctx, c.id, config)
}

// ID returns the container's ID.
func (c *Container) ID() string {
	return c.id
}

// Modify sends a modify request to the container.
func (c *Container) Modify(ctx context.Context, config interface{}) (err error) {
	c.logEntry(ctx).Trace("gcs::Container::Modify")

	req := containerModifySettings{
		requestBase: makeRequest(ctx, c.id),
		Request:     config,
	}
	var resp responseBase
	return c.gc.brdg.RPC(ctx, rpcModifySettings, &req, &resp, false)
}

// Properties returns the requested container properties targeting a V1 schema container.
func (c *Container) Properties(ctx context.Context, types ...schema1.PropertyType) (_ *schema1.ContainerProperties, err error) {
	c.logEntry(ctx).Trace("gcs::Container::Properties")

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
	c.logEntry(ctx).Trace("gcs::Container::PropertiesV2")

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
	c.logEntry(ctx).Trace("gcs::Container::Start")

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
			c.logEntry(ctx).WithError(err).Warn("ignoring missing container")
		}
	}
	return nil
}

// Shutdown sends a graceful shutdown request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Shutdown(ctx context.Context) (err error) {
	c.logEntry(ctx).Trace("gcs::Container::Shutdown")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, rpcShutdownGraceful)
}

// Terminate sends a forceful terminate request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Terminate(ctx context.Context) (err error) {
	c.logEntry(ctx).Trace("gcs::Container::Terminate")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, rpcShutdownForced)
}

// Wait waits for the container to terminate (or Close to be called, or the
// guest connection to terminate).
func (c *Container) Wait() error {
	select {
	case <-c.notifyCh:
		return nil
	case <-c.closeCh:
		return errors.New("container closed")
	}
}

func (c *Container) waitBackground() {
	var err error
	_, span := oc.StartSpan(context.Background(), "gcs::Container::waitBackground")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	err = c.Wait()
}

func (c *Container) logEntry(ctx context.Context) *logrus.Entry {
	return log.G(ctx).WithField(logfields.ContainerID, c.id)
}
