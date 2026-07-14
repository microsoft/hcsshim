//go:build windows && wcowprocess

package pod

import (
	"context"
	"fmt"
	"sync"
	"time"

	container "github.com/Microsoft/hcsshim/internal/controller/windowscontainer/processisolated"
	"github.com/Microsoft/hcsshim/internal/log"
)

// Controller manages the lifecycle of a single process-isolated WCOW pod on the host.
type Controller struct {
	mu sync.RWMutex

	// podID is the containerd facing pod identifier.
	podID string

	// ioRetryTimeout is the IO connection retry timeout applied to every
	// container created in this pod.
	ioRetryTimeout time.Duration

	// containers maps containerID → [container.Controller] for every live
	// container in this pod. Access must be guarded by mu.
	containers map[string]*container.Controller
}

// New creates a ready-to-use [Controller] for the given pod.
func New(podID string, ioRetryTimeout time.Duration) *Controller {
	return &Controller{
		podID:          podID,
		ioRetryTimeout: ioRetryTimeout,
		containers:     make(map[string]*container.Controller),
	}
}

// PodID returns the pod's containerd-facing identifier.
func (c *Controller) PodID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.podID
}

// GetContainer returns the container controller for the given containerID.
func (c *Controller) GetContainer(containerID string) (*container.Controller, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	containerCtrl, ok := c.containers[containerID]
	if !ok {
		return nil, fmt.Errorf("container %q not found in pod %q", containerID, c.podID)
	}

	return containerCtrl, nil
}

// NewContainer creates a new [container.Controller] and registers it in this pod.
func (c *Controller) NewContainer(ctx context.Context, containerID string) (*container.Controller, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.containers[containerID]; ok {
		return nil, fmt.Errorf("container %q already exists in pod %q", containerID, c.podID)
	}

	containerCtrl := container.New(containerID, c.ioRetryTimeout)
	c.containers[containerID] = containerCtrl

	log.G(ctx).Debugf("created new container controller for container %q in pod %q", containerID, c.podID)
	return containerCtrl, nil
}

// ListContainers returns a snapshot of all live container controllers in this pod.
func (c *Controller) ListContainers() map[string]*container.Controller {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*container.Controller, len(c.containers))
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

	log.G(ctx).Debugf("deleted container controller for container %q in pod %q", containerID, c.podID)
	return nil
}
