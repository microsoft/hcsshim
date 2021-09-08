package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cni"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/regstate"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/runhcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

var errContainerStopped = errors.New("container is stopped")

type persistedState struct {
	// ID is the id of this container/UVM.
	ID string `json:",omitempty"`
	// Owner is the owner value passed into the runhcs command and may be `""`.
	Owner string `json:",omitempty"`
	// SandboxID is the sandbox identifer passed in via OCI specifications. This
	// can either be the sandbox itself or the sandbox this container should run
	// in. See `parseSandboxAnnotations`.
	SandboxID string `json:",omitempty"`
	// HostID will be VM ID hosting this container. If a sandbox is used it will
	// match the `SandboxID`.
	HostID string `json:",omitempty"`
	// Bundle is the folder path on disk where the container state and spec files
	// reside.
	Bundle  string    `json:",omitempty"`
	Created time.Time `json:",omitempty"`
	Rootfs  string    `json:",omitempty"`
	// Spec is the in memory deserialized values found on `Bundle\config.json`.
	Spec           *specs.Spec `json:",omitempty"`
	RequestedNetNS string      `json:",omitempty"`
	// IsHost is `true` when this is a VM isolated config.
	IsHost bool `json:",omitempty"`
	// UniqueID is a unique ID generated per container config.
	UniqueID guid.GUID `json:",omitempty"`
	// HostUniqueID is the unique ID of the hosting VM if this container is
	// hosted.
	HostUniqueID guid.GUID `json:",omitempty"`
}

type containerStatus string

const (
	containerRunning containerStatus = "running"
	containerStopped containerStatus = "stopped"
	containerCreated containerStatus = "created"
	containerPaused  containerStatus = "paused"
	containerUnknown containerStatus = "unknown"

	keyState     = "state"
	keyResources = "resources"
	keyShimPid   = "shim"
	keyInitPid   = "pid"
	keyNetNS     = "netns"
	// keyPidMapFmt is the format to use when mapping a host OS pid to a guest
	// pid.
	keyPidMapFmt = "pid-%d"
)

type container struct {
	persistedState
	ShimPid int
	hc      *hcs.System
}

func startProcessShim(id, pidFile, logFile string, spec *specs.Process) (_ *os.Process, err error) {
	// Ensure the stdio handles inherit to the child process. This isn't undone
	// after the StartProcess call because the caller never launches another
	// process before exiting.
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		err = windows.SetHandleInformation(windows.Handle(f.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)
		if err != nil {
			return nil, err
		}
	}

	args := []string{
		"--stdin", strconv.Itoa(int(os.Stdin.Fd())),
		"--stdout", strconv.Itoa(int(os.Stdout.Fd())),
		"--stderr", strconv.Itoa(int(os.Stderr.Fd())),
	}
	if spec != nil {
		args = append(args, "--exec")
	}
	if strings.HasPrefix(logFile, runhcs.SafePipePrefix) {
		args = append(args, "--log-pipe", logFile)
	}
	args = append(args, id)
	return launchShim("shim", pidFile, logFile, args, spec)
}

func launchShim(cmd, pidFile, logFile string, args []string, data interface{}) (_ *os.Process, err error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	// Create a pipe to use as stderr for the shim process. This is used to
	// retrieve early error information, up to the point that the shim is ready
	// to launch a process in the container.
	rp, wp, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer rp.Close()
	defer wp.Close()

	// Create a pipe to send the data, if one is provided.
	var rdatap, wdatap *os.File
	if data != nil {
		rdatap, wdatap, err = os.Pipe()
		if err != nil {
			return nil, err
		}
		defer rdatap.Close()
		defer wdatap.Close()
	}

	var log *os.File
	fullargs := []string{os.Args[0]}
	if logFile != "" {
		if !strings.HasPrefix(logFile, runhcs.SafePipePrefix) {
			log, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_SYNC, 0666)
			if err != nil {
				return nil, err
			}
			defer log.Close()
		}

		fullargs = append(fullargs, "--log-format", logFormat)
		if logrus.GetLevel() == logrus.DebugLevel {
			fullargs = append(fullargs, "--debug")
		}
	}
	fullargs = append(fullargs, cmd)
	fullargs = append(fullargs, args...)
	attr := &os.ProcAttr{
		Files: []*os.File{rdatap, wp, log},
	}
	p, err := os.StartProcess(executable, fullargs, attr)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = p.Kill()
		}
	}()

	wp.Close()

	// Write the data if provided.
	if data != nil {
		rdatap.Close()
		dataj, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		_, err = wdatap.Write(dataj)
		if err != nil {
			return nil, err
		}
		wdatap.Close()
	}

	err = runhcs.GetErrorFromPipe(rp, p)
	if err != nil {
		return nil, err
	}

	if pidFile != "" {
		if err = createPidFile(pidFile, p.Pid); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// parseSandboxAnnotations searches `a` for various annotations used by
// different runtimes to represent a sandbox ID, and sandbox type.
//
// If found returns the tuple `(sandboxID, isSandbox)` where `isSandbox == true`
// indicates the identifer is the sandbox itself; `isSandbox == false` indicates
// the identifer is the sandbox in which to place this container. Otherwise
// returns `("", false)`.
func parseSandboxAnnotations(a map[string]string) (string, bool) {
	var t, id string
	if t = a["io.kubernetes.cri.container-type"]; t != "" {
		id = a["io.kubernetes.cri.sandbox-id"]
	} else if t = a["io.kubernetes.cri-o.ContainerType"]; t != "" {
		id = a["io.kubernetes.cri-o.SandboxID"]
	} else if t = a["io.kubernetes.docker.type"]; t != "" {
		id = a["io.kubernetes.sandbox.id"]
		if t == "podsandbox" {
			t = "sandbox"
		}
	}
	if t == "container" {
		return id, false
	}
	if t == "sandbox" {
		return id, true
	}
	return "", false
}

// startVMShim starts a vm-shim command with the specified `opts`. `opts` can be `uvm.OptionsWCOW` or `uvm.OptionsLCOW`
func (c *container) startVMShim(logFile string, opts interface{}) (*os.Process, error) {
	var os string
	if _, ok := opts.(*uvm.OptionsLCOW); ok {
		os = "linux"
	} else {
		os = "windows"
	}
	args := []string{"--os", os}
	if strings.HasPrefix(logFile, runhcs.SafePipePrefix) {
		args = append(args, "--log-pipe", logFile)
	}
	args = append(args, c.VMPipePath())
	return launchShim("vmshim", "", logFile, args, opts)
}

type containerConfig struct {
	ID                     string
	Owner                  string
	HostID                 string
	PidFile                string
	ShimLogFile, VMLogFile string
	Spec                   *specs.Spec
	VMConsolePipe          string
}

func createContainer(cfg *containerConfig) (_ *container, err error) {
	// Store the container information in a volatile registry key.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	vmisolated := cfg.Spec.Linux != nil || (cfg.Spec.Windows != nil && cfg.Spec.Windows.HyperV != nil)

	sandboxID, isSandbox := parseSandboxAnnotations(cfg.Spec.Annotations)
	hostID := cfg.HostID
	if isSandbox {
		if sandboxID != cfg.ID {
			return nil, errors.New("sandbox ID must match ID")
		}
	} else if sandboxID != "" {
		// Validate that the sandbox container exists.
		sandbox, err := getContainer(sandboxID, false)
		if err != nil {
			return nil, err
		}
		defer sandbox.Close()
		if sandbox.SandboxID != sandboxID {
			return nil, fmt.Errorf("container %s is not a sandbox", sandboxID)
		}
		if hostID == "" {
			// Use the sandbox's host.
			hostID = sandbox.HostID
		} else if sandbox.HostID == "" {
			return nil, fmt.Errorf("sandbox container %s is not running in a VM host, but host %s was specified", sandboxID, hostID)
		} else if hostID != sandbox.HostID {
			return nil, fmt.Errorf("sandbox container %s has a different host %s from the requested host %s", sandboxID, sandbox.HostID, hostID)
		}
		if vmisolated && hostID == "" {
			return nil, fmt.Errorf("container %s is not a VM isolated sandbox", sandboxID)
		}
	}

	uniqueID, err := guid.NewV4()
	if err != nil {
		return nil, err
	}

	newvm := false
	var hostUniqueID guid.GUID
	if hostID != "" {
		host, err := getContainer(hostID, false)
		if err != nil {
			return nil, err
		}
		defer host.Close()
		if !host.IsHost {
			return nil, fmt.Errorf("host container %s is not a VM host", hostID)
		}
		hostUniqueID = host.UniqueID
	} else if vmisolated && (isSandbox || cfg.Spec.Linux != nil || osversion.Build() >= osversion.RS5) {
		// This handles all LCOW, Pod Sandbox, and (Windows Xenon V2 for RS5+)
		hostID = cfg.ID
		newvm = true
		hostUniqueID = uniqueID
	}

	// Make absolute the paths in Root.Path and Windows.LayerFolders.
	rootfs := ""
	if cfg.Spec.Root != nil {
		rootfs = cfg.Spec.Root.Path
		if rootfs != "" && !filepath.IsAbs(rootfs) && !strings.HasPrefix(rootfs, `\\?\`) {
			rootfs = filepath.Join(cwd, rootfs)
			cfg.Spec.Root.Path = rootfs
		}
	}

	netNS := ""
	if cfg.Spec.Windows != nil {
		for i, f := range cfg.Spec.Windows.LayerFolders {
			if !filepath.IsAbs(f) && !strings.HasPrefix(rootfs, `\\?\`) {
				cfg.Spec.Windows.LayerFolders[i] = filepath.Join(cwd, f)
			}
		}

		// Determine the network namespace to use.
		if cfg.Spec.Windows.Network != nil {
			if cfg.Spec.Windows.Network.NetworkSharedContainerName != "" {
				// RS4 case
				err = stateKey.Get(cfg.Spec.Windows.Network.NetworkSharedContainerName, keyNetNS, &netNS)
				if err != nil {
					if _, ok := err.(*regstate.NoStateError); !ok {
						return nil, err
					}
				}
			} else if cfg.Spec.Windows.Network.NetworkNamespace != "" {
				// RS5 case
				netNS = cfg.Spec.Windows.Network.NetworkNamespace
			}
		}
	}

	// Store the initial container state in the registry so that the delete
	// command can clean everything up if something goes wrong.
	c := &container{
		persistedState: persistedState{
			ID:             cfg.ID,
			Owner:          cfg.Owner,
			Bundle:         cwd,
			Rootfs:         rootfs,
			Created:        time.Now(),
			Spec:           cfg.Spec,
			SandboxID:      sandboxID,
			HostID:         hostID,
			IsHost:         newvm,
			RequestedNetNS: netNS,
			UniqueID:       uniqueID,
			HostUniqueID:   hostUniqueID,
		},
	}
	err = stateKey.Create(cfg.ID, keyState, &c.persistedState)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = c.Remove()
		}
	}()
	if isSandbox && vmisolated {
		cnicfg := cni.NewPersistedNamespaceConfig(netNS, cfg.ID, hostUniqueID)
		err = cnicfg.Store()
		if err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				_ = cnicfg.Remove()
			}
		}()
	}

	// Start a VM if necessary.
	if newvm {
		opts, err := oci.SpecToUVMCreateOpts(context.Background(), cfg.Spec, vmID(c.ID), cfg.Owner)
		if err != nil {
			return nil, err
		}
		switch opts := opts.(type) {
		case *uvm.OptionsLCOW:
			opts.ConsolePipe = cfg.VMConsolePipe
		case *uvm.OptionsWCOW:
			// In order for the UVM sandbox.vhdx not to collide with the actual
			// nested Argon sandbox.vhdx we append the \vm folder to the last entry
			// in the list.
			layersLen := len(cfg.Spec.Windows.LayerFolders)
			layers := make([]string, layersLen)
			copy(layers, cfg.Spec.Windows.LayerFolders)

			vmPath := filepath.Join(layers[layersLen-1], "vm")
			err := os.MkdirAll(vmPath, 0)
			if err != nil {
				return nil, err
			}
			layers[layersLen-1] = vmPath

			opts.LayerFolders = layers
		}

		shim, err := c.startVMShim(cfg.VMLogFile, opts)
		if err != nil {
			return nil, err
		}
		_ = shim.Release()
	}

	if c.HostID != "" {
		// Call to the VM shim process to create the container. This is done so
		// that the VM process can keep track of the VM's virtual hardware
		// resource use.
		err = c.issueVMRequest(runhcs.OpCreateContainer)
		if err != nil {
			return nil, err
		}
		c.hc, err = hcs.OpenComputeSystem(context.Background(), cfg.ID)
		if err != nil {
			return nil, err
		}
	} else {
		// Create the container directly from this process.
		err = createContainerInHost(c, nil)
		if err != nil {
			return nil, err
		}
	}

	// Create the shim process for the container.
	err = startContainerShim(c, cfg.PidFile, cfg.ShimLogFile)
	if err != nil {
		if e := c.Kill(); e == nil {
			_ = c.Remove()
		}
		return nil, err
	}

	return c, nil
}

func (c *container) ShimPipePath() string {
	return runhcs.SafePipePath("runhcs-shim-" + c.UniqueID.String())
}

func (c *container) VMPipePath() string {
	return runhcs.VMPipePath(c.HostUniqueID)
}

func (c *container) VMIsolated() bool {
	return c.HostID != ""
}

func (c *container) unmountInHost(vm *uvm.UtilityVM, all bool) error {
	r := &resources.Resources{}
	err := stateKey.Get(c.ID, keyResources, r)
	if _, ok := err.(*regstate.NoStateError); ok {
		return nil
	}
	if err != nil {
		return err
	}
	err = resources.ReleaseResources(context.Background(), r, vm, all)
	if err != nil {
		_ = stateKey.Set(c.ID, keyResources, r)
		return err
	}

	err = stateKey.Clear(c.ID, keyResources)
	if err != nil {
		return err
	}
	return nil
}

func (c *container) Unmount(all bool) error {
	if c.VMIsolated() {
		op := runhcs.OpUnmountContainerDiskOnly
		if all {
			op = runhcs.OpUnmountContainer
		}
		err := c.issueVMRequest(op)
		if _, ok := err.(*noVMError); ok {
			logrus.WithFields(logrus.Fields{
				logfields.ContainerID: c.ID,
				logfields.UVMID:       c.HostID,
				logrus.ErrorKey:       errors.New("failed to unmount container resources"),
			}).Warning("VM shim could not be contacted")
		} else {
			return err
		}
	} else {
		_ = c.unmountInHost(nil, false)
	}
	return nil
}

func createContainerInHost(c *container, vm *uvm.UtilityVM) (err error) {
	if c.hc != nil {
		return errors.New("container already created")
	}

	// Create the container without starting it.
	opts := &hcsoci.CreateOptions{
		ID:               c.ID,
		Owner:            c.Owner,
		Spec:             c.Spec,
		HostingSystem:    vm,
		NetworkNamespace: c.RequestedNetNS,
	}
	vmid := ""
	if vm != nil {
		vmid = vm.ID()
	}
	logrus.WithFields(logrus.Fields{
		logfields.ContainerID: c.ID,
		logfields.UVMID:       vmid,
	}).Info("creating container in UVM")
	hc, r, err := hcsoci.CreateContainer(context.Background(), opts)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = hc.Terminate(context.Background())
			_ = hc.Wait()
			_ = resources.ReleaseResources(context.Background(), r, vm, true)
		}
	}()

	// Record the network namespace to support namespace sharing by container ID.
	if r.NetNS() != "" {
		err = stateKey.Set(c.ID, keyNetNS, r.NetNS())
		if err != nil {
			return err
		}
	}

	err = stateKey.Set(c.ID, keyResources, r)
	if err != nil {
		return err
	}
	c.hc = hc.(*hcs.System)
	return nil
}

func startContainerShim(c *container, pidFile, logFile string) error {
	// Launch a shim process to later execute a process in the container.
	shim, err := startProcessShim(c.ID, pidFile, logFile, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = shim.Release()
	}()
	defer func() {
		if err != nil {
			_ = shim.Kill()
		}
	}()

	c.ShimPid = shim.Pid
	err = stateKey.Set(c.ID, keyShimPid, shim.Pid)
	if err != nil {
		return err
	}

	if pidFile != "" {
		if err = createPidFile(pidFile, shim.Pid); err != nil {
			return err
		}
	}

	return nil
}

func (c *container) Close() error {
	if c.hc == nil {
		return nil
	}
	return c.hc.Close()
}

func (c *container) Exec() error {
	err := c.hc.Start(context.Background())
	if err != nil {
		return err
	}

	if c.Spec.Process == nil {
		return nil
	}

	// Alert the shim that the container is ready.
	pipe, err := winio.DialPipe(c.ShimPipePath(), nil)
	if err != nil {
		return err
	}
	defer pipe.Close()

	shim, err := os.FindProcess(c.ShimPid)
	if err != nil {
		return err
	}
	defer func() {
		_ = shim.Release()
	}()

	err = runhcs.GetErrorFromPipe(pipe, shim)
	if err != nil {
		return err
	}

	return nil
}

func getContainer(id string, notStopped bool) (*container, error) {
	var c container
	err := stateKey.Get(id, keyState, &c.persistedState)
	if err != nil {
		return nil, err
	}
	err = stateKey.Get(id, keyShimPid, &c.ShimPid)
	if err != nil {
		if _, ok := err.(*regstate.NoStateError); !ok {
			return nil, err
		}
		c.ShimPid = -1
	}
	if notStopped && c.ShimPid == 0 {
		return nil, errContainerStopped
	}

	hc, err := hcs.OpenComputeSystem(context.Background(), c.ID)
	if err == nil {
		c.hc = hc
	} else if !hcs.IsNotExist(err) {
		return nil, err
	} else if notStopped {
		return nil, errContainerStopped
	}

	return &c, nil
}

func (c *container) Remove() error {
	// Unmount any layers or mapped volumes.
	err := c.Unmount(!c.IsHost)
	if err != nil {
		return err
	}

	// Follow kata's example and delay tearing down the VM until the owning
	// container is removed.
	if c.IsHost {
		vm, err := hcs.OpenComputeSystem(context.Background(), vmID(c.ID))
		if err == nil {
			_ = vm.Terminate(context.Background())
			_ = vm.Wait()
		}
	}
	return stateKey.Remove(c.ID)
}

func (c *container) Kill() error {
	if c.hc == nil {
		return nil
	}
	_ = c.hc.Terminate(context.Background())
	return c.hc.Wait()
}

func (c *container) Status() (containerStatus, error) {
	if c.hc == nil || c.ShimPid == 0 {
		return containerStopped, nil
	}
	props, err := c.hc.Properties(context.Background())
	if err != nil {
		if !strings.Contains(err.Error(), "operation is not valid in the current state") {
			return "", err
		}
		return containerUnknown, nil
	}
	state := containerUnknown
	switch props.State {
	case "", "Created":
		state = containerCreated
	case "Running":
		state = containerRunning
	case "Paused":
		state = containerPaused
	case "Stopped":
		state = containerStopped
	}
	return state, nil
}
