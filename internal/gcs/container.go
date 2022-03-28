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
	"github.com/Microsoft/hcsshim/internal/queue"
	"go.opencensus.io/trace"
)

const hrComputeSystemDoesNotExist = 0xc037010e

// Container implements the cow.Container interface for containers
// created via GuestConnection.
type Container struct {
	gc *GuestConnection
	id string
	// Blocks Wait() until waitBackground() returns or Close() is called.
	// Does not represent the status of the container in the guest
	waitBlock chan struct{}
	// set only in closeOnce, read after <-waitBlock returns
	waitError error
	closeOnce sync.Once
	// communication channels with the guest connection
	notifyChs     *notifyChans
	notifications *queue.MessageQueue
}

var _ cow.Container = &Container{}

func newContainer(gc *GuestConnection, id string) *Container {
	return &Container{
		gc:            gc,
		id:            id,
		waitBlock:     make(chan struct{}),
		notifyChs:     newNotifyChans(),
		notifications: queue.NewMessageQueue(),
	}
}

// CreateContainer creates a container using ID `cid` and `cfg`. The request
// will likely not be cancellable even if `ctx` becomes done.
func (gc *GuestConnection) CreateContainer(ctx context.Context, cid string, config interface{}) (_ *Container, err error) {
	ctx, span := trace.StartSpan(ctx, "gcs::GuestConnection::CreateContainer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", cid))

	c := newContainer(gc, cid)
	err = gc.requestNotify(cid, c.notifyChs)
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
	go c.waitBackground(ctx)
	go c.publishNotifications(ctx)
	return c, nil
}

// CloneContainer just creates the wrappers and sets up notification requests for a
// container that is already running inside the UVM (after cloning).
func (gc *GuestConnection) CloneContainer(ctx context.Context, cid string) (_ *Container, err error) {
	c := newContainer(gc, cid)
	err = gc.requestNotify(cid, c.notifyChs)
	if err != nil {
		return nil, err
	}
	go c.waitBackground(ctx)
	go c.publishNotifications(ctx)
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

// Close releases resources associated with the container, but does not terminate
// the container in the guest, or wait for it.
func (c *Container) Close() error {
	_, span := trace.StartSpan(context.Background(), "gcs::Container::Close")
	defer span.End()
	span.AddAttributes(trace.StringAttribute(logfields.ContainerID, c.id))

	c.closeOnce.Do(func() {
		c.waitError = errors.New("container closed")
		// `notifyChs` channels are closed by `GuestConnection`, so do not close here
		close(c.waitBlock)
		c.notifications.Close()
	})
	return nil
}

// CreateProcess creates a process in the container.
func (c *Container) CreateProcess(ctx context.Context, config interface{}) (_ cow.Process, err error) {
	ctx, span := trace.StartSpan(ctx, "gcs::Container::CreateProcess")
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
	ctx, span := trace.StartSpan(ctx, "gcs::Container::Modify")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	req := containerModifySettings{
		requestBase: makeRequest(ctx, c.id),
		Request:     config,
	}
	var resp responseBase
	return c.gc.brdg.RPC(ctx, rpcModifySettings, &req, &resp, false)
}

// Properties returns the requested container properties targeting a V1 schema container.
func (c *Container) Properties(ctx context.Context, types ...schema1.PropertyType) (_ *schema1.ContainerProperties, err error) {
	ctx, span := trace.StartSpan(ctx, "gcs::Container::Properties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

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
	ctx, span := trace.StartSpan(ctx, "gcs::Container::PropertiesV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

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
	ctx, span := trace.StartSpan(ctx, "gcs::Container::Start")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

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
		case <-c.waitBlock:
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
	ctx, span := trace.StartSpan(ctx, "gcs::Container::Shutdown")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, rpcShutdownGraceful)
}

// Terminate sends a forceful terminate request to the container. The container
// might not be terminated by the time the request completes (and might never
// terminate).
func (c *Container) Terminate(ctx context.Context) (err error) {
	ctx, span := trace.StartSpan(ctx, "gcs::Container::Terminate")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.shutdown(ctx, rpcShutdownForced)
}

// Wait waits for the container to terminate (or Close to be called, or the
// guest connection to terminate).
func (c *Container) Wait() error {
	<-c.waitBlock
	return c.waitError
}

// waitBackground waits for the GuestConnection to notify of container exit and
// unblocks other Wait calls
//
// This MUST be called via a goroutine to wait on a background thread, and can
// only be called once
func (c *Container) waitBackground(ctx context.Context) {
	var err error
	ctx, span := trace.StartSpan(ctx, "gcs::Container::waitBackground")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.ContainerID, c.id))

	// wait for container exit
	select {
	case <-c.notifyChs.close:
		// container in the guest was closed
	case <-c.waitBlock:
		// `c.Close()` was called
	}

	c.closeOnce.Do(func() {
		// `notifyChs` channels are closed by `GuestConnection`, so do not close here
		close(c.waitBlock)
		c.notifications.Close()
	})

	err = c.waitError
	log.G(ctx).Debug("container exited")
}

// Notifications returns a list of notifications (including shutdown) about the container
//
// The Notifications queue will be closed by Close or when the container in the
// guest exits
func (c *Container) Notifications() (*queue.MessageQueue, error) {
	return c.notifications, nil
}

// publishNotifications publishes notifications from the container in the guest
// that are forwarded by the GuestConnection. Currently only OOM events are published.
//
// This MUST be called via a goroutine to wait on a background thread.
func (c *Container) publishNotifications(ctx context.Context) {
	var err error
	ctx, span := trace.StartSpan(ctx, "gcs::Container::publishNotifications")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.ContainerID, c.id))

	// read in notifications while waiting for container to exit
	for {
		select {
		case <-c.waitBlock:
			// `c.Close` was called
			err = c.waitError
			return
		case <-c.notifyChs.close:
			// container was closed
			return
		case ntf, ok := <-c.notifyChs.notify:
			if !ok {
				// GuestConnection should close c.notifyChs.close first, before
				// closing c.notifyChs.notify, but, just in case, guard againt
				// reading from a closed channel
				return
			}
			// enqueue the notification; will block until write succeeds
			err = c.notifications.Write(ntf)
			if err != nil {
				log.G(ctx).WithError(err).
					WithField("notification", ntf.String()).
					Error("could not enqueue notification")
			}
		}
	}

}
