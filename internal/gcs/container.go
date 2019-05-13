package gcs

import (
	"context"
	"errors"
	"sync"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/schema1"
	"github.com/sirupsen/logrus"
)

const (
	hrComputeSystemDoesNotExist = 0xc037010e
)

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
func (gc *GuestConnection) CreateContainer(ctx context.Context, cid string, cfg interface{}) (*Container, error) {
	c := &Container{
		gc:       gc,
		id:       cid,
		notifyCh: make(chan struct{}),
		closeCh:  make(chan struct{}),
	}
	err := gc.requestNotify(cid, c.notifyCh)
	if err != nil {
		return nil, err
	}
	req := containerCreate{
		requestBase:     makeRequest(cid),
		ContainerConfig: anyInString{cfg},
	}
	var resp containerCreateResponse
	err = gc.brdg.RPC(ctx, rpcCreate, &req, &resp, false)
	if err != nil {
		return nil, err
	}
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
		close(c.closeCh)
	})
	return nil
}

// CreateProcess creates a process in the container.
func (c *Container) CreateProcess(config interface{}) (cow.Process, error) {
	return c.gc.exec(context.TODO(), c.id, config)
}

// ID returns the container's ID.
func (c *Container) ID() string {
	return c.id
}

// Modify sends a modify request to the container.
func (c *Container) Modify(config interface{}) (err error) {
	req := containerModifySettings{
		requestBase: makeRequest(c.id),
		Request:     config,
	}
	var resp responseBase
	return c.gc.brdg.RPC(context.TODO(), rpcModifySettings, &req, &resp, false)
}

// Properties requests properties of the container.
func (c *Container) Properties(types ...schema1.PropertyType) (_ *schema1.ContainerProperties, err error) {
	req := containerGetProperties{
		requestBase: makeRequest(c.id),
		Query:       containerPropertiesQuery{PropertyTypes: types},
	}
	var resp containerGetPropertiesResponse
	err = c.gc.brdg.RPC(context.TODO(), rpcGetProperties, &req, &resp, true)
	if err != nil {
		return nil, err
	}
	return (*schema1.ContainerProperties)(&resp.Properties), nil
}

// Start starts the container.
func (c *Container) Start() error {
	req := makeRequest(c.id)
	var resp responseBase
	return c.gc.brdg.RPC(context.TODO(), rpcStart, &req, &resp, false)
}

func (c *Container) shutdown(ctx context.Context, proc rpcProc) error {
	req := makeRequest(c.id)
	var resp responseBase
	err := c.gc.brdg.RPC(ctx, proc, &req, &resp, true)
	if err != nil {
		if uint32(resp.Result) != hrComputeSystemDoesNotExist {
			return err
		}
		select {
		case <-c.notifyCh:
		default:
			logrus.WithFields(logrus.Fields{
				logrus.ErrorKey:       err,
				logfields.ContainerID: c.id,
			}).Warn("ignoring missing container")
		}
	}
	return nil
}

// Shutdown sends a graceful shutdown request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Shutdown() error {
	return c.shutdown(context.TODO(), rpcShutdownGraceful)
}

// Terminate sends a forceful terminate request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Terminate() error {
	return c.shutdown(context.TODO(), rpcShutdownForced)
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
