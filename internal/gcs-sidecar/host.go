//go:build windows
// +build windows

package bridge

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

type Host struct {
	securityOptions *securitypolicy.SecurityOptions
	containersMutex sync.Mutex
	containers      map[string]*Container

	// mapping of volumeGUID to container layer hashes
	blockCIMVolumeHashes map[guid.GUID][]string
	// mapping of volumeGUID to container IDs
	blockCIMVolumeContainers map[guid.GUID]map[string]struct{}
	// mapping of containerID to the ContainerRootPath recorded when
	// CWCOWCombinedLayers mounted it, used to validate the createContainer
	// Storage.Path.
	containerRootPaths map[string]string
	// mountedRoots holds the combined-layers container roots that are currently
	// mounted (set on CWCOWCombinedLayers Add, cleared on Remove), keyed by the
	// lower-cased root path. Used to refuse deleting a container whose root is
	// still mounted.
	mountedRoots map[string]struct{}
}

type Container struct {
	id              string
	spec            oci.Spec
	processesMutex  sync.Mutex
	processes       map[uint32]*containerProcess
	commandLine     bool
	commandLineExec bool
	// allowStdio is the create-time stdio-access policy decision.
	allowStdio bool
	// terminated is set once the container's init process has exited (via the
	// guest container-exit notification).
	terminated atomic.Bool
}

// Process is a struct that defines the lifetime and operations associated with
// an oci.Process.
type containerProcess struct {
	processspec hcsschema.ProcessParameters
	// cid is the container id that owns this process.
	cid string
	pid uint32
}

func NewHost(initialEnforcer securitypolicy.SecurityPolicyEnforcer, logWriter io.Writer) *Host {
	securityPolicyOptions := securitypolicy.NewSecurityOptions(
		initialEnforcer,
		false,
		"",
		"",
		logWriter,
	)
	return &Host{
		containers:               make(map[string]*Container),
		blockCIMVolumeHashes:     make(map[guid.GUID][]string),
		blockCIMVolumeContainers: make(map[guid.GUID]map[string]struct{}),
		containerRootPaths:       make(map[string]string),
		mountedRoots:             make(map[string]struct{}),
		securityOptions:          securityPolicyOptions,
	}
}

func (h *Host) AddContainer(ctx context.Context, id string, c *Container) error {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if _, ok := h.containers[id]; ok {
		log.G(ctx).Tracef("Container exists in the map: %v", ok)
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}
	log.G(ctx).Tracef("AddContainer: ID: %v", id)
	h.containers[id] = c
	return nil
}

func (h *Host) RemoveContainer(ctx context.Context, id string) error {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	_, ok := h.containers[id]
	if !ok {
		log.G(ctx).Tracef("RemoveContainer: Container not found: ID: %v", id)
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}

	if rootPath, ok := h.containerRootPaths[id]; ok {
		delete(h.mountedRoots, strings.ToLower(rootPath))
	}
	delete(h.containers, id)
	delete(h.containerRootPaths, id)
	return nil
}

func (h *Host) GetCreatedContainer(ctx context.Context, id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, ok := h.containers[id]
	if !ok {
		log.G(ctx).Tracef("GetCreatedContainer: Container not found: ID: %v", id)
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}
	return c, nil
}

// IsContainerRootInUse reports whether a container that has not exited is still
// using the combined-layers root mounted at rootPath as its rootfs, so the
// sidecar can refuse to unmount it. (cf. LCOW Host.IsOverlayInUse in
// internal/guest/runtime/hcsv2/uvm.go; WCOW uses a filesystem filter / combined
// layers rather than an overlayfs.)
func (h *Host) IsContainerRootInUse(rootPath string) bool {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	for id, c := range h.containers {
		if c.terminated.Load() {
			continue
		}
		if strings.EqualFold(h.containerRootPaths[id], rootPath) {
			return true
		}
	}
	return false
}

// SetContainerRootMounted records (mounted=true) or clears (mounted=false)
// whether the combined-layers root at rootPath is currently mounted.
func (h *Host) SetContainerRootMounted(rootPath string, mounted bool) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	key := strings.ToLower(rootPath)
	if mounted {
		h.mountedRoots[key] = struct{}{}
	} else {
		delete(h.mountedRoots, key)
	}
}

// IsContainerRootMountedForContainer reports whether the combined-layers root
// recorded for the given container is still mounted.
// (cf. LCOW hostMounts.HasOverlayMountedAt)
func (h *Host) IsContainerRootMountedForContainer(cid string) bool {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	rootPath, ok := h.containerRootPaths[cid]
	if !ok {
		return false
	}
	_, mounted := h.mountedRoots[strings.ToLower(rootPath)]
	return mounted
}

// GetProcess returns the Process with the matching 'pid'. If the 'pid' does
// not exit returns error.
func (c *Container) GetProcess(pid uint32) (*containerProcess, error) {
	//todo: thread a context to this function call
	logrus.WithFields(logrus.Fields{
		logfields.ContainerID: c.id,
		logfields.ProcessID:   pid,
	}).Info("opengcs::Container::GetProcess")

	c.processesMutex.Lock()
	defer c.processesMutex.Unlock()

	p, ok := c.processes[pid]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	return p, nil
}
