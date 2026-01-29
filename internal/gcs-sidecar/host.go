//go:build windows
// +build windows

package bridge

import (
	"context"
	"io"
	"sync"

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
}

type Container struct {
	id              string
	spec            oci.Spec
	processesMutex  sync.Mutex
	processes       map[uint32]*containerProcess
	commandLine     bool
	commandLineExec bool
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
		logWriter,
	)
	return &Host{
		containers:               make(map[string]*Container),
		blockCIMVolumeHashes:     make(map[guid.GUID][]string),
		blockCIMVolumeContainers: make(map[guid.GUID]map[string]struct{}),
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

func (h *Host) RemoveContainer(ctx context.Context, id string) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	_, ok := h.containers[id]
	if !ok {
		log.G(ctx).Tracef("RemoveContainer: Container not found: ID: %v", id)
		return
	}

	delete(h.containers, id)
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
