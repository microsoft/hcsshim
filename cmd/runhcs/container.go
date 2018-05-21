package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/regstate"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

var errContainerStopped = errors.New("container is stopped")

type persistedState struct {
	ID         string
	SandboxID  string
	Bundle     string
	Created    time.Time
	Rootfs     string
	Spec       *specs.Spec
	IsSandbox  bool
	VMIsolated bool
}

type containerStatus string

var (
	containerRunning containerStatus = "running"
	containerStopped containerStatus = "stopped"
	containerCreated containerStatus = "created"
	containerPaused  containerStatus = "paused"
	containerUnknown containerStatus = "unknown"
)

type container struct {
	persistedState
	ShimPid    int
	hc         *hcs.System
	vsmbMounts []string
}

func getErrorFromPipe(pipe io.Reader, p *os.Process) error {
	serr, err := ioutil.ReadAll(pipe)
	if err != nil {
		return err
	}

	if bytes.Equal(serr, shimSuccess) {
		return nil
	}

	extra := ""
	if p != nil {
		p.Kill()
		state, err := p.Wait()
		if err != nil {
			panic(err)
		}
		extra = fmt.Sprintf(", exit code %d", state.Sys().(syscall.WaitStatus).ExitCode)
	}
	if len(serr) == 0 {
		return fmt.Errorf("unknown shim failure%s", extra)
	}

	return errors.New(string(serr))
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
		log, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_SYNC, 0666)
		if err != nil {
			return nil, err
		}
		defer log.Close()

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
			p.Kill()
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

	err = getErrorFromPipe(rp, p)
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

func parseSandboxAnnotations(spec *specs.Spec) (string, bool) {
	a := spec.Annotations
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

func startVMShim(id, logFile string, spec *specs.Spec) (*os.Process, error) {
	layers := make([]string, len(spec.Windows.LayerFolders))
	for i, f := range spec.Windows.LayerFolders {
		if i == len(spec.Windows.LayerFolders)-1 {
			f = filepath.Join(f, "vm")
			err := os.MkdirAll(f, 0)
			if err != nil {
				return nil, err
			}
		}
		layers[i] = f
	}
	vmspec := &specs.Spec{
		Windows: &specs.Windows{
			LayerFolders: layers,
			Resources:    spec.Windows.Resources,
		},
	}
	return launchShim("vmshim", "", logFile, []string{id}, vmspec)
}

type containerConfig struct {
	ID                     string
	PidFile                string
	ShimLogFile, VMLogFile string
	Spec                   *specs.Spec
}

func createContainer(cfg *containerConfig) (_ *container, err error) {
	// Store the container information in a volatile registry key.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	sandboxID, isSandbox := parseSandboxAnnotations(cfg.Spec)

	// Determine whether this container should be created in a VM.
	newvm := false
	vmisolated := cfg.Spec.Windows != nil && cfg.Spec.Windows.HyperV != nil
	if isSandbox {
		if sandboxID != cfg.ID {
			return nil, errors.New("sandbox ID must match ID")
		}
		if vmisolated {
			newvm = true
		}
	} else if sandboxID != "" {
		// Validate that the sandbox container exists.
		sandbox, err := getContainer(sandboxID, false)
		if err != nil {
			return nil, err
		}
		defer sandbox.Close()
		if !sandbox.IsSandbox {
			return nil, fmt.Errorf("container %s is not a sandbox", sandboxID)
		}
		if vmisolated && !sandbox.VMIsolated {
			return nil, fmt.Errorf("container %s is not a VM isolated sandbox", sandboxID)
		}
		if sandbox.VMIsolated {
			vmisolated = true
		}
	} else {
		// Don't explicitly manage the VM for the container since this requires
		// RS5.
		vmisolated = false
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

	if cfg.Spec.Windows != nil {
		for i, f := range cfg.Spec.Windows.LayerFolders {
			if !filepath.IsAbs(f) && !strings.HasPrefix(rootfs, `\\?\`) {
				cfg.Spec.Windows.LayerFolders[i] = filepath.Join(cwd, f)
			}
		}
	}

	// Store the initial container state in the registry so that the delete
	// command can clean everything up if something goes wrong.
	c := &container{
		persistedState: persistedState{
			ID:         cfg.ID,
			Bundle:     cwd,
			Rootfs:     rootfs,
			Created:    time.Now(),
			Spec:       cfg.Spec,
			VMIsolated: vmisolated,
			IsSandbox:  isSandbox,
			SandboxID:  sandboxID,
		},
	}
	err = stateKey.Create(cfg.ID, "state", &c.persistedState)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			c.Remove()
		}
	}()

	// Start a VM if necessary.
	if newvm {
		shim, err := startVMShim(cfg.ID, cfg.VMLogFile, cfg.Spec)
		if err != nil {
			return nil, err
		}
		shim.Release()
	}

	if vmisolated {
		// Call to the VM shim process to create the container. This is done so
		// that the VM process can keep track of the VM's virtual hardware
		// resource use.
		/* DISABLED
		err = issueVMRequest(sandboxID, cfg.ID, opCreateContainer)
		if err != nil {
			return nil, err
		}
		*/
		c.hc, err = hcs.OpenComputeSystem(cfg.ID)
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
			c.Remove()
		}
		return nil, err
	}

	return c, nil
}

func (c *container) forceUnmount(vm *uvm.UtilityVM, all bool) error {
	// Unmount the container layers. Only the writable layer needs to be
	// unmounted (in order to flush any filesystem changes) if the VM is about
	// to terminate.
	op := hcsoci.UnmountOperation(hcsoci.UnmountOperationSCSI)
	if all {
		op = hcsoci.UnmountOperationAll
	}
	if err := hcsoci.UnmountContainerLayers(c.Spec.Windows.LayerFolders, vm, op); err != nil {
		return err
	}

	/* DISABLED
	// Unmount any VSMB mounts for volume mappings
	if all && vm != nil {
		for len(c.vsmbMounts) != 0 {
			mount := c.vsmbMounts[len(c.vsmbMounts)-1]
			if err := hcsshim.RemoveVSMB(vm, mount); err != nil {
				return err
			}
			c.vsmbMounts = c.vsmbMounts[:len(c.vsmbMounts)-1]
		}
	}
	*/

	// Remember that the unmount is complete.
	return stateKey.Set(c.ID, "mount", false)
}

func (c *container) Unmount(all bool) error {
	mounted := false
	err := stateKey.Get(c.ID, "mount", &mounted)
	if err != nil {
		if _, ok := err.(*regstate.NoStateError); ok {
			return nil
		}
		return err
	}
	if mounted {
		if c.VMIsolated {
			/* DISABLED
			op := opUnmountContainerDiskOnly
			if all {
				// The sandbox i
				op = opUnmountContainerDiskOnly
			}
			err = issueVMRequest(c.SandboxID, c.ID, op)
			if err == errNoVM {
				// The VM is not running, so the storage should be unmounted
				// already.
				err = nil
			}
			*/
		} else {
			err = c.forceUnmount(nil, true)
		}
	}
	return err
}

func createContainerInHost(c *container, vm *uvm.UtilityVM) (err error) {
	if c.hc != nil {
		return errors.New("container already created")
	}

	if c.Rootfs == "" && c.Spec.Windows != nil && (vm != nil || c.Spec.Windows.HyperV == nil) {
		root, err := hcsoci.MountContainerLayers(c.Spec.Windows.LayerFolders, vm)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				if e := c.forceUnmount(vm, true); e != nil {
					logrus.Error("failed to unwind mount for ", c.ID, ": ", e)
				}
			}
		}()

		err = stateKey.Set(c.ID, "mount", true)
		if err != nil {
			return err
		}
		if c.Spec.Root == nil {
			c.Spec.Root = &specs.Root{}
		}
		if vm == nil {
			c.Spec.Root.Path = root.(string)
		} else {
			c.Spec.Root.Path = root.(schema2.CombinedLayersV2).ContainerRootPath
		}
	}

	// Create the container without starting it.
	opts := &hcsoci.CreateOptions{
		Id:            c.ID,
		Spec:          c.Spec,
		HostingSystem: vm,
	}
	if vm != nil {
		opts.SchemaVersion = schemaversion.SchemaV20()
	}

	vmid := ""
	if vm != nil {
		/* DISABLED
		vmid = vm.ID()
		*/
	}
	logrus.Infof("creating container %s (VM: '%s')", c.ID, vmid)
	hc, err := hcsoci.CreateContainer(opts)
	if err != nil {
		return err
	}
	if vm != nil {
		/* DISABLED
		// Track the VSMB mounts to unmount later
		for _, m := range c.Spec.Mounts {
			c.vsmbMounts = append(c.vsmbMounts, m.Source)
		}
		*/
	}
	c.hc = hc
	return nil
}

func startContainerShim(c *container, pidFile, logFile string) error {
	// Launch a shim process to later execute a process in the container.
	shim, err := startProcessShim(c.ID, pidFile, logFile, nil)
	if err != nil {
		return err
	}
	defer shim.Release()
	defer func() {
		if err != nil {
			shim.Kill()
		}
	}()

	c.ShimPid = shim.Pid
	err = stateKey.Set(c.ID, "shim", shim.Pid)
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
	err := c.hc.Start()
	if err != nil {
		return err
	}

	if c.Spec.Process == nil {
		return nil
	}

	// Alert the shim that the container is ready.
	pipe, err := winio.DialPipe(containerPipePath(c.ID), nil)
	if err != nil {
		return err
	}
	defer pipe.Close()

	shim, err := os.FindProcess(c.ShimPid)
	if err != nil {
		return err
	}
	defer shim.Release()

	err = getErrorFromPipe(pipe, shim)
	if err != nil {
		return err
	}

	return nil
}

func getContainer(id string, notStopped bool) (*container, error) {
	var c container
	err := stateKey.Get(id, "state", &c.persistedState)
	if err != nil {
		return nil, err
	}
	err = stateKey.Get(id, "shim", &c.ShimPid)
	if err != nil {
		if _, ok := err.(*regstate.NoStateError); !ok {
			return nil, err
		}
		c.ShimPid = -1
	}
	if notStopped && c.ShimPid == 0 {
		return nil, errContainerStopped
	}

	hc, err := hcs.OpenComputeSystem(c.ID)
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
	err := c.Unmount(!c.VMIsolated || !c.IsSandbox)
	if err != nil {
		return err
	}

	// Follow kata's example and delay tearing down the VM until the owning
	// container is removed.
	if c.IsSandbox && c.VMIsolated {
		/* DISABLED
		vm, err := hcsshim.OpenContainer(vmID(c.ID))
		if err == nil {
			if err := vm.Terminate(); hcsshim.IsPending(err) {
				vm.Wait()
			}
		}
		*/
	}
	return stateKey.Remove(c.ID)
}

func (c *container) Kill() error {
	if c.hc == nil {
		return nil
	}
	err := c.hc.Terminate()
	if hcs.IsPending(err) {
		err = c.hc.Wait()
	}
	if hcs.IsAlreadyStopped(err) {
		err = nil
	}
	return err
}

func (c *container) Status() (containerStatus, error) {
	if c.hc == nil || c.ShimPid == 0 {
		return containerStopped, nil
	}
	props, err := c.hc.Properties()
	if err != nil {
		return "", err
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
