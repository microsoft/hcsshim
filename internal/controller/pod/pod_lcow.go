//go:build windows && lcow

package pod

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer"
	"github.com/Microsoft/hcsshim/internal/controller/network"
)

// Controller manages the lifecycle of a single pod inside a Utility VM.
type Controller struct {
	mu sync.RWMutex

	// podID is the containerd facing pod identifier.
	podID string

	// gcsPodID is the identifier used when communicating with the GCS.
	gcsPodID string

	// vm is the parent Utility VM that hosts this pod.
	vm vmController

	// network manages the network namespace and endpoint lifecycle
	// for this pod.
	network networkController

	// containers maps containerID → [linuxcontainer.Controller] for every
	// live container in this pod. Access must be guarded by mu.
	containers map[string]*linuxcontainer.Controller
}

// New creates a ready-to-use [Controller] for the given pod.
func New(
	podID string,
	vm vmController,
) *Controller {
	return &Controller{
		podID: podID,
		// Same ID is used as the pod. Post migration, we can always change
		// the primary ID while GCS continues to use the original one.
		gcsPodID:   podID,
		vm:         vm,
		network:    vm.NetworkController(),
		containers: make(map[string]*linuxcontainer.Controller),
	}
}

// SetupNetwork performs network setup for the pod.
func (c *Controller) SetupNetwork(ctx context.Context, opts *network.SetupOptions) error {
	if err := c.network.Setup(ctx, opts); err != nil {
		return fmt.Errorf("setup network for pod %s: %w", c.podID, err)
	}
	return nil
}

// TeardownNetwork performs network teardown for the pod.
func (c *Controller) TeardownNetwork(ctx context.Context) error {
	if err := c.network.Teardown(ctx); err != nil {
		return fmt.Errorf("teardown network for pod %s: %w", c.podID, err)
	}
	return nil
}

// GetContainer returns the container controller for the given containerID.
func (c *Controller) GetContainer(containerID string) (*linuxcontainer.Controller, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	containerCtrl, ok := c.containers[containerID]
	if !ok {
		return nil, fmt.Errorf("container %q not found in pod %q", containerID, c.podID)
	}

	return containerCtrl, nil
}

// NewContainer creates a new [linuxcontainer.Controller] and registers it
// in this pod.
func (c *Controller) NewContainer(ctx context.Context, containerID string) (*linuxcontainer.Controller, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure we don't create a duplicate container controller.
	if _, ok := c.containers[containerID]; ok {
		return nil, fmt.Errorf("container %q already exists in pod %q", containerID, c.podID)
	}

	containerCtrl := linuxcontainer.New(
		c.vm.RuntimeID(),
		c.gcsPodID,
		containerID,
		c.vm.Guest(),
		c.vm.SCSIController(),
		c.vm.Plan9Controller(),
		c.vm.VPCIController(),
	)
	c.containers[containerID] = containerCtrl
	return containerCtrl, nil
}

// ListContainers returns a snapshot of all live container controllers in
// this pod, keyed by container ID.
func (c *Controller) ListContainers() map[string]*linuxcontainer.Controller {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*linuxcontainer.Controller, len(c.containers))
	for containerID, containerCtrl := range c.containers {
		result[containerID] = containerCtrl
	}

	return result
}

// DeleteContainer removes a container from the pod's container map.
func (c *Controller) DeleteContainer(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.containers[containerID]; !ok {
		return fmt.Errorf("container %q not found in pod %q", containerID, c.podID)
	}

	delete(c.containers, containerID)
	return nil
}
