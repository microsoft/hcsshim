// Package gcs defines the core functionality of the GCS. This includes all
// the code which manages container and their state, including interfacing with
// the container runtime, forwarding container stdio through
// transport.Connections, and configuring networking for a container.
package gcs

import (
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	shellwords "github.com/mattn/go-shellwords"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"

	"github.com/Microsoft/opengcs/service/gcs/core"
	gcserr "github.com/Microsoft/opengcs/service/gcs/errors"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/runtime/runc"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
)

const (
	terminateProcessTimeout = time.Second * 10
)

// gcsCore is an implementation of the Core interface, defining the
// functionality of the GCS.
type gcsCore struct {
	// Rtime is the Runtime interface used by the GCS core.
	Rtime runtime.Runtime

	// OS is the OS interface used by the GCS core.
	OS oslayer.OS

	containerCacheMutex sync.RWMutex
	// containerCache stores information about containers which persists
	// between calls into the gcsCore. It is structured as a map from container
	// ID to cache entry.
	containerCache map[string]*containerCacheEntry

	processCacheMutex sync.RWMutex
	// processCache stores information about processes which persists between
	// calls into the gcsCore. It is structured as a map from pid to cache
	// entry.
	processCache map[int]*processCacheEntry

	externalProcessCacheMutex sync.RWMutex
	// externalProcessCache stores information about external processes which
	// persists between calls into the gcsCore. It is structured as a map from
	// pid to cache entry.
	externalProcessCache map[int]*processCacheEntry
}

// NewGCSCore creates a new gcsCore struct initialized with the given Runtime.
func NewGCSCore(rtime runtime.Runtime, os oslayer.OS) *gcsCore {
	return &gcsCore{
		Rtime:                rtime,
		OS:                   os,
		containerCache:       make(map[string]*containerCacheEntry),
		processCache:         make(map[int]*processCacheEntry),
		externalProcessCache: make(map[int]*processCacheEntry),
	}
}

// containerCacheEntry stores cached information for a single container.
type containerCacheEntry struct {
	ID                 string
	ExitStatus         oslayer.ProcessExitState
	Processes          []int
	ExitHooks          []func(oslayer.ProcessExitState)
	MappedVirtualDisks map[uint8]prot.MappedVirtualDisk
	NetworkAdapters    []prot.NetworkAdapter
	container          runtime.Container
}

func newContainerCacheEntry(id string) *containerCacheEntry {
	return &containerCacheEntry{
		ID:                 id,
		MappedVirtualDisks: make(map[uint8]prot.MappedVirtualDisk),
	}
}
func (e *containerCacheEntry) AddExitHook(hook func(oslayer.ProcessExitState)) {
	e.ExitHooks = append(e.ExitHooks, hook)
}
func (e *containerCacheEntry) AddProcess(pid int) {
	e.Processes = append(e.Processes, pid)
}
func (e *containerCacheEntry) AddNetworkAdapter(adapter prot.NetworkAdapter) {
	e.NetworkAdapters = append(e.NetworkAdapters, adapter)
}
func (e *containerCacheEntry) AddMappedVirtualDisk(disk prot.MappedVirtualDisk) error {
	if _, ok := e.MappedVirtualDisks[disk.Lun]; ok {
		return errors.Errorf("a mapped virtual disk with lun %d is already attached to container %s", disk.Lun, e.ID)
	}
	e.MappedVirtualDisks[disk.Lun] = disk
	return nil
}
func (e *containerCacheEntry) RemoveMappedVirtualDisk(disk prot.MappedVirtualDisk) error {
	if _, ok := e.MappedVirtualDisks[disk.Lun]; !ok {
		return errors.Errorf("a mapped virtual disk with lun %d is not attached to container %s", disk.Lun, e.ID)
	}
	delete(e.MappedVirtualDisks, disk.Lun)
	return nil
}

// processCacheEntry stores cached information for a single process.
type processCacheEntry struct {
	ExitStatus oslayer.ProcessExitState
	ExitHooks  []func(oslayer.ProcessExitState)
}

func newProcessCacheEntry() *processCacheEntry {
	return &processCacheEntry{}
}
func (e *processCacheEntry) AddExitHook(hook func(oslayer.ProcessExitState)) {
	e.ExitHooks = append(e.ExitHooks, hook)
}

func (c *gcsCore) getContainer(id string) *containerCacheEntry {
	if entry, ok := c.containerCache[id]; ok {
		return entry
	}
	return nil
}

// CreateContainer creates all the infrastructure for a container, including
// setting up layers and networking, and then starts up its init process in a
// suspended state waiting for a call to StartContainer.
func (c *gcsCore) CreateContainer(id string, settings prot.VMHostedContainerSettings) error {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	if c.getContainer(id) != nil {
		return errors.WithStack(gcserr.NewContainerExistsError(id))
	}

	containerEntry := newContainerCacheEntry(id)

	// Set up mapped virtual disks.
	if err := c.setupMappedVirtualDisks(id, settings.MappedVirtualDisks, containerEntry); err != nil {
		return errors.Wrapf(err, "failed to set up mapped virtual disks during create for container %s", id)
	}

	// Set up layers.
	scratchDevice, layers, err := c.getLayerDevices(settings.SandboxDataPath, settings.Layers)
	if err != nil {
		return errors.Wrapf(err, "failed to get layer devices for container %s", id)
	}
	if err := c.mountLayers(id, scratchDevice, layers); err != nil {
		return errors.Wrapf(err, "failed to mount layers for container %s", id)
	}

	// Set up networking.
	for _, adapter := range settings.NetworkAdapters {
		if err := c.configureNetworkAdapter(adapter); err != nil {
			return errors.Wrapf(err, "failed to configure network adapter %s", adapter.AdapterInstanceID)
		}
		containerEntry.AddNetworkAdapter(adapter)
	}

	c.containerCache[id] = containerEntry

	return nil
}

// ExecProcess executes a new process in the container. It forwards the
// process's stdio through the members of the core.StdioSet provided.
func (c *gcsCore) ExecProcess(id string, params prot.ProcessParameters, stdioSet *core.StdioSet) (int, error) {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return -1, errors.WithStack(gcserr.NewContainerDoesNotExistError(id))
	}
	processEntry := newProcessCacheEntry()

	var p runtime.Process

	stdioOptions := runtime.StdioOptions{
		CreateIn:  params.CreateStdInPipe,
		CreateOut: params.CreateStdOutPipe,
		CreateErr: params.CreateStdErrPipe,
	}
	isInitProcess := len(containerEntry.Processes) == 0
	if isInitProcess {
		if err := c.writeConfigFile(id, params.OCISpecification); err != nil {
			return -1, err
		}

		container, err := c.Rtime.CreateContainer(id, c.getContainerStoragePath(id), stdioOptions)
		if err != nil {
			return -1, err
		}

		containerEntry.container = container
		p = container

		// Move the container's network adapters into its namespace.
		for _, adapter := range containerEntry.NetworkAdapters {
			if err := c.moveAdapterIntoNamespace(container, adapter); err != nil {
				return -1, err
			}
		}

		go func() {
			state, err := container.Wait()
			c.containerCacheMutex.Lock()
			if err != nil {
				logrus.Error(err)
				if err := c.cleanupContainer(containerEntry); err != nil {
					logrus.Error(err)
				}
			}
			utils.LogMsgf("container init process %d exited with exit status %d", p.Pid(), state.ExitCode())

			if err := c.cleanupContainer(containerEntry); err != nil {
				logrus.Error(err)
			}
			c.containerCacheMutex.Unlock()

			c.processCacheMutex.Lock()
			processEntry.ExitStatus = state
			for _, hook := range processEntry.ExitHooks {
				hook(state)
			}
			c.processCacheMutex.Unlock()
			c.containerCacheMutex.Lock()
			containerEntry.ExitStatus = state
			for _, hook := range containerEntry.ExitHooks {
				hook(state)
			}
			delete(c.containerCache, id)
			c.containerCacheMutex.Unlock()
		}()

		if err := container.Start(); err != nil {
			return -1, err
		}
	} else {
		ociProcess, err := processParametersToOCI(params)
		if err != nil {
			return -1, err
		}
		p, err = containerEntry.container.ExecProcess(ociProcess, stdioOptions)
		if err != nil {
			return -1, err
		}

		go func() {
			state, err := p.Wait()
			if err != nil {
				logrus.Error(err)
			}
			utils.LogMsgf("container process %d exited with exit status %d", p.Pid(), state.ExitCode())

			// Close stdin.
			// TODO: Remove this conditional when stdio forwarding for non-terminal processes is fixed.
			if ociProcess.Terminal {
				if err := stdioSet.In.CloseRead(); err != nil {
					logrus.Errorf("failed call to CloseRead for non-initial process stdin: %v: %s", ociProcess.Args, err)
				}
				if err := stdioSet.In.Close(); err != nil {
					logrus.Errorf("failed call to Close for non-initial process stdin: %v: %s", ociProcess.Args, err)
				}
			}

			c.processCacheMutex.Lock()
			processEntry.ExitStatus = state
			for _, hook := range processEntry.ExitHooks {
				hook(state)
			}
			c.processCacheMutex.Unlock()
			if err := p.Delete(); err != nil {
				logrus.Error(err)
			}
		}()
	}

	// Connect the container's stdio to the stdio pipes.
	if err := c.setupStdioPipes(p, stdioSet); err != nil {
		return -1, err
	}

	c.processCacheMutex.Lock()
	// If a processCacheEntry with the given pid already exists in the cache,
	// this will overwrite it. This behavior is expected. Processes are kept in
	// the cache even after they exit, which allows for exit hooks registered
	// on exited processed to still run. For example, if the HCS were to wait
	// on a process which had already exited (due to a race condition between
	// the wait call and the process exiting), the process's exit state would
	// still be available to send back to the HCS. However, when pids are
	// reused on the system, it makes sense to overwrite the old cache entry.
	// This is because registering an exit hook on the pid and expecting it to
	// apply to the old process no longer makes sense, so since the old
	// process's pid has been reused, its cache entry can also be reused.  This
	// applies to external processes as well.
	c.processCache[p.Pid()] = processEntry
	c.processCacheMutex.Unlock()
	containerEntry.AddProcess(p.Pid())
	return p.Pid(), nil
}

// SignalContainer sends the specified signal to the container's init process.
func (c *gcsCore) SignalContainer(id string, signal oslayer.Signal) error {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return errors.WithStack(gcserr.NewContainerDoesNotExistError(id))
	}

	if containerEntry.container != nil {
		if err := containerEntry.container.Kill(signal); err != nil {
			return err
		}
	}

	return nil
}

// TerminateProcess sends a SIGTERM signal to the given process. If it does not
// exit after a timeout, it then sends a SIGKILL.
func (c *gcsCore) TerminateProcess(pid int) error {
	c.processCacheMutex.Lock()
	c.externalProcessCacheMutex.Lock()
	if _, ok := c.processCache[pid]; !ok {
		if _, ok := c.externalProcessCache[pid]; !ok {
			c.processCacheMutex.Unlock()
			c.externalProcessCacheMutex.Unlock()
			return errors.WithStack(gcserr.NewProcessDoesNotExistError(pid))
		}
	}
	c.processCacheMutex.Unlock()
	c.externalProcessCacheMutex.Unlock()

	// First, send the process a SIGTERM. If it doesn't exit before the
	// specified timeout, send it a SIGKILL.
	exitedChannel := make(chan bool, 1)
	exitHook := func(state oslayer.ProcessExitState) {
		exitedChannel <- true
	}
	if err := c.RegisterProcessExitHook(pid, exitHook); err != nil {
		return errors.Wrapf(err, "failed to register exit hook during call to TerminateProcess for process %d", pid)
	}
	if err := c.OS.Kill(pid, syscall.SIGTERM); err != nil {
		return errors.Wrapf(err, "failed call to kill on process %d", pid)
	}
	select {
	case <-exitedChannel: // Do nothing.
	case <-time.After(terminateProcessTimeout):
		// If the timeout is exceeded, kill the process with SIGKILL.
		// TODO: Properly handle the race condition between the process exiting
		// and Kill being called, so that the error doesn't need to be ignored.
		// This can be done by waiting on processes without hanging, locking on
		// the waits, and setting a flag to indicate that the process has
		// exited. Then, this code can lock on the same lock, and check if the
		// process has exited or not before calling Kill.
		if err := c.OS.Kill(pid, syscall.SIGKILL); err != nil {
			logrus.Error(err)
		}
	}

	return nil
}

// ListProcesses returns all container processes, even zombies.
func (c *gcsCore) ListProcesses(id string) ([]runtime.ContainerProcessState, error) {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return nil, errors.WithStack(gcserr.NewContainerDoesNotExistError(id))
	}

	if containerEntry.container == nil {
		return nil, nil
	}

	processes, err := containerEntry.container.GetAllProcesses()
	if err != nil {
		return nil, err
	}
	return processes, nil
}

// RunExternalProcess runs a process in the utility VM outside of a container's
// namespace.
// This can be used for things like debugging or diagnosing the utility VM's
// state.
func (c *gcsCore) RunExternalProcess(params prot.ProcessParameters, stdioSet *core.StdioSet) (pid int, err error) {
	stdioOptions := runtime.StdioOptions{
		CreateIn:  params.CreateStdInPipe,
		CreateOut: params.CreateStdOutPipe,
		CreateErr: params.CreateStdErrPipe,
	}
	var master io.ReadWriteCloser
	var console oslayer.File
	emulateConsole := params.EmulateConsole
	if emulateConsole {
		// Allocate a console for the process.
		// TODO: Should I duplicate the NewConsole functionality outside of
		// runc to make for better separation between gcscore and runtime?
		var consolePath string
		master, consolePath, err = runc.NewConsole()
		if err != nil {
			return -1, errors.Wrap(err, "failed to create console for external process")
		}
		console, err = c.OS.OpenFile(consolePath, os.O_RDWR, 0777)
		if err != nil {
			return -1, errors.Wrap(err, "failed to open console file for external process")
		}
	}

	ociProcess, err := processParametersToOCI(params)
	if err != nil {
		return -1, err
	}
	cmd := c.OS.Command(ociProcess.Args[0], ociProcess.Args[1:]...)
	cmd.SetDir(ociProcess.Cwd)
	cmd.SetEnv(ociProcess.Env)

	var wg sync.WaitGroup
	if emulateConsole {
		// Begin copying data to and from the console.
		// In order to ensure master is closed only after it's done being read
		// from and written to, a sync.WaitGroup is used.
		if stdioOptions.CreateIn {
			wg.Add(1)
			go func() {
				io.Copy(master, stdioSet.In)
				wg.Done()
			}()
		}
		if stdioOptions.CreateOut {
			wg.Add(1)
			go func() {
				io.Copy(stdioSet.Out, master)
				wg.Done()
			}()
		}

		cmd.SetStdin(console)
		cmd.SetStdout(console)
		cmd.SetStderr(console)
	} else {
		if stdioOptions.CreateIn {
			// Stdin uses cmd.StdinPipe() instead of cmd.Stdin because cmd.Wait()
			// waits for cmd.Stdin to return EOF. We can only guarantee an EOF by
			// closing the pipe after the process exits.
			cmdStdin, err := cmd.StdinPipe()
			if err != nil {
				return -1, errors.Wrap(err, "failed to get stdin pipe for command")
			}
			wg.Add(1)
			go func() {
				io.Copy(cmdStdin, stdioSet.In)
				cmdStdin.Close() // Notify the process that there is no more input.
				wg.Done()
			}()
		}
		if stdioOptions.CreateOut {
			cmd.SetStdout(stdioSet.Out)
		}
		if stdioOptions.CreateErr {
			cmd.SetStderr(stdioSet.Err)
		}
	}
	if err := cmd.Start(); err != nil {
		return -1, errors.Wrap(err, "failed call to Start for external process")
	}

	processEntry := newProcessCacheEntry()
	go func() {
		if err := cmd.Wait(); err != nil {
			// TODO: When cmd is a shell, and last command in the shell
			// returned an error (e.g. typing a non-existing command gives
			// error 127), Wait also returns an error. We should find a way to
			// distinguish between these errors and ones which are actually
			// important.
			logrus.Error(errors.Wrap(err, "failed call to Wait for external process"))
		}
		utils.LogMsgf("external process %d exited with exit status %d", cmd.Process().Pid(), cmd.ExitState().ExitCode())

		// Close stdin so that the copying goroutine is safely unblocked; this is necessary
		// because the host expects stdin to be closed before it will report process
		// exit back to the client, and the client expects the process notification before
		// it will close its side of stdin (which io.Copy is waiting on in the copying goroutine).
		if stdioSet.In != nil {
			if err := stdioSet.In.CloseRead(); err != nil {
				logrus.Errorf("failed call to CloseRead for external process stdin: %v: %s", ociProcess.Args, err)
			}
		}

		// Close the console slave file to unblock IO to the console master file.
		if console != nil {
			console.Close()
		}

		// Wait for all users of stdioSet and master to finish before closing them.
		wg.Wait()

		if master != nil {
			master.Close()
		}
		if stdioSet.In != nil {
			stdioSet.In.Close()
		}
		if stdioSet.Out != nil {
			stdioSet.Out.Close()
		}
		if stdioSet.Err != nil {
			stdioSet.Err.Close()
		}

		// Run exit hooks for the process.
		state := cmd.ExitState()
		c.externalProcessCacheMutex.Lock()
		processEntry.ExitStatus = state
		for _, hook := range processEntry.ExitHooks {
			hook(state)
		}
		c.externalProcessCacheMutex.Unlock()
	}()

	pid = cmd.Process().Pid()
	c.externalProcessCacheMutex.Lock()
	c.externalProcessCache[pid] = processEntry
	c.externalProcessCacheMutex.Unlock()
	return pid, nil
}

// ModifySettings takes the given request and performs the modification it
// specifies. At the moment, this function only supports the request types Add
// and Remove, both for the resource type MappedVirtualDisk.
func (c *gcsCore) ModifySettings(id string, request prot.ResourceModificationRequestResponse) error {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return errors.WithStack(gcserr.NewContainerDoesNotExistError(id))
	}

	switch request.RequestType {
	case prot.RtAdd:
		if request.ResourceType != prot.PtMappedVirtualDisk {
			return errors.Errorf("only the resource type \"%s\" is currently supported for request type \"%s\"", prot.PtMappedVirtualDisk, request.RequestType)
		}
		settings, ok := request.Settings.(prot.ResourceModificationSettings)
		if !ok {
			return errors.New("the request's settings are not of type ResourceModificationSettings")
		}
		if err := c.setupMappedVirtualDisks(id, []prot.MappedVirtualDisk{*settings.MappedVirtualDisk}, containerEntry); err != nil {
			return errors.Wrapf(err, "failed to hot add mapped virtual disk for container %s", id)
		}
	case prot.RtRemove:
		if request.ResourceType != prot.PtMappedVirtualDisk {
			return errors.Errorf("only the resource type \"%s\" is currently supported for request type \"%s\"", prot.PtMappedVirtualDisk, request.RequestType)
		}
		settings, ok := request.Settings.(prot.ResourceModificationSettings)
		if !ok {
			return errors.New("the request's settings are not of type ResourceModificationSettings")
		}
		if err := c.removeMappedVirtualDisks(id, []prot.MappedVirtualDisk{*settings.MappedVirtualDisk}, containerEntry); err != nil {
			return errors.Wrapf(err, "failed to hot remove mapped virtual disk for container %s", id)
		}
	default:
		return errors.Errorf("the request type \"%s\" is not yet supported", request.RequestType)
	}

	return nil
}

// RegisterContainerExitHook registers an exit hook on the container with the
// given ID. When the container exits, the given exit function will be called.
// If the container has already exited, the function will be called
// immediately.  A container may have multiple exit hooks registered for it.
func (c *gcsCore) RegisterContainerExitHook(id string, exitHook func(oslayer.ProcessExitState)) error {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	entry := c.getContainer(id)
	if entry == nil {
		return errors.WithStack(gcserr.NewContainerDoesNotExistError(id))
	}

	exitStatus := entry.ExitStatus
	// If the container has already exited, run the hook immediately.
	// Otherwise, add it to the container's hook list.
	if exitStatus != nil {
		exitHook(exitStatus)
	} else {
		entry.AddExitHook(exitHook)
	}
	return nil
}

// RegisterProcessExitHook registers an exit hook on the process with the given
// pid. When the process exits, the given exit function will be called. if the
// process has already exited, the function will be called immediately. A
// process may have multiple exit hooks registered for it.
// This function works for both processes that are running in a container, and
// ones that are running externally to a container.
func (c *gcsCore) RegisterProcessExitHook(pid int, exitHook func(oslayer.ProcessExitState)) error {
	c.processCacheMutex.Lock()
	defer c.processCacheMutex.Unlock()
	c.externalProcessCacheMutex.Lock()
	defer c.externalProcessCacheMutex.Unlock()

	var entry *processCacheEntry
	var ok bool
	entry, ok = c.processCache[pid]
	if !ok {
		entry, ok = c.externalProcessCache[pid]
		if !ok {
			return errors.WithStack(gcserr.NewProcessDoesNotExistError(pid))
		}
	}

	exitStatus := entry.ExitStatus
	// If the process has already exited, run the hook immediately.  Otherwise,
	// add it to the process's hook list.
	if exitStatus != nil {
		exitHook(exitStatus)
	} else {
		entry.AddExitHook(exitHook)
	}
	return nil
}

// setupStdioPipes begins copying data between each stdioSet reader/writer and
// the container's stdio pipes.
func (c *gcsCore) setupStdioPipes(p runtime.Process, stdioSet *core.StdioSet) error {
	pipes, err := p.GetStdioPipes()
	if err != nil {
		return err
	}
	if pipes.In != nil {
		go func() {
			io.Copy(pipes.In, stdioSet.In)
			pipes.In.Close()
			stdioSet.In.Close()
		}()
	}
	if pipes.Out != nil {
		go func() {
			io.Copy(stdioSet.Out, pipes.Out)
			pipes.Out.Close()
			stdioSet.Out.Close()
		}()
	}
	if pipes.Err != nil {
		go func() {
			io.Copy(stdioSet.Err, pipes.Err)
			pipes.Err.Close()
			stdioSet.Err.Close()
		}()
	}

	return nil
}

// setupMappedVirtualDisks is a helper function which calls into the functions
// in storage.go to set up a set of mapped virtual disks for a given container.
// It then adds them to the container's cache entry.
// This function expects containerCacheMutex to be locked on entry.
func (c *gcsCore) setupMappedVirtualDisks(id string, disks []prot.MappedVirtualDisk, containerEntry *containerCacheEntry) error {
	devices, err := c.getMappedVirtualDiskDevices(disks)
	if err != nil {
		return errors.Wrapf(err, "failed to get mapped virtual disk devices for container %s", id)
	}
	if err := c.mountMappedVirtualDisks(disks, devices); err != nil {
		return errors.Wrapf(err, "failed to mount mapped virtual disks for container %s", id)
	}
	for _, disk := range disks {
		if err := containerEntry.AddMappedVirtualDisk(disk); err != nil {
			return err
		}
	}
	return nil
}

// removeMappedVirtualDisks is a helper function which calls into the functions
// in storage.go to unmount a set of mapped virtual disks for a given
// container. It then removes them from the container's cache entry.
// This function expects containerCacheMutex to be locked on entry.
func (c *gcsCore) removeMappedVirtualDisks(id string, disks []prot.MappedVirtualDisk, containerEntry *containerCacheEntry) error {
	if err := c.unmountMappedVirtualDisks(disks); err != nil {
		return errors.Wrapf(err, "failed to mount mapped virtual disks for container %s", id)
	}
	for _, disk := range disks {
		if err := containerEntry.RemoveMappedVirtualDisk(disk); err != nil {
			return err
		}
	}
	return nil
}

// processParametersToOCI converts the given ProcessParameters struct into an
// oci.Process struct for OCI version 1.0.0-rc5-dev. Since ProcessParameters
// doesn't include various fields which are available in oci.Process, default
// values for these fields are chosen.
func processParametersToOCI(params prot.ProcessParameters) (oci.Process, error) {
	var args []string
	if len(params.CommandArgs) == 0 {
		var err error
		args, err = processParamCommandLineToOCIArgs(params.CommandLine)
		if err != nil {
			return oci.Process{}, err
		}
	} else {
		args = params.CommandArgs
	}
	return oci.Process{
		Args:     args,
		Cwd:      params.WorkingDirectory,
		Env:      processParamEnvToOCIEnv(params.Environment),
		Terminal: params.EmulateConsole,

		// TODO: We might want to eventually choose alternate default values
		// for these.
		User: oci.User{UID: 0, GID: 0},
		Capabilities: &oci.LinuxCapabilities{
			Bounding: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Effective: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Inheritable: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Permitted: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Ambient: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
		},
		Rlimits: []oci.LinuxRlimit{
			oci.LinuxRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
		},
		NoNewPrivileges: true,
	}, nil
}

// processParamCommandLineToOCIArgs converts a CommandLine field from
// ProcessParameters (a space separate argument string) into an array of string
// arguments which can be used by an oci.Process.
func processParamCommandLineToOCIArgs(commandLine string) ([]string, error) {
	args, err := shellwords.Parse(commandLine)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse command line string \"%s\"", commandLine)
	}
	return args, nil
}

// processParamEnvToOCIEnv converts an Environment field from ProcessParameters
// (a map from environment variable to value) into an array of environment
// variable assignments (where each is in the form "<variable>=<value>") which
// can be used by an oci.Process.
func processParamEnvToOCIEnv(environment map[string]string) []string {
	environmentList := make([]string, 0, len(environment))
	for k, v := range environment {
		// TODO: Do we need to escape things like quotation marks in
		// environment variable values?
		environmentList = append(environmentList, fmt.Sprintf("%s=%s", k, v))
	}
	return environmentList
}
