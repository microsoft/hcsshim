//go:build windows && wcowprocess

package processisolated

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/signals"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/Microsoft/hcsshim/pkg/annotations"

	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/runtime/task/v3"
	containerdtypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// owner is recorded as the HCS compute-system owner.
const owner = "containerd-shim-wcowprocess-v2"

// Controller manages the lifecycle of a single process-isolated WCOW container.
type Controller struct {
	mu sync.RWMutex

	containerID    string
	ioRetryTimeout time.Duration

	// system is the host HCS compute system backing this container.
	system cow.Container

	// layerCloser releases the mounted container layers on teardown.
	layerCloser resources.ResourceCloser

	state State

	// processes maps exec IDs to their process controllers (init = "").
	processes map[string]*process.Controller

	// terminatedCh is closed once when the container has fully terminated.
	terminatedCh chan struct{}

	// localUserAccount is the ephemeral local user created for a HostProcess
	// container, deleted during teardown.
	localUserAccount string
}

// New creates a ready-to-use Controller.
func New(containerID string, ioRetryTimeout time.Duration) *Controller {
	return &Controller{
		containerID:    containerID,
		ioRetryTimeout: ioRetryTimeout,
		processes:      make(map[string]*process.Controller),
		state:          StateNotCreated,
		terminatedCh:   make(chan struct{}),
	}
}

// Create mounts layers, builds the HCS document, creates the host compute
// system, and sets up the init process.
func (c *Controller) Create(ctx context.Context, spec *specs.Spec, req *task.CreateTaskRequest, _ *CreateOpts) (err error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.ContainerID, c.containerID))

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateNotCreated {
		return fmt.Errorf("container %s is in state %s; cannot create: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	shimOpts, err := vmutils.UnmarshalRuntimeOptions(ctx, req.Options)
	if err != nil {
		return fmt.Errorf("unmarshal runtime options: %w", err)
	}
	*spec = oci.UpdateSpecFromOptions(*spec, shimOpts)
	if err = oci.ProcessAnnotations(ctx, spec.Annotations); err != nil {
		return fmt.Errorf("process OCI spec annotations: %w", err)
	}
	if shimOpts != nil {
		c.ioRetryTimeout = time.Duration(shimOpts.IoRetryTimeoutInSec) * time.Second
	}

	defer func() {
		if err != nil {
			c.state = StateInvalid
			c.teardown(ctx)
		}
	}()

	if spec.Windows == nil {
		return fmt.Errorf("windows section must be present for wcow container")
	}

	// Mount the container layers on the host (no utility VM).
	wcowLayers, err := layers.ParseWCOWLayers(req.Rootfs, spec.Windows.LayerFolders)
	if err != nil {
		return fmt.Errorf("parse wcow layers: %w", err)
	}
	mounted, closer, err := layers.MountWCOWLayers(ctx, c.containerID, nil, wcowLayers)
	if err != nil {
		return fmt.Errorf("mount wcow layers: %w", err)
	}
	c.layerCloser = closer

	if spec.Root == nil {
		spec.Root = &specs.Root{}
	}
	if spec.Root.Path == "" {
		spec.Root.Path = mounted.RootFS
	}

	// For HostProcess containers, a group username maps to an ephemeral local user.
	if oci.IsJobContainer(spec) && spec.Process != nil && spec.Process.User.Username != "" {
		user, uerr := c.setupHostProcessUser(ctx, spec.Process.User.Username)
		if uerr != nil {
			return fmt.Errorf("setup host process user: %w", uerr)
		}
		if user != "" {
			spec.Process.User.Username = user
			c.localUserAccount = user
		}
	}

	doc, err := c.buildDoc(spec, mounted)
	if err != nil {
		return fmt.Errorf("build compute system document: %w", err)
	}
	c.system, err = hcs.CreateComputeSystem(ctx, c.containerID, doc)
	if err != nil {
		return fmt.Errorf("create compute system: %w", err)
	}

	// The default pause image entrypoint is a bare cmd.exe, which exits
	// immediately on Windows; rewrite it so the sandbox container stays alive.
	if ct, _, cerr := oci.GetSandboxTypeAndID(spec.Annotations); cerr == nil && ct == oci.KubernetesContainerTypeSandbox && spec.Process != nil {
		const defaultCmd = `c:\windows\system32\cmd.exe`
		if (len(spec.Process.Args) == 1 && strings.EqualFold(spec.Process.Args[0], defaultCmd)) ||
			strings.EqualFold(spec.Process.CommandLine, defaultCmd) {
			spec.Process.CommandLine = "cmd /c ping -t 127.0.0.1 > nul"
		}
	}

	initProcess := process.New(c.containerID, "", c.system, c.ioRetryTimeout)
	if err = initProcess.Create(ctx, &process.CreateOptions{
		Bundle: req.Bundle,
		// A WCOW host container is not an OCI ProcessHost, so the init process
		// spec must be supplied (unlike LCOW where the guest creates init).
		Spec:     spec.Process,
		Terminal: req.Terminal,
		Stdin:    req.Stdin,
		Stdout:   req.Stdout,
		Stderr:   req.Stderr,
	}); err != nil {
		return fmt.Errorf("create init process: %w", err)
	}
	c.processes[""] = initProcess

	c.state = StateCreated
	return nil
}

// buildDoc constructs the HCS compute-system document for the container.
func (c *Controller) buildDoc(spec *specs.Spec, mounted *layers.MountedWCOWLayers) (*hcsschema.ComputeSystem, error) {
	storage := &hcsschema.Storage{Path: mounted.RootFS}
	for _, l := range mounted.MountedLayerPaths {
		storage.Layers = append(storage.Layers, hcsschema.Layer{Id: l.LayerID, Path: l.MountedPath})
	}

	container := &hcsschema.Container{Storage: storage}

	// The network namespace is CRI-assigned per container (all containers in a pod
	// share it); use the container's own spec value.
	if spec.Windows.Network != nil && spec.Windows.Network.NetworkNamespace != "" {
		container.Networking = &hcsschema.Networking{Namespace: spec.Windows.Network.NetworkNamespace}
	}

	if res := spec.Windows.Resources; res != nil {
		if res.CPU != nil {
			p := &hcsschema.Processor{}
			if res.CPU.Count != nil {
				p.Count = int32(*res.CPU.Count)
			}
			if res.CPU.Maximum != nil {
				p.Maximum = int32(*res.CPU.Maximum)
			}
			if res.CPU.Shares != nil {
				p.Weight = int32(*res.CPU.Shares)
			}
			container.Processor = p
		}
		if res.Memory != nil && res.Memory.Limit != nil {
			container.Memory = &hcsschema.Memory{SizeInMB: *res.Memory.Limit / memory.MiB}
		}
	}

	// Bind mounts become mapped directories.
	for _, m := range spec.Mounts {
		readOnly := false
		for _, o := range m.Options {
			if strings.EqualFold(o, "ro") {
				readOnly = true
			}
		}
		container.MappedDirectories = append(container.MappedDirectories, hcsschema.MappedDirectory{
			HostPath:      m.Source,
			ContainerPath: m.Destination,
			ReadOnly:      readOnly,
		})
	}

	if oci.IsJobContainer(spec) {
		container.IsolationType = hcsschema.IsolationTypeHostProcess
		if p, ok := spec.Annotations[annotations.HostProcessRootfsLocation]; ok {
			storage.PrivilegedContainerRootPath = p
		}
	}

	return &hcsschema.ComputeSystem{
		Owner:                             owner,
		SchemaVersion:                     schemaversion.SchemaV21(),
		ShouldTerminateOnLastHandleClosed: true,
		Container:                         container,
	}, nil
}

// Start starts the container and its init process, returning the init PID.
func (c *Controller) Start(ctx context.Context, events chan interface{}) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateCreated {
		return 1, fmt.Errorf("container %s is in state %s; cannot start: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	if err := c.system.Start(ctx); err != nil {
		c.state = StateInvalid
		return 1, fmt.Errorf("start container %s: %w", c.containerID, err)
	}

	initProcess := c.processes[""]
	pid, err := initProcess.Start(ctx, nil)
	if err != nil {
		c.state = StateInvalid
		return 1, fmt.Errorf("start init process: %w", err)
	}

	c.state = StateRunning
	go c.handleInitProcessExit(ctx, initProcess, events)

	return uint32(pid), nil
}

// handleInitProcessExit blocks until the init process exits, then marks the
// container stopped and publishes the exit event.
func (c *Controller) handleInitProcessExit(ctx context.Context, initProcess *process.Controller, events chan interface{}) {
	ctx = context.WithoutCancel(ctx)
	initProcess.Wait(ctx)

	c.mu.Lock()
	c.state = StateStopped
	c.signalTerminated()
	c.mu.Unlock()

	if events != nil {
		status := initProcess.Status(true)
		events <- &eventstypes.TaskExit{
			ContainerID: c.containerID,
			ID:          status.ExecID,
			Pid:         status.Pid,
			ExitStatus:  status.ExitStatus,
			ExitedAt:    status.ExitedAt,
		}
	}
}

// teardown releases all host-side resources. It is idempotent. The caller must hold c.mu.
func (c *Controller) teardown(ctx context.Context) {
	if c.system != nil {
		if err := c.system.Shutdown(ctx); err != nil {
			_ = c.system.Terminate(ctx)
		} else {
			ch := make(chan error, 1)
			go func() { ch <- c.system.Wait() }()
			select {
			case <-ch:
			case <-time.After(30 * time.Second):
				_ = c.system.Terminate(ctx)
			}
		}
	}

	if c.layerCloser != nil {
		if err := c.layerCloser.Release(ctx); err != nil {
			log.G(ctx).WithError(err).Warn("failed to release container layers")
		}
		c.layerCloser = nil
	}

	if c.localUserAccount != "" {
		if err := winapi.NetUserDel("", c.localUserAccount); err != nil {
			log.G(ctx).WithError(err).Warn("failed to delete local user account")
		}
		c.localUserAccount = ""
	}

	if c.system != nil {
		_ = c.system.Close()
		c.system = nil
	}

	c.signalTerminated()
}

// signalTerminated closes terminatedCh exactly once. The caller must hold c.mu.
func (c *Controller) signalTerminated() {
	select {
	case <-c.terminatedCh:
	default:
		close(c.terminatedCh)
	}
}

// Wait blocks until the container has fully terminated.
func (c *Controller) Wait(ctx context.Context) {
	select {
	case <-c.terminatedCh:
	case <-ctx.Done():
		log.G(ctx).WithError(ctx.Err()).Error("wait for container to exit failed")
	}
}

// Update modifies the container's resource constraints.
func (c *Controller) Update(ctx context.Context, resources interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateRunning {
		return fmt.Errorf("container %s is in state %s; cannot update: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	res, ok := resources.(*specs.WindowsResources)
	if !ok {
		return fmt.Errorf("invalid container resources: expected *specs.WindowsResources, got %T", resources)
	}

	if res.Memory != nil && res.Memory.Limit != nil {
		if err := c.modify(ctx, resourcepaths.SiloMemoryResourcePath, *res.Memory.Limit/memory.MiB); err != nil {
			return fmt.Errorf("update memory: %w", err)
		}
	}
	if res.CPU != nil {
		p := &hcsschema.Processor{}
		if res.CPU.Count != nil {
			p.Count = int32(*res.CPU.Count)
		}
		if res.CPU.Maximum != nil {
			p.Maximum = int32(*res.CPU.Maximum)
		}
		if res.CPU.Shares != nil {
			p.Weight = int32(*res.CPU.Shares)
		}
		if err := c.modify(ctx, resourcepaths.SiloProcessorResourcePath, p); err != nil {
			return fmt.Errorf("update cpu: %w", err)
		}
	}
	return nil
}

// modify issues an HCS update request for the given resource path.
func (c *Controller) modify(ctx context.Context, resourcePath string, settings interface{}) error {
	return c.system.Modify(ctx, &hcsschema.ModifySettingRequest{
		ResourcePath: resourcePath,
		RequestType:  guestrequest.RequestTypeUpdate,
		Settings:     settings,
	})
}

// NewProcess creates a new exec process controller in the container.
func (c *Controller) NewProcess(execID string) (*process.Controller, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateRunning {
		return nil, fmt.Errorf("container %s is in state %s; cannot create new process: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}
	if _, exists := c.processes[execID]; exists {
		return nil, fmt.Errorf("exec process %q already exists in container %s", execID, c.containerID)
	}

	newProcess := process.New(c.containerID, execID, c.system, c.ioRetryTimeout)
	c.processes[execID] = newProcess
	return newProcess, nil
}

// GetProcess returns the process controller for the given exec ID.
func (c *Controller) GetProcess(execID string) (*process.Controller, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.getProcess(execID)
}

// getProcess returns the process controller for the given exec ID. The caller must hold c.mu.
func (c *Controller) getProcess(execID string) (*process.Controller, error) {
	proc, ok := c.processes[execID]
	if !ok {
		return nil, fmt.Errorf("process %q not found in container %s: %w", execID, c.containerID, errdefs.ErrNotFound)
	}
	return proc, nil
}

// ListProcesses returns all exec processes (excluding the init process).
func (c *Controller) ListProcesses() (map[string]*process.Controller, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*process.Controller, len(c.processes))
	for id, proc := range c.processes {
		if id == "" {
			continue
		}
		result[id] = proc
	}
	return result, nil
}

// Pids queries the host for the container process list annotated with exec IDs.
func (c *Controller) Pids(ctx context.Context) ([]*containerdtypes.ProcessInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state != StateRunning {
		return nil, fmt.Errorf("container %s is in state %s; cannot query pids: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	pidMap := make(map[int]string, len(c.processes))
	for execID, proc := range c.processes {
		pidMap[proc.Pid()] = execID
	}

	props, err := c.system.Properties(ctx, schema1.PropertyTypeProcessList)
	if err != nil {
		return nil, fmt.Errorf("fetch container properties: %w", err)
	}

	processes := make([]*containerdtypes.ProcessInfo, len(props.ProcessList))
	for i, proc := range props.ProcessList {
		pd := &runhcsopts.ProcessDetails{
			ImageName:                    proc.ImageName,
			CreatedAt:                    timestamppb.New(proc.CreateTimestamp),
			KernelTime_100Ns:             proc.KernelTime100ns,
			MemoryCommitBytes:            proc.MemoryCommitBytes,
			MemoryWorkingSetPrivateBytes: proc.MemoryWorkingSetPrivateBytes,
			MemoryWorkingSetSharedBytes:  proc.MemoryWorkingSetSharedBytes,
			ProcessID:                    proc.ProcessId,
			UserTime_100Ns:               proc.UserTime100ns,
		}
		if execID, ok := pidMap[int(proc.ProcessId)]; ok {
			pd.ExecID = execID
		}
		anyVal, err := typeurl.MarshalAny(pd)
		if err != nil {
			return nil, fmt.Errorf("marshal process details for exec %s in container %s: %w", pd.ExecID, c.containerID, err)
		}
		processes[i] = &containerdtypes.ProcessInfo{
			Pid:  pd.ProcessID,
			Info: typeurl.MarshalProto(anyVal),
		}
	}
	return processes, nil
}

// Stats returns the runtime statistics for the container.
func (c *Controller) Stats(ctx context.Context) (*stats.Statistics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state != StateRunning {
		return nil, fmt.Errorf("container %s is in state %s; cannot fetch stats: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	props, err := c.system.PropertiesV2(ctx, hcsschema.PTStatistics)
	if err != nil {
		return nil, fmt.Errorf("fetch container statistics: %w", err)
	}

	containerStats := &stats.Statistics{}
	if props != nil {
		containerStats.Container = hcsPropertiesToWindowsStats(props)
	}
	return containerStats, nil
}

// hcsPropertiesToWindowsStats converts HCS properties into Windows container statistics.
func hcsPropertiesToWindowsStats(props *hcsschema.Properties) *stats.Statistics_Windows {
	wcs := &stats.Statistics_Windows{Windows: &stats.WindowsContainerStatistics{}}
	if props.Statistics == nil {
		return wcs
	}
	wcs.Windows.Timestamp = timestamppb.New(props.Statistics.Timestamp)
	wcs.Windows.ContainerStartTime = timestamppb.New(props.Statistics.ContainerStartTime)
	wcs.Windows.UptimeNS = props.Statistics.Uptime100ns * 100
	if props.Statistics.Processor != nil {
		wcs.Windows.Processor = &stats.WindowsContainerProcessorStatistics{
			TotalRuntimeNS:  props.Statistics.Processor.TotalRuntime100ns * 100,
			RuntimeUserNS:   props.Statistics.Processor.RuntimeUser100ns * 100,
			RuntimeKernelNS: props.Statistics.Processor.RuntimeKernel100ns * 100,
		}
	}
	if props.Statistics.Memory != nil {
		wcs.Windows.Memory = &stats.WindowsContainerMemoryStatistics{
			MemoryUsageCommitBytes:            props.Statistics.Memory.MemoryUsageCommitBytes,
			MemoryUsageCommitPeakBytes:        props.Statistics.Memory.MemoryUsageCommitPeakBytes,
			MemoryUsagePrivateWorkingSetBytes: props.Statistics.Memory.MemoryUsagePrivateWorkingSetBytes,
		}
	}
	if props.Statistics.Storage != nil {
		wcs.Windows.Storage = &stats.WindowsContainerStorageStatistics{
			ReadCountNormalized:  props.Statistics.Storage.ReadCountNormalized,
			ReadSizeBytes:        props.Statistics.Storage.ReadSizeBytes,
			WriteCountNormalized: props.Statistics.Storage.WriteCountNormalized,
			WriteSizeBytes:       props.Statistics.Storage.WriteSizeBytes,
		}
	}
	return wcs
}

// KillProcess delivers a signal to the specified process or all processes in the container.
func (c *Controller) KillProcess(ctx context.Context, execID string, signal uint32, all bool) error {
	if all && execID != "" {
		return fmt.Errorf("cannot signal all for non-empty exec %q: %w", execID, errdefs.ErrFailedPrecondition)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == StateNotCreated {
		return fmt.Errorf("container %s is in state %s; cannot kill: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}
	if c.state == StateStopped || c.state == StateInvalid {
		return nil
	}

	signalOptions, err := signals.ValidateWCOW(int(signal), true)
	if err != nil {
		return fmt.Errorf("validate signal %d for container %s: %w", signal, c.containerID, err)
	}
	// A nil result means terminate rather than signal.
	var opts interface{}
	if signalOptions != nil {
		opts = signalOptions
	}

	if all || execID == "" {
		for eid, proc := range c.processes {
			if eid == "" {
				continue
			}
			if killErr := proc.Kill(ctx, opts); killErr != nil {
				log.G(ctx).WithError(killErr).WithField(logfields.ExecID, eid).Warn("failed to kill exec in container")
			}
		}
	}

	targetProcess, err := c.getProcess(execID)
	if err != nil {
		return err
	}
	return targetProcess.Kill(ctx, opts)
}

// DeleteProcess removes the process identified by execID and returns its last status.
func (c *Controller) DeleteProcess(ctx context.Context, execID string) (*task.StateResponse, error) {
	if execID == "" {
		c.mu.RLock()
		isStarted := c.state == StateRunning || c.state == StateStopped
		c.mu.RUnlock()

		if isStarted {
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			c.Wait(waitCtx)
			if waitCtx.Err() != nil {
				return nil, fmt.Errorf("wait for container %s resource cleanup: %w", c.containerID, waitCtx.Err())
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == StateNotCreated {
		return nil, fmt.Errorf("container %s is in state %s; cannot delete process: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	proc, err := c.getProcess(execID)
	if err != nil {
		return nil, err
	}
	if err = proc.Delete(ctx); err != nil {
		return nil, err
	}
	status := proc.Status(true)

	// Deleting the init process tears down the container.
	if execID == "" {
		c.teardown(ctx)
	}

	delete(c.processes, execID)
	return status, nil
}
